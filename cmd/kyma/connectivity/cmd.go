package connectivity

import (
	"github.com/kyma-project/cli/internal/kube"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
