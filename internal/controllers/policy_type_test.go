package controllers

import (
	"testing"

	weka "github.com/weka/weka-k8s-api/api/v1alpha1"
)

func TestResolvePolicyType(t *testing.T) {
	cases := []struct {
		name    string
		spec    weka.WekaPolicySpec
		want    weka.WekaPolicyType
		wantErr bool
	}{
		{
			name: "explicit type wins over an unrelated payload",
			spec: weka.WekaPolicySpec{
				Type:    weka.WekaPolicyTypeEnsureNICs,
				Payload: weka.PolicyPayload{SignDrives: &weka.SignDrivesPayload{}},
			},
			want: weka.WekaPolicyTypeEnsureNICs,
		},
		{
			name: "explicit type with no payload at all",
			spec: weka.WekaPolicySpec{Type: weka.WekaPolicyTypeDiscoverDrives},
			want: weka.WekaPolicyTypeDiscoverDrives,
		},
		{
			name: "infer sign-drives",
			spec: weka.WekaPolicySpec{Payload: weka.PolicyPayload{SignDrives: &weka.SignDrivesPayload{}}},
			want: weka.WekaPolicyTypeSignDrives,
		},
		{
			name: "infer discover-drives",
			spec: weka.WekaPolicySpec{Payload: weka.PolicyPayload{DiscoverDrives: &weka.DiscoverDrivesPayload{}}},
			want: weka.WekaPolicyTypeDiscoverDrives,
		},
		{
			name: "infer ensure-nics",
			spec: weka.WekaPolicySpec{Payload: weka.PolicyPayload{EnsureNICs: &weka.EnsureNICsPayload{}}},
			want: weka.WekaPolicyTypeEnsureNICs,
		},
		{
			name: "infer enable-local-drivers-distribution",
			spec: weka.WekaPolicySpec{Payload: weka.PolicyPayload{DriverDistPayload: &weka.DriverDistPayload{}}},
			want: weka.WekaPolicyTypeEnableLocalDriversDistribution,
		},
		{
			name: "infer remote-traces-session",
			spec: weka.WekaPolicySpec{Payload: weka.PolicyPayload{RemoteTracesSession: &weka.RemoteTracesSessionConfig{}}},
			want: weka.WekaPolicyTypeRemoteTracesSession,
		},
		{
			name: "infer clean-stale-virtual-drives",
			spec: weka.WekaPolicySpec{Payload: weka.PolicyPayload{CleanStaleVirtualDrives: &weka.CleanStaleVirtualDrivesPayload{}}},
			want: weka.WekaPolicyTypeCleanStaleVirtualDrives,
		},
		{
			name:    "no type and no payload is undeterminable",
			spec:    weka.WekaPolicySpec{},
			wantErr: true,
		},
		{
			name: "two payloads are ambiguous, not a guess",
			spec: weka.WekaPolicySpec{Payload: weka.PolicyPayload{
				SignDrives:     &weka.SignDrivesPayload{},
				DiscoverDrives: &weka.DiscoverDrivesPayload{},
			}},
			wantErr: true,
		},
		{
			name:    "schedulingConfig carries no type, so it is not a signal",
			spec:    weka.WekaPolicySpec{Payload: weka.PolicyPayload{SchedulingConfig: &weka.SchedulingConfigPayload{}}},
			wantErr: true,
		},
		{
			name: "interval alone is not a signal",
			spec: weka.WekaPolicySpec{Payload: weka.PolicyPayload{
				WaitForPolicies: []string{"some-policy"},
			}},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ResolvePolicyType(&tc.spec)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got type %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("type = %q, want %q", got, tc.want)
			}
		})
	}
}

// A configuration policy is inert and never reaches ResolvePolicyType, but if the branch that
// intercepts it were ever removed, inference must not silently map it onto a runnable type.
func TestResolvePolicyTypeRejectsConfigurationPayload(t *testing.T) {
	spec := weka.WekaPolicySpec{Payload: weka.PolicyPayload{
		Configuration: &weka.ConfigurationPayload{},
	}}

	if got, err := ResolvePolicyType(&spec); err == nil {
		t.Fatalf("expected an error for a configuration-only payload, got type %q", got)
	}
}
