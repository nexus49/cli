package bindNamespace

import (
	"strings"

	"github.com/kyma-project/cli/cmd/kyma/connectivity"
	"github.com/kyma-project/cli/internal/cli"
	"github.com/kyma-project/cli/internal/kube"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
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
		RunE:  func(_ *cobra.Command, args []string) error { return c.Run(args) },
	}

	cmd.Args = cobra.ExactArgs(1)
	cmd.Flags().StringVarP(&o.Namespace, "namespace", "n", "", "Namespace to bind")
	cmd.Flags().BoolVar(&o.CreateIfNotExisting, "create", true, "Create the namespace if not existing")
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

	applicationName := args[0]

	var err error
	if cmd.K8s, err = kube.NewFromConfig("", cmd.KubeconfigPath); err != nil {
		return errors.Wrap(err, "Could not initialize the Kubernetes client. Make sure your kubeconfig is valid")
	}

	err = bindNamespace(applicationName, cmd.opts.Namespace, cmd.opts.CreateIfNotExisting, cmd.opts.IgnoreIfExisting, cmd.K8s)
	if err != nil {
		return err
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
		if !ignoreIfExisting {
			return errors.New("Mapping already exists")
		}
	}

	err = ensureInstances(namespace, kube)
	if err != nil {
		return errors.Wrap(err, "Failed to manage service instances")
	}

	return nil
}

func ensureInstances(namespace string, kube kube.KymaKube) error {
	classes, err := collectClassesForProvider("SAP Commerce", "application-broker", namespace, kube)
	if err != nil {
		return errors.Wrap(err, "Failed to collect service classes")
	}

	instances, err := collectServiceInstances(namespace, kube)
	if err != nil {
		return err
	}

	for _, cl := range classes {
		scName := cl.Object["metadata"].(map[string]interface{})["name"].(string)
		spec := cl.Object["spec"].(map[string]interface{})
		description := spec["description"].(string)
		scExternalName := spec["externalName"].(string)
		if !hasInstance(scName, instances) {
			log.Infof("Creating service instance for, %s", description)
			newSci := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "servicecatalog.k8s.io/v1beta1",
					"kind":       "ServiceInstance",
					"metadata": map[string]interface{}{
						"generateName": scExternalName,
					},
					"spec": map[string]interface{}{
						"serviceClassExternalName": scExternalName,
						"servicePlanExternalName":  "default",
					},
				},
			}
			sciRes := schema.GroupVersionResource{Group: "servicecatalog.k8s.io", Version: "v1beta1", Resource: "serviceinstances"}
			_, err := kube.Dynamic().Resource(sciRes).Namespace(namespace).Create(newSci, metav1.CreateOptions{})
			if err != nil {
				return errors.Wrap(err, "Failed to create Token, application does not exist")
			}

		} else {
			log.Infof("Instance exists for service class %s", description)
		}
	}

	return nil
}

func hasInstance(name string, instances *unstructured.UnstructuredList) bool {
	for _, instance := range instances.Items {
		spec := instance.Object["spec"].(map[string]interface{})
		serviceClassRef := spec["serviceClassRef"].(map[string]interface{})
		scName := serviceClassRef["name"].(string)

		if scName == name {
			return true
		}
	}

	return false
}

func collectServiceInstances(namespace string, kube kube.KymaKube) (*unstructured.UnstructuredList, error) {
	scRes := schema.GroupVersionResource{Group: "servicecatalog.k8s.io", Version: "v1beta1", Resource: "serviceinstances"}
	return kube.Dynamic().Resource(scRes).Namespace(namespace).List(metav1.ListOptions{})
}

func collectClassesForProvider(provider string, broker string, namespace string, kube kube.KymaKube) ([]unstructured.Unstructured, error) {
	classes := []unstructured.Unstructured{}
	// iterate over existing service classes
	scRes := schema.GroupVersionResource{Group: "servicecatalog.k8s.io", Version: "v1beta1", Resource: "serviceclasses"}
	result, err := kube.Dynamic().Resource(scRes).Namespace(namespace).List(metav1.ListOptions{})
	if err != nil {
		return classes, errors.Wrap(err, "Failed to list serviceClasses.")
	}
	for _, sc := range result.Items {
		spec := sc.Object["spec"].(map[string]interface{})
		serviceBrokerName := spec["serviceBrokerName"].(string)
		externalMetadata := spec["externalMetadata"].(map[string]interface{})

		providerDisplayName := externalMetadata["providerDisplayName"].(string)
		if providerDisplayName == "SAP Commerce" && serviceBrokerName == "application-broker" {
			classes = append(classes, sc)
		}
	}
	return classes, nil
}
