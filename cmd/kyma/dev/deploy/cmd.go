package deploy

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/kyma-project/cli/cmd/kyma/dev"
	"github.com/kyma-project/cli/internal/cli"
	"github.com/kyma-project/cli/internal/kube"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type command struct {
	opts *Options
	cli.Command
}

//NewCmd creates a new minikube command
func NewCmd(o *Options) *cobra.Command {
	c := command{
		Command: cli.Command{Options: o.Options},
		opts:    o,
	}

	cmd := &cobra.Command{
		Use:   "deploy",
		Short: "Deploy Lambda function to cluster",
		Long:  `Creates a new local lambda function setup to start development`,
		RunE:  func(_ *cobra.Command, _ []string) error { return c.Run() },
	}

	cmd.Flags().StringVarP(&o.WorkingDir, "workdir", "d", "", "Directory where to run the command")

	return cmd
}

func (cmd *command) Run() error {
	if err := cmd.validateFlags(); err != nil {
		return err
	}

	kubeconfigPath := cmd.KubeconfigPath
	if len(cmd.KubeconfigPath) == 0 {
		usr, err := user.Current()
		if err != nil {
			return errors.Wrap(err, "Could not determine home directory")
		}
		kubeconfigPath = filepath.Join(usr.HomeDir, ".kube/config")
	}

	var err error
	if cmd.K8s, err = kube.NewFromConfig("", kubeconfigPath); err != nil {
		return errors.Wrap(err, "Could not initialize the Kubernetes client. Make sure your kubeconfig is valid")
	}

	err = deployFunctionFromPath(cmd.K8s, cmd.opts.WorkingDir)
	if err != nil {
		return err
	}

	return nil
}

func (c *command) validateFlags() error {
	var errMessage strings.Builder

	if errMessage.Len() != 0 {
		return errors.New(errMessage.String())
	}
	return nil
}

func deployFunctionFromPath(kube kube.KymaKube, workdir string) error {
	config, currentDir, err := dev.GetConfig(workdir)
	if err != nil {
		return err
	}

	err = deployFunctionForConfig(config, *currentDir, kube)
	if err != nil {
		return err
	}

	return nil
}

func deployFunctionForConfig(config *dev.Config, currentDir string, kube kube.KymaKube) error {
	err := ensureFunction(config, currentDir, kube)
	if err != nil {
		return err
	}

	if config.Expose {
		err := ensureApi(config, currentDir, kube)
		if err != nil {
			return err
		}
	}

	return nil
}

