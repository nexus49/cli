package createApplication

import (
	"strings"

	"github.com/google/martian/log"
	"github.com/kyma-project/cli/cmd/kyma/connectivity"
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
	args []string
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
		RunE:  func(_ *cobra.Command, args []string) error { return c.Run(args) },
	}
	cmd.Args = cobra.ExactArgs(1)
	cmd.Flags().BoolVar(&o.IgnoreIfExisting, "ignore-if-existing", false, "This flags ignores it silently, if the application already exists ")
	return cmd
}

func (cmd *command) Run(args []string) error {
	if err := cmd.validateFlags(); err != nil {
		return err
	}

	if err := cmd.validateArgs(args); err != nil {
		return err
	}

	name := args[0]

	var err error
	if cmd.K8s, err = kube.NewFromConfig("", cmd.KubeconfigPath); err != nil {
		return errors.Wrap(err, "Could not initialize the Kubernetes client. Make sure your kubeconfig is valid")
	}

	err = createApplication(name, cmd.opts.IgnoreIfExisting, cmd.K8s)
	if err != nil {
		return errors.Wrap(err, "Could not create Application")
	}
	return nil
}

func (c *command) validateArgs(args []string) error {
	var errMessage strings.Builder
	// mandatory flags
	if len(args) != 1 {
		errMessage.WriteString("\nRequired argument `name` has not been set.")
	}

	if errMessage.Len() != 0 {
		return errors.New(errMessage.String())
	}
	return nil
}

func (c *command) validateFlags() error {
	var errMessage strings.Builder
	// mandatory flags
	// if c.opts.Name == "" {
	// 	errMessage.WriteString("\nRequired flag `name` has not been set.")
	// }

	if errMessage.Len() != 0 {
		return errors.New(errMessage.String())
	}
	return nil
}

func createApplication(name string, ignoreIfExisting bool, kube kube.KymaKube) error {
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
		if ignoreIfExisting {
			return nil
		} else {
			return errors.New("Item already exists")
		}
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

	err = connectivity.WaitForDeployed(name, nil, 15, "applications", kube)
	if err != nil {
		return errors.Wrap(err, "Failed to wait for application deployment.")
	}
	log.Infof("Application %s created successfully.", name)

	return nil
}
