package controllers

import (
	"context"
	"testing"

	weka "github.com/weka/weka-k8s-api/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func configurationReconciler(t *testing.T, policy *weka.WekaPolicy) *WekaPolicyReconciler {
	t.Helper()

	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatalf("add client-go scheme: %v", err)
	}
	if err := weka.AddToScheme(scheme); err != nil {
		t.Fatalf("add weka scheme: %v", err)
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(policy).
		WithStatusSubresource(&weka.WekaPolicy{}).
		Build()

	return &WekaPolicyReconciler{Client: c, Scheme: scheme}
}

func newConfigurationPolicy() *weka.WekaPolicy {
	return &weka.WekaPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "operator-configuration", Namespace: "weka-operator-system"},
		Spec: weka.WekaPolicySpec{
			Payload: weka.PolicyPayload{
				Configuration: &weka.ConfigurationPayload{},
			},
		},
	}
}

// The CRD marks status.lastRunTime required, and a zero metav1.Time marshals to null. Leaving it
// unset makes the API server reject the status update, which returns an error from Reconcile and
// puts the policy in a permanent retry loop - observed live before this was fixed.
func TestReconcileConfigurationSetsRequiredStatusFields(t *testing.T) {
	policy := newConfigurationPolicy()
	r := configurationReconciler(t, policy)
	ctx := context.Background()

	if _, err := r.reconcileConfiguration(ctx, policy); err != nil {
		t.Fatalf("reconcileConfiguration: %v", err)
	}

	stored := &weka.WekaPolicy{}
	key := types.NamespacedName{Name: policy.Name, Namespace: policy.Namespace}
	if err := r.Get(ctx, key, stored); err != nil {
		t.Fatalf("get policy: %v", err)
	}

	if stored.Status.Status != configurationPolicyStatus {
		t.Errorf("status = %q, want %q", stored.Status.Status, configurationPolicyStatus)
	}
	if stored.Status.LastRunTime.IsZero() {
		t.Error("lastRunTime is zero; it is required by the CRD and null is rejected by the API server")
	}
}

// A second pass must not write again: an unconditional status update trips the controller's own
// watch and spins.
func TestReconcileConfigurationIsIdempotent(t *testing.T) {
	policy := newConfigurationPolicy()
	r := configurationReconciler(t, policy)
	ctx := context.Background()

	if _, err := r.reconcileConfiguration(ctx, policy); err != nil {
		t.Fatalf("first reconcile: %v", err)
	}

	stored := &weka.WekaPolicy{}
	key := types.NamespacedName{Name: policy.Name, Namespace: policy.Namespace}
	if err := r.Get(ctx, key, stored); err != nil {
		t.Fatalf("get policy: %v", err)
	}
	firstVersion := stored.ResourceVersion

	if _, err := r.reconcileConfiguration(ctx, stored); err != nil {
		t.Fatalf("second reconcile: %v", err)
	}

	again := &weka.WekaPolicy{}
	if err := r.Get(ctx, key, again); err != nil {
		t.Fatalf("get policy again: %v", err)
	}
	if again.ResourceVersion != firstVersion {
		t.Errorf("resourceVersion changed from %s to %s; the second pass wrote status again",
			firstVersion, again.ResourceVersion)
	}
}
