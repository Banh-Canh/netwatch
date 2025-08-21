// internal/controller/cleanup_controller.go
package controller

import (
	"context"
	"fmt"

	vtkiov1alpha1 "github.com/Banh-Canh/maxtac/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/Banh-Canh/netwatch/internal/utils/logger"
)

const finalizerName = "netwatch.vtk.io/cleanup-finalizer"

// NetwatchCleanupReconciler reconciles Access and ExternalAccess objects for cleanup.
type NetwatchCleanupReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// Reconcile is the main loop that manages cleanup of associated resources.
func (r *NetwatchCleanupReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	// Try to fetch Access resource first
	access := &vtkiov1alpha1.Access{}
	err := r.Get(ctx, req.NamespacedName, access)
	if err == nil {
		return r.handleCleanup(ctx, access)
	}
	if !errors.IsNotFound(err) {
		logger.Logger.Error("failed to get Access resource", "error", err, "name", req.Name, "namespace", req.Namespace)
		return reconcile.Result{}, err
	}

	// If not found, try to fetch ExternalAccess resource
	extAccess := &vtkiov1alpha1.ExternalAccess{}
	err = r.Get(ctx, req.NamespacedName, extAccess)
	if err == nil {
		return r.handleCleanup(ctx, extAccess)
	}
	if !errors.IsNotFound(err) {
		logger.Logger.Error("failed to get ExternalAccess resource", "error", err, "name", req.Name, "namespace", req.Namespace)
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

// handleCleanup contains the generic logic for cleaning up based on an object's labels.
func (r *NetwatchCleanupReconciler) handleCleanup(ctx context.Context, obj client.Object) (reconcile.Result, error) {
	log := logger.Logger.With("resource", obj.GetName(), "namespace", obj.GetNamespace())

	if obj.GetDeletionTimestamp().IsZero() {
		if !controllerutil.ContainsFinalizer(obj, finalizerName) {
			controllerutil.AddFinalizer(obj, finalizerName)
			if err := r.Update(ctx, obj); err != nil {
				log.Error("failed to add finalizer", "error", err)
				return reconcile.Result{}, err
			}
			log.Info("Finalizer added to resource.")
		}
		return reconcile.Result{}, nil
	}

	if controllerutil.ContainsFinalizer(obj, finalizerName) {
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

		controllerutil.RemoveFinalizer(obj, finalizerName)
		if err := r.Update(ctx, obj); err != nil {
			// Handle the race condition where the object is already deleted.
			// If the update fails because the object is not found, it means another
			// controller (likely the maxtac controller) removed the last finalizer and
			// the object was garbage collected. In this case, our work is done.
			if errors.IsNotFound(err) {
				log.Info("Resource not found after cleanup, assuming it was already deleted. Finalizer removal is complete.")
				return reconcile.Result{}, nil
			}
			// For any other type of error, we should log it and requeue.
			log.Error("failed to remove finalizer", "error", err)
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

// SetupWithManager sets up the controller with the Manager.
func (r *NetwatchCleanupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&vtkiov1alpha1.Access{}).
		Watches(
			&vtkiov1alpha1.ExternalAccess{},
			&handler.EnqueueRequestForObject{},
		).
		Complete(r)
}
