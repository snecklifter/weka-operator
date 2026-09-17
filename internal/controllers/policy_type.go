package controllers

import (
	"fmt"
	"sort"
	"strings"

	weka "github.com/weka/weka-k8s-api/api/v1alpha1"
)

// inferredPolicyType names one runnable payload and the type it implies. spec.type is optional, so
// a policy that sets exactly one of these is dispatched on that payload alone.
//
// SchedulingConfig is deliberately absent: it has no corresponding WekaPolicyType, so it is neither
// inferable nor dispatchable. Configuration is absent too - it is inert settings, not an operation,
// and is handled before the type is ever resolved.
type inferredPolicyType struct {
	field   string
	typ     weka.WekaPolicyType
	present func(*weka.PolicyPayload) bool
}

var inferredPolicyTypes = []inferredPolicyType{
	{
		field:   "signDrivesPayload",
		typ:     weka.WekaPolicyTypeSignDrives,
		present: func(p *weka.PolicyPayload) bool { return p.SignDrives != nil },
	},
	{
		field:   "discoverDrivesPayload",
		typ:     weka.WekaPolicyTypeDiscoverDrives,
		present: func(p *weka.PolicyPayload) bool { return p.DiscoverDrives != nil },
	},
	{
		field:   "ensureNICsPayload",
		typ:     weka.WekaPolicyTypeEnsureNICs,
		present: func(p *weka.PolicyPayload) bool { return p.EnsureNICs != nil },
	},
	{
		field:   "driverDistPayload",
		typ:     weka.WekaPolicyTypeEnableLocalDriversDistribution,
		present: func(p *weka.PolicyPayload) bool { return p.DriverDistPayload != nil },
	},
	{
		field:   "remoteTracesSessionPayload",
		typ:     weka.WekaPolicyTypeRemoteTracesSession,
		present: func(p *weka.PolicyPayload) bool { return p.RemoteTracesSession != nil },
	},
	{
		field:   "cleanStaleVirtualDrivesPayload",
		typ:     weka.WekaPolicyTypeCleanStaleVirtualDrives,
		present: func(p *weka.PolicyPayload) bool { return p.CleanStaleVirtualDrives != nil },
	},
}

// ResolvePolicyType returns the effective type of a policy.
//
// An explicit spec.type always wins, so existing policies keep dispatching exactly as before.
// Otherwise the type is inferred from whichever payload is populated. Anything other than a single
// runnable payload is an error rather than a guess: the user can always disambiguate by setting
// spec.type.
func ResolvePolicyType(spec *weka.WekaPolicySpec) (weka.WekaPolicyType, error) {
	if spec.Type != "" {
		return spec.Type, nil
	}

	var found []inferredPolicyType
	for _, candidate := range inferredPolicyTypes {
		if candidate.present(&spec.Payload) {
			found = append(found, candidate)
		}
	}

	switch len(found) {
	case 1:
		return found[0].typ, nil
	case 0:
		return "", fmt.Errorf("cannot determine policy type: spec.type is unset and no recognized payload is set; set spec.type explicitly or provide one of %s", strings.Join(inferrableFields(), ", "))
	default:
		set := make([]string, 0, len(found))
		for _, candidate := range found {
			set = append(set, candidate.field)
		}
		sort.Strings(set)
		return "", fmt.Errorf("cannot determine policy type: spec.type is unset and %d payloads are set (%s); set spec.type explicitly or provide exactly one payload", len(found), strings.Join(set, ", "))
	}
}

func inferrableFields() []string {
	fields := make([]string, 0, len(inferredPolicyTypes))
	for _, candidate := range inferredPolicyTypes {
		fields = append(fields, candidate.field)
	}
	return fields
}
