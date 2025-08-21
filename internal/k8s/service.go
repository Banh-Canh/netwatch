// internal/k8s/service.go
package k8s

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func ListAllServices(ctx context.Context) (*corev1.ServiceList, error) {
	var serviceList corev1.ServiceList
	if err := appKubeClient.List(ctx, &serviceList); err != nil {
		return nil, err
	}
	return &serviceList, nil
}

func CloneService(
	ctx context.Context,
	k8sClient client.Client,
	originalNamespace, originalName, newName string,
	uniqueLabel map[string]string,
	overridePorts []corev1.ServicePort,
) (*corev1.Service, error) {
	var originalService corev1.Service
	if err := appKubeClient.Get(ctx, client.ObjectKey{Namespace: originalNamespace, Name: originalName}, &originalService); err != nil {
		return nil, fmt.Errorf("could not get original service %s/%s: %w", originalNamespace, originalName, err)
	}

	portsToUse := originalService.Spec.Ports
	if len(overridePorts) > 0 {
		portsToUse = overridePorts
	}

	var portStrings []string
	for _, p := range portsToUse {
		portStrings = append(portStrings, strconv.Itoa(int(p.Port)))
	}
	portsAnnotationValue := strings.Join(portStrings, ", ")

	clonedService := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      newName,
			Namespace: originalService.Namespace,
			Labels:    uniqueLabel,
			Annotations: map[string]string{
				"netwatch.vtk.io/cloned-from": fmt.Sprintf("%s/%s", originalNamespace, originalName),
				"netwatch.vtk.io/ports":       portsAnnotationValue,
			},
		},
		Spec: corev1.ServiceSpec{
			Ports:    portsToUse,
			Selector: originalService.Spec.Selector,
			Type:     originalService.Spec.Type,
		},
	}

	if clonedService.Spec.Type == corev1.ServiceTypeClusterIP {
		clonedService.Spec.ClusterIP = ""
		clonedService.Spec.ClusterIPs = nil
	}

	if err := k8sClient.Create(ctx, clonedService); err != nil {
		return nil, fmt.Errorf("could not create service clone %s/%s: %w", clonedService.Namespace, clonedService.Name, err)
	}

	return clonedService, nil
}

func DeleteService(ctx context.Context, k8sClient client.Client, namespace, name string) error {
	svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace}}
	return k8sClient.Delete(ctx, svc)
}
