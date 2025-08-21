// internal/k8s/accessrequest.go
package k8s

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	netwatchv1alpha1 "github.com/Banh-Canh/netwatch/api/v1alpha1"
)

func CreateAccessRequestAsApp(ctx context.Context, req *netwatchv1alpha1.AccessRequest) error {
	return appKubeClient.Create(ctx, req)
}

func ListAccessRequestsAsApp(ctx context.Context) (*netwatchv1alpha1.AccessRequestList, error) {
	var requestList netwatchv1alpha1.AccessRequestList
	if err := appKubeClient.List(ctx, &requestList); err != nil {
		return nil, err
	}
	return &requestList, nil
}

func GetAccessRequestAsApp(ctx context.Context, name string) (*netwatchv1alpha1.AccessRequest, error) {
	var request netwatchv1alpha1.AccessRequest
	if err := appKubeClient.Get(ctx, client.ObjectKey{Name: name}, &request); err != nil {
		return nil, err
	}
	return &request, nil
}

func DeleteAccessRequestAsApp(ctx context.Context, name string) error {
	req := &netwatchv1alpha1.AccessRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	return appKubeClient.Delete(ctx, req)
}
