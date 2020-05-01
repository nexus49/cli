package connectivity

import (
	"time"

	"github.com/kyma-project/cli/internal/kube"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

//NewCmd creates a new provision command
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "connectivity",
		Short: "Manages connected applications",
	}
	return cmd
}

func ApplicationExists(name string, kube kube.KymaKube) (bool, error) {
	// Verify if application exists
	res := schema.GroupVersionResource{Group: "applicationconnector.kyma-project.io", Version: "v1alpha1", Resource: "applications"}
	_, err := kube.Dynamic().Resource(res).Get(name, metav1.GetOptions{})
	if err != nil {
		if k8sErrors.IsNotFound(err) {
			return false, nil
		}
		return false, errors.Wrap(err, "Cannot request application")
	}
	return true, nil
}

func NamespaceExists(name string, kube kube.KymaKube) (bool, error) {
	// Verify if application exists
	res := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "namespaces"}
	_, err := kube.Dynamic().Resource(res).Get(name, metav1.GetOptions{})
	if err != nil {
		if k8sErrors.IsNotFound(err) {
			return false, nil
		}
		return false, errors.Wrap(err, "Cannot request application")
	}
	return true, nil
}

func ApplicationMappingExists(name string, namespace string, kube kube.KymaKube) (bool, error) {
	// Verify if application exists
	res := schema.GroupVersionResource{Group: "applicationconnector.kyma-project.io", Version: "v1alpha1", Resource: "applicationmappings"}
	_, err := kube.Dynamic().Resource(res).Namespace(namespace).Get(name, metav1.GetOptions{})
	if err != nil {
		if k8sErrors.IsNotFound(err) {
			return false, nil
		}
		return false, errors.Wrap(err, "Cannot request application mapping")
	}
	return true, nil
}

func WaitForDeployed(name string, namespace *string, maxRetries int, resource string, kube kube.KymaKube) error {

	applicationRes := schema.GroupVersionResource{Group: "applicationconnector.kyma-project.io", Version: "v1alpha1", Resource: resource}

	var itm *unstructured.Unstructured
	var err error
	if namespace != nil {
		itm, err = kube.Dynamic().Resource(applicationRes).Namespace(*namespace).Get(name, metav1.GetOptions{})
	} else {
		itm, err = kube.Dynamic().Resource(applicationRes).Get(name, metav1.GetOptions{})
	}

	if err != nil {
		if !k8sErrors.IsNotFound(err) {
			return errors.Wrap(err, "Failed to check Application")
		} else {
			return errors.Wrap(err, "Resource does not exist")
		}
	}

	if status, ok := itm.Object["status"]; ok {
		statusObj := status.(map[string]interface{})
		installationStatus := statusObj["installationStatus"].(map[string]interface{})
		installationStatusStatus := installationStatus["status"].(string)
		if installationStatusStatus == "DEPLOYED" {
			return nil
		}
	}
	time.Sleep(5 * time.Second)
	if maxRetries > 0 {
		return WaitForDeployed(name, namespace, maxRetries-1, resource, kube)
	} else {
		return errors.New("Application deployment did not end up in DEPLOYED")
	}
}
