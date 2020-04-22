package createToken

import (
	"fmt"
	"strings"
	"time"

	"github.com/kyma-project/cli/internal/cli"
	"github.com/kyma-project/cli/internal/kube"
	"github.com/pkg/errors"

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
		Use:   "create-token",
		Short: "Creates a new Token for an Application",
		Long:  `Use this command to create a new token for an application`,
		RunE:  func(_ *cobra.Command, _ []string) error { return c.Run() },
	}

	cmd.Flags().StringVarP(&o.Name, "name", "n", "", "Name of application to create the token for")

	return cmd
}

func (cmd *command) Run() error {
	if err := cmd.validateFlags(); err != nil {
		return err
	}

	var err error
	if cmd.K8s, err = kube.NewFromConfig("", cmd.KubeconfigPath); err != nil {
		return errors.Wrap(err, "Could not initialize the Kubernetes client. Make sure your kubeconfig is valid")
	}

	token, err := createToken(cmd.opts.Name, cmd.K8s)
	if err != nil {
		return errors.Wrap(err, "Could not create Application")
	}

	fmt.Println(*token)
	return nil
}

func (c *command) validateFlags() error {
	var errMessage strings.Builder
	// mandatory flags
	if c.opts.Name == "" {
		errMessage.WriteString("\nRequired flag `name` has not been set.")
	}

	if errMessage.Len() != 0 {
		return errors.New(errMessage.String())
	}
	return nil
}

func createToken(name string, kube kube.KymaKube) (*string, error) {

	applicationRes := schema.GroupVersionResource{
		Group:    "applicationconnector.kyma-project.io",
		Version:  "v1alpha1",
		Resource: "applications",
	}

	// Verify if application exists
	_, err := kube.Dynamic().Resource(applicationRes).Get(name, metav1.GetOptions{})
	if err != nil {
		if k8sErrors.IsNotFound(err) {
			return nil, errors.Wrap(err, "Failed to create Token, application does not exist")
		}
		return nil, errors.Wrap(err, "Failed to create Token, cannot select application")
	}

	newToken := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "applicationconnector.kyma-project.io/v1alpha1",
			"kind":       "TokenRequest",
			"metadata": map[string]interface{}{
				"name": name,
			},
		},
	}
	tokenRequestRes := schema.GroupVersionResource{
		Group:    "applicationconnector.kyma-project.io",
		Version:  "v1alpha1",
		Resource: "tokenrequests",
	}

	// Check if a token with that name already exists
	itm, err := kube.Dynamic().Resource(tokenRequestRes).Namespace("default").Get(name, metav1.GetOptions{})
	if err != nil {
		if !k8sErrors.IsNotFound(err) {
			return nil, errors.Wrap(err, "Failed to create Token, application does not exist")
		}
	}

	if itm != nil {
		// Token already exists, deleting it.
		err = kube.Dynamic().Resource(tokenRequestRes).Namespace("default").Delete(name, &metav1.DeleteOptions{})
		if err != nil {
			if !k8sErrors.IsNotFound(err) {
				return nil, errors.Wrap(err, "Failed to remove Token")
			}
		}
	}

	_, err = kube.Dynamic().Resource(tokenRequestRes).Namespace("default").Create(newToken, metav1.CreateOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "Failed to create token.")
	}

	maxCounter := 30
	for i := 0; i < maxCounter; i++ {
		itm, err := kube.Dynamic().Resource(tokenRequestRes).Namespace("default").Get(name, metav1.GetOptions{})
		if err != nil {
			return nil, errors.Wrap(err, "Failed to get resource.")
		}

		if itm == nil {
			return nil, errors.Wrap(err, "Failed to request token.")
		}

		tokenurl, exists, err := unstructured.NestedString(itm.Object, "status", "url")
		if err != nil {
			return nil, errors.Wrap(err, "Failed to get token")
		}

		if !exists {
			time.Sleep(2 * time.Second)
			continue
		}

		return &tokenurl, nil
	}

	return nil, errors.Wrap(err, "Failed to request token.")
}
