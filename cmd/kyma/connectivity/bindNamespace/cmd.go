package bindNamespace

import (
	"strings"

	"github.com/kyma-project/cli/cmd/kyma/connectivity"
	"github.com/kyma-project/cli/internal/cli"
	"github.com/kyma-project/cli/internal/kube"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
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
		Use:   "bind-namespace",
		Short: "Creates a new Token for an Application",
		Long:  `Use this command to create a new token for an application`,
		RunE:  func(_ *cobra.Command, _ []string) error { return c.Run() },
	}

	cmd.Flags().StringVarP(&o.Name, "name", "n", "", "Name of application to create the token for")
	cmd.Flags().StringVar(&o.Namespace, "namespace", "", "Namespace to bind")
	cmd.Flags().BoolVar(&o.CreateIfNotExisting, "create", false, "Create the namespace if not existing")
	cmd.Flags().BoolVar(&o.IgnoreIfExisting, "ignore-if-existing", false, "This flags ignores it silently, if the application already exists ")

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

	err = bindNamespace(cmd.opts.Name, cmd.opts.Namespace, cmd.opts.CreateIfNotExisting, cmd.opts.IgnoreIfExisting, cmd.K8s)
	if err != nil {
		return err
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

func bindNamespace(name string, namespace string, createIfNotExisting bool, ignoreIfExisting bool, kube kube.KymaKube) error {

	exists, err := connectivity.ApplicationExists(name, kube)
	if err != nil {
		return errors.Wrap(err, "Failed to request Application")
	}

	if !exists {
		return errors.New("Application does not exist")
	}

	exists, err = connectivity.NamespaceExists(namespace, kube)
	if err != nil {
		return errors.Wrap(err, "Failed to request Namespace")
	}

	if !exists {
		if createIfNotExisting {
			nsRes := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "namespaces"}
			newNs := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "Namespace",
					"metadata": map[string]interface{}{
						"name": namespace,
					},
				},
			}
			_, err := kube.Dynamic().Resource(nsRes).Create(newNs, metav1.CreateOptions{})
			if err != nil {
				return errors.Wrap(err, "Failed to create Token, application does not exist")
			}
		} else {
			return errors.New("Namespace does not exist")
		}
	}

	exists, err = connectivity.ApplicationMappingExists(name, namespace, kube)
	if err != nil {
		return errors.Wrap(err, "Failed to request Namespace")
	}

	if !exists {
		applicationMappingRes := schema.GroupVersionResource{Group: "applicationconnector.kyma-project.io", Version: "v1alpha1", Resource: "applicationmappings"}
		newApplicationMapping := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "applicationconnector.kyma-project.io/v1alpha1",
				"kind":       "ApplicationMapping",
				"metadata": map[string]interface{}{
					"name": name,
				},
			},
		}

		_, err = kube.Dynamic().Resource(applicationMappingRes).Namespace(namespace).Create(newApplicationMapping, metav1.CreateOptions{})
		if err != nil {
			return errors.Wrap(err, "Failed to create applicationMapping.")
		}
	} else {
		if ignoreIfExisting {
			return nil
		} else {
			return errors.New("Mapping already exists")
		}
	}

	return nil
}
