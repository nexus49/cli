package createApplication

import (
	"strings"

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
		Use:   "create-application",
		Short: "Creates a new Application",
		Long:  `Use this command to create a new application in the cluster`,
		RunE:  func(_ *cobra.Command, _ []string) error { return c.Run() },
	}

	cmd.Flags().StringVarP(&o.Name, "name", "n", "", "Name of application to be created")

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

	err = createApplication(cmd.opts.Name, cmd.K8s)
	if err != nil {
		return errors.Wrap(err, "Could not create Application")
	}
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

func createApplication(name string, kube kube.KymaKube) error {
	applicationRes := schema.GroupVersionResource{
		Group:    "applicationconnector.kyma-project.io",
		Version:  "v1alpha1",
		Resource: "applications",
	}
	itm, err := kube.Dynamic().Resource(applicationRes).Get(name, metav1.GetOptions{})
	if err != nil {
		if !k8sErrors.IsNotFound(err) {
			return errors.Wrap(err, "Failed to check Application")
		}
	}

	if itm != nil {
		return errors.New("Item already exists")
	}

	newApplication := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "applicationconnector.kyma-project.io/v1alpha1",
			"kind":       "Application",
			"metadata": map[string]interface{}{
				"name": name,
			},
		},
	}

	_, err = kube.Dynamic().Resource(applicationRes).Create(newApplication, metav1.CreateOptions{})
	if err != nil {
		return errors.Wrap(err, "Failed to create application.")
	}

	return nil
}
