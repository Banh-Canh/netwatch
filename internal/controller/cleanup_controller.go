package controller

import (
	"context"
	"fmt"
	"strings"

	vtkiov1alpha1 "github.com/Banh-Canh/maxtac/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	netwatchv1alpha1 "github.com/Banh-Canh/netwatch/api/v1alpha1"
	"github.com/Banh-Canh/netwatch/internal/utils/logger"
)

const (
	accessFinalizerName        = "netwatch.vtk.io/access-cleanup-finalizer"
	accessRequestFinalizerName = "netwatch.vtk.io/request-cleanup-finalizer"
)

// NetwatchCleanupReconciler reconciles all Netwatch-related resources for cleanup.
type NetwatchCleanupReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// Reconcile is the main loop that determines which resource type triggered the event and acts accordingly.
func (r *NetwatchCleanupReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	// Priority 1: Check if it's an AccessRequest event.
	accessRequest := &netwatchv1alpha1.AccessRequest{}
	if err := r.Get(ctx, req.NamespacedName, accessRequest); err == nil {
		return r.reconcileAccessRequest(ctx, accessRequest)
	} else if !errors.IsNotFound(err) {
		logger.Logger.Error("failed to get AccessRequest resource", "error", err, "name", req.Name)
		return reconcile.Result{}, err
	}

	// Priority 2: Check if it's an Access event.
	access := &vtkiov1alpha1.Access{}
	if err := r.Get(ctx, req.NamespacedName, access); err == nil {
		return r.reconcileAccessFinalizer(ctx, access)
	} else if !errors.IsNotFound(err) {
		logger.Logger.Error("failed to get Access resource", "error", err, "name", req.Name, "namespace", req.Namespace)
		return reconcile.Result{}, err
	}

	// Priority 3: Check if it's an ExternalAccess event.
	extAccess := &vtkiov1alpha1.ExternalAccess{}
	if err := r.Get(ctx, req.NamespacedName, extAccess); err == nil {
		return r.reconcileAccessFinalizer(ctx, extAccess)
	} else if !errors.IsNotFound(err) {
		logger.Logger.Error("failed to get ExternalAccess resource", "error", err, "name", req.Name, "namespace", req.Namespace)
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

// reconcileAccessRequest handles the lifecycle of an AccessRequest, including cleanup of its own partial resources.
func (r *NetwatchCleanupReconciler) reconcileAccessRequest(
	ctx context.Context,
	request *netwatchv1alpha1.AccessRequest,
) (reconcile.Result, error) {
	log := logger.Logger.With("resource", request.Name)

	// --- Finalizer Logic for AccessRequest ---
	if request.ObjectMeta.DeletionTimestamp.IsZero() {
		// The object is not being deleted, so if it does not have our finalizer,
		// let's add it and update the object.
		if !controllerutil.ContainsFinalizer(request, accessRequestFinalizerName) {
			controllerutil.AddFinalizer(request, accessRequestFinalizerName)
			if err := r.Update(ctx, request); err != nil {
				return reconcile.Result{}, err
			}
		}
	} else {
		// The object is being deleted.
		if controllerutil.ContainsFinalizer(request, accessRequestFinalizerName) {
			// Our finalizer is present, so let's handle any external dependency cleanup.
			if err := r.cleanupPartialAccess(ctx, request); err != nil {
				// if fail to delete the external dependency here, return with error
				// so that it can be retried.
				log.Error("Failed during partial access cleanup, will retry", "error", err)
				return reconcile.Result{}, err
			}

			// remove our finalizer from the list and update it.
			controllerutil.RemoveFinalizer(request, accessRequestFinalizerName)
			if err := r.Update(ctx, request); err != nil {
				return reconcile.Result{}, err
			}
		}
		// Stop reconciliation as the item is being deleted
		return reconcile.Result{}, nil
	}

	// --- Orphan Check Logic ---
	// This logic only applies to requests that are partially completed.
	if request.Spec.Status != "PendingTarget" && request.Spec.Status != "PendingSource" {
		return reconcile.Result{}, nil
	}

	var cloneName, accessName, namespace string
	if request.Spec.Status == "PendingTarget" {
		cloneName = request.Spec.SourceCloneName
		namespace = strings.Split(request.Spec.SourceService, "/")[0]
	} else { // PendingSource
		cloneName = request.Spec.TargetCloneName
		namespace = strings.Split(request.Spec.TargetService, "/")[0]
	}
	accessName = fmt.Sprintf("access-%s", cloneName)

	var existingAccess vtkiov1alpha1.Access
	err := r.Get(ctx, client.ObjectKey{Namespace: namespace, Name: accessName}, &existingAccess)

	if err != nil && errors.IsNotFound(err) {
		// The Access object is GONE. This AccessRequest is an orphan and must be deleted.
		log.Info("Orphaned partial access request found (Access object missing), cleaning up...", "status", request.Spec.Status)
		if err := r.Delete(ctx, request); err != nil {
			log.Error("Failed to delete orphaned AccessRequest", "error", err)
			return reconcile.Result{}, err
		}
		log.Info("Successfully deleted orphaned AccessRequest.")
	} else if err != nil {
		// A real error occurred (e.g., RBAC). Requeue the request.
		log.Error("Failed to check for orphaned access request", "error", err)
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

// cleanupPartialAccess finds and deletes the single Access object associated with a deleted AccessRequest.
func (r *NetwatchCleanupReconciler) cleanupPartialAccess(ctx context.Context, request *netwatchv1alpha1.AccessRequest) error {
	log := logger.Logger.With("resource", request.Name)

	if request.Spec.Status != "PendingTarget" && request.Spec.Status != "PendingSource" {
		// This is a PendingFull request or already completed, no partial resources to clean up.
		return nil
	}

	log.Info("AccessRequest is being deleted, cleaning up associated partial access...")

	var cloneName, namespace string
	if request.Spec.Status == "PendingTarget" {
		cloneName = request.Spec.SourceCloneName
		namespace = strings.Split(request.Spec.SourceService, "/")[0]
	} else { // PendingSource
		cloneName = request.Spec.TargetCloneName
		namespace = strings.Split(request.Spec.TargetService, "/")[0]
	}

	if cloneName == "" || namespace == "" {
		log.Warn("Partial AccessRequest is missing clone name or namespace, cannot clean up.")
		return nil
	}

	accessNameToDelete := fmt.Sprintf("access-%s", cloneName)
	accessToDelete := &vtkiov1alpha1.Access{
		ObjectMeta: metav1.ObjectMeta{
			Name:      accessNameToDelete,
			Namespace: namespace,
		},
	}

	log.Info("Deleting associated partial Access object", "name", accessNameToDelete, "namespace", namespace)
	// When we delete this Access object, its own finalizer will trigger the deletion of the Service clone.
	if err := r.Delete(ctx, accessToDelete); err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete partial access object: %w", err)
	}

	return nil
}

// reconcileAccessFinalizer contains the original logic for cleaning up Service clones based on finalizers.
func (r *NetwatchCleanupReconciler) reconcileAccessFinalizer(ctx context.Context, obj client.Object) (reconcile.Result, error) {
	log := logger.Logger.With("resource", obj.GetName(), "namespace", obj.GetNamespace())

	if obj.GetDeletionTimestamp().IsZero() {
		if !controllerutil.ContainsFinalizer(obj, accessFinalizerName) {
			controllerutil.AddFinalizer(obj, accessFinalizerName)
			if err := r.Update(ctx, obj); err != nil {
				log.Error("failed to add finalizer", "error", err)
				return reconcile.Result{}, err
			}
			log.Info("Finalizer added to resource.")
		}
		return reconcile.Result{}, nil
	}

	if controllerutil.ContainsFinalizer(obj, accessFinalizerName) {
		log.Info("Resource is being deleted, running cleanup...")
		reqID, ok := obj.GetLabels()["netwatch.vtk.io/request-id"]
		if !ok {
			log.Warn("Resource is missing request-id label, cannot perform cleanup.")
		} else {
			if err := r.deleteClonedServices(ctx, reqID); err != nil {
				log.Error("cleanup failed during service deletion", "error", err)
				return reconcile.Result{}, err
			}
		}

		controllerutil.RemoveFinalizer(obj, accessFinalizerName)
		if err := r.Update(ctx, obj); err != nil {
			if errors.IsNotFound(err) {
				log.Info("Resource not found after cleanup, assuming it was already deleted.")
				return reconcile.Result{}, nil
			}
			log.Error("Failed to remove finalizer, will retry.", "error", err)
			return reconcile.Result{}, err
		}
		log.Info("Cleanup successful, finalizer removed.")
	}

	return reconcile.Result{}, nil
}

// deleteClonedServices finds and deletes service clones with a specific request-id.
func (r *NetwatchCleanupReconciler) deleteClonedServices(ctx context.Context, reqID string) error {
	var serviceClones corev1.ServiceList
	listOpts := []client.ListOption{
		client.MatchingLabels{"netwatch.vtk.io/request-id": reqID},
	}

	if err := r.List(ctx, &serviceClones, listOpts...); err != nil {
		return fmt.Errorf("failed to list service clones for cleanup: %w", err)
	}

	for _, service := range serviceClones.Items {
		log := logger.Logger.With("request-id", reqID)
		log.Info("Deleting associated service clone", "service", service.Name, "namespace", service.Namespace)
		if err := r.Delete(ctx, &service); err != nil && !errors.IsNotFound(err) {
			log.Error("failed to delete service clone", "error", err, "service", service.Name, "namespace", service.Namespace)
		}
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager to watch all relevant resources.
func (r *NetwatchCleanupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &netwatchv1alpha1.AccessRequest{}, "spec.requestID", func(rawObj client.Object) []string {
		req := rawObj.(*netwatchv1alpha1.AccessRequest)
		if req.Spec.RequestID == "" {
			return nil
		}
		return []string{req.Spec.RequestID}
	}); err != nil {
		return err
	}

	mapAccessToAccessRequest := handler.EnqueueRequestsFromMapFunc(
		func(ctx context.Context, o client.Object) []reconcile.Request {
			reqID, ok := o.GetLabels()["netwatch.vtk.io/request-id"]
			if !ok {
				return nil
			}

			var accessRequestList netwatchv1alpha1.AccessRequestList
			if err := r.List(ctx, &accessRequestList, client.MatchingFields{"spec.requestID": reqID}); err != nil {
				logger.Logger.Error("Failed to list AccessRequests for mapping", "error", err)
				return nil
			}

			if len(accessRequestList.Items) > 0 {
				return []reconcile.Request{
					{NamespacedName: types.NamespacedName{Name: accessRequestList.Items[0].Name}},
				}
			}
			return nil
		},
	)

	return ctrl.NewControllerManagedBy(mgr).
		For(&netwatchv1alpha1.AccessRequest{}).
		Watches(
			&vtkiov1alpha1.Access{},
			&handler.EnqueueRequestForObject{},
		).
		Watches(
			&vtkiov1alpha1.ExternalAccess{},
			&handler.EnqueueRequestForObject{},
		).
		Watches(
			&vtkiov1alpha1.Access{},
			mapAccessToAccessRequest,
		).
		Complete(r)
}
