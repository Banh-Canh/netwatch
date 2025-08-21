// internal/k8s/externalaccess.go
package k8s

import (
	"context"

	vtkiov1alpha1 "github.com/Banh-Canh/maxtac/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func CreateExternalAccess(ctx context.Context, k8sClient client.Client, access *vtkiov1alpha1.ExternalAccess) error {
	return k8sClient.Create(ctx, access)
}

func DeleteExternalAccess(ctx context.Context, k8sClient client.Client, namespace, name string) error {
	access := &vtkiov1alpha1.ExternalAccess{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace}}
	return k8sClient.Delete(ctx, access)
}

func ListNetwatchExternalAccesses(ctx context.Context, accessList *vtkiov1alpha1.ExternalAccessList) error {
	labelSelector := labels.SelectorFromSet(map[string]string{"app.kubernetes.io/managed-by": "netwatch"})
	listOptions := &client.ListOptions{LabelSelector: labelSelector}
	return appKubeClient.List(ctx, accessList, listOptions)
}