func ensureFunction(config *dev.Config, currentDir string, kube kube.KymaKube) error {
	functionRes := schema.GroupVersionResource{
		Group:    "kubeless.io",
		Version:  "v1beta1",
		Resource: "functions",
	}
	itm, err := kube.Dynamic().Resource(functionRes).Namespace(config.Namespace).Get(config.Name, metav1.GetOptions{})
	if err != nil {
		if !k8sErrors.IsNotFound(err) {
			return errors.Wrap(err, "Failed to check Application")
		}
	}
	sourceFilePath := filepath.Join(currentDir, config.File)
	if !dev.FileExists(sourceFilePath) {
		return errors.New(fmt.Sprintf("Referenced file in kyma.json does not exist at %s", sourceFilePath))
	}

	b, err := ioutil.ReadFile(sourceFilePath)
	if err != nil {
		return errors.Wrap(err, "Cannot read config file")
	}
	functionCode := string(b)

	checksum, err := getChecksum(functionCode)
	if err != nil {
		return errors.Wrap(err, "Cannot generate checksum")
	}
	checksum = fmt.Sprintf("sha256:%s", checksum)

	var dependencies *string
	packageFile := filepath.Join(currentDir, "package.json")
	if dev.FileExists(packageFile) {
		b, err := ioutil.ReadFile(packageFile)
		if err != nil {
			return errors.Wrap(err, "Cannot read package json")
		}
		var result map[string]interface{}
		json.Unmarshal([]byte(b), &result)
		if dep, ok := result["dependencies"]; ok {
			depArr, err := json.Marshal(dep)
			if err != nil {
				return errors.Wrap(err, "Cannot read package json")
			}
			depsStr := fmt.Sprintf("{\n \"dependencies\": %s \n}", string(depArr))
			dependencies = &depsStr
		}

	}

	if itm != nil {
		// Update Function parameter
		log.Infof("[UPDATE] Updating Function - functions.kubeless.io/v1beta1 %s/%s", config.Namespace, config.Name)
		if err := unstructured.SetNestedField(itm.Object, functionCode, "spec", "function"); err != nil {
			errors.Wrap(err, "Failed to update function.")
		}
		if err := unstructured.SetNestedField(itm.Object, checksum, "spec", "checksum"); err != nil {
			errors.Wrap(err, "Failed to update checksum.")
		}
		if dependencies != nil {
			if err := unstructured.SetNestedField(itm.Object, *dependencies, "spec", "deps"); err != nil {
				errors.Wrap(err, "Failed to update function.")
			}
		}

		_, err := kube.Dynamic().Resource(functionRes).Namespace(config.Namespace).Update(itm, metav1.UpdateOptions{})
		if err != nil {
			return errors.Wrap(err, "Failed to update function.")
		}
	} else {
		// Create Function
		log.Infof("[CREATE] Creating Function - functions.kubeless.io/v1beta1 %s/%s", config.Namespace, config.Name)
		newFunction := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "kubeless.io/v1beta1",
				"kind":       "Function",
				"metadata": map[string]interface{}{
					"name": config.Name,
					"labels": map[string]interface{}{
						"app": config.Name,
					},
				},
				"spec": map[string]interface{}{
					"checksum": checksum,
					"runtime":  "nodejs8",
					"type":     "HTTP",
					"handler":  "handler.main",
					"function": functionCode,
					"deps":     "",
				},
			},
		}

		if dependencies != nil {
			if err := unstructured.SetNestedField(newFunction.Object, *dependencies, "spec", "deps"); err != nil {
				errors.Wrap(err, "Failed to update function.")
			}
		}

		itm, err = kube.Dynamic().Resource(functionRes).Namespace(config.Namespace).Create(newFunction, metav1.CreateOptions{})
		if err != nil {
			return errors.Wrap(err, "Failed to create function.")
		}
	}

	return nil
}

func ensureApi(config *dev.Config, currentDir string, kube kube.KymaKube) error {
	apiRes := schema.GroupVersionResource{
		Group:    "gateway.kyma-project.io",
		Version:  "v1alpha2",
		Resource: "apis",
	}
	itm, err := kube.Dynamic().Resource(apiRes).Namespace(config.Namespace).Get(config.Name, metav1.GetOptions{})
	if err != nil {
		if !k8sErrors.IsNotFound(err) {
			return errors.Wrap(err, "Failed to check Application")
		}
	}

	if itm != nil {
		log.Infof("[SKIP] API already exists - apis.gateway.kyma-project.io/v1alpha2 %s/%s", config.Namespace, config.Name)
	} else {
		// Create Function
		log.Infof("[CREATE] Creating API - apis.gateway.kyma-project.io/v1alpha2 %s/%s", config.Namespace, config.Name)
		newApi := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "gateway.kyma-project.io/v1alpha2",
				"kind":       "Api",
				"metadata": map[string]interface{}{
					"name": config.Name,
				},
				"spec": map[string]interface{}{
					"authentication": []map[string]interface{}{},
					"hostname":       fmt.Sprintf("%s.%s", config.Name, config.ClusterDomain),
					"service": map[string]interface{}{
						"name": config.Name,
						"port": 8080,
					},
				},
			},
		}

		itm, err = kube.Dynamic().Resource(apiRes).Namespace(config.Namespace).Create(newApi, metav1.CreateOptions{})
		if err != nil {
			return errors.Wrap(err, "Failed to create function.")
		}
	}

	return nil
}

func getChecksum(content string) (string, error) {
	h := sha256.New()
	_, err := h.Write([]byte(content))
	if err != nil {
		return "", nil
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
