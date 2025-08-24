// internal/k8s/access.go
package k8s

import (
	"context"
	"fmt"

	vtkiov1alpha1 "github.com/Banh-Canh/maxtac/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func CreateAccess(ctx context.Context, k8sClient client.Client, access *vtkiov1alpha1.Access) error {
	return k8sClient.Create(ctx, access)
}

func DeleteAccess(ctx context.Context, k8sClient client.Client, namespace, name string) error {
	access := &vtkiov1alpha1.Access{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace}}
	return k8sClient.Delete(ctx, access)
}

func GetAccessAsUser(ctx context.Context, k8sClient client.Client, namespace, name string) (*vtkiov1alpha1.Access, error) {
	access := &vtkiov1alpha1.Access{}
	if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, access); err != nil {
		return nil, err
	}
	return access, nil
}

func UpdateAccess(ctx context.Context, k8sClient client.Client, access *vtkiov1alpha1.Access) error {
	return k8sClient.Update(ctx, access)
}

func ListAllAccessesWithLabelAsApp(ctx context.Context, reqID string) (*vtkiov1alpha1.AccessList, error) {
	var accessList vtkiov1alpha1.AccessList
	listOpts := []client.ListOption{
		client.MatchingLabels{"netwatch.vtk.io/request-id": reqID},
	}
	if err := appKubeClient.List(ctx, &accessList, listOpts...); err != nil {
		return nil, fmt.Errorf("failed to list accesses with app client: %w", err)
	}
	return &accessList, nil
}

func ListNetwatchAccesses(ctx context.Context, accessList *vtkiov1alpha1.AccessList) error {
	labelSelector := labels.SelectorFromSet(map[string]string{"app.kubernetes.io/managed-by": "netwatch"})
	listOptions := &client.ListOptions{LabelSelector: labelSelector}
	return appKubeClient.List(ctx, accessList, listOptions)
}

// GetAccessAsApp fetches an Access resource using the privileged application client.
func GetAccessAsApp(ctx context.Context, namespace, name string) (*vtkiov1alpha1.Access, error) {
	access := &vtkiov1alpha1.Access{}
	if err := appKubeClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, access); err != nil {
		return nil, err
	}
	return access, nil
}

func UpdateAccessWithRetry(
	ctx context.Context,
	k8sClient client.Client,
	namespace, name string,
	updateFunc func(*vtkiov1alpha1.Access),
) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// 1. Always fetch the latest version of the object within the loop.
		access, err := GetAccessAsUser(ctx, k8sClient, namespace, name)
		if err != nil {
			return err
		}

		// 2. Apply the desired changes to the object.
		updateFunc(access)

		// 3. Attempt to update. If the object was modified, RetryOnConflict will try again.
		return k8sClient.Update(ctx, access)
	})
}
