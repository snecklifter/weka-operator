package controllers

import (
	"context"
	"time"

	"github.com/weka/go-weka-observability/instrumentation"
	weka "github.com/weka/weka-k8s-api/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
)

// configurationPolicyStatus marks a configuration policy as carrying live settings.
//
// Deliberately not "Done": DurationTillNext keys off that value and would then requeue the policy
// every spec.payload.interval forever, for a policy that never runs.
const configurationPolicyStatus = "Active"

// reconcileConfiguration handles a configuration policy, which holds operator-wide settings rather
// than an operation to perform. Other reconcilers read those settings on demand through
// services.ConfigurationCache, so there is nothing to execute here.
func (r *WekaPolicyReconciler) reconcileConfiguration(ctx context.Context, wekaPolicy *weka.WekaPolicy) (ctrl.Result, error) {
	ctx, logger := instrumentation.CreateLogSpan(ctx, "WekaPolicyReconcileConfiguration")
	defer logger.End()

	// Only write when the value actually changes: an unconditional status update would trip this
	// controller's own watch and spin.
	if wekaPolicy.Status.Status == configurationPolicyStatus {
		return ctrl.Result{}, nil
	}

	wekaPolicy.Status.Status = configurationPolicyStatus
	if wekaPolicy.Status.LastRunTime.IsZero() {
		// lastRunTime is required by the CRD and a zero metav1.Time marshals to null, which the
		// API server rejects. A configuration policy never runs, so backdate it the same way the
		// operation path does; DurationTillNext only schedules on status "Done", so this does not
		// cause a requeue.
		wekaPolicy.Status.LastRunTime = metav1.NewTime(time.Now().Add(-time.Hour))
	}
	if err := r.Status().Update(ctx, wekaPolicy); err != nil {
		logger.Error(err, "Failed to update configuration WekaPolicy status")
		return ctrl.Result{}, err
	}

	logger.Info("Configuration policy active")
	return ctrl.Result{}, nil
}
