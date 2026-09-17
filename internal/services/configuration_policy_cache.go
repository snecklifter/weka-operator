package services

import (
	"context"
	"sync"
	"time"

	weka "github.com/weka/weka-k8s-api/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/weka/weka-operator/pkg/util"
)

// configurationCacheTTL bounds how stale the operator-wide configuration may be: the policy is
// re-read at most once per this interval, so an edit takes effect within it.
const configurationCacheTTL = 30 * time.Second

// make this service globally available
var ConfigurationCache ConfigurationCacheService

func init() {
	ConfigurationCache = NewConfigurationCacheService()
}

// DriversSettings holds the operator-wide settings for building and distributing drivers.
type DriversSettings struct {
	// ForceBuilderCli takes the weka CLI from the builder image regardless of what the cluster
	// image's feature flags report.
	ForceBuilderCli bool
}

// ConfigurationSettings holds the operator-wide settings carried by a configuration WekaPolicy.
type ConfigurationSettings struct {
	Drivers DriversSettings
}

// DefaultConfigurationSettings returns the built-in defaults, used whenever no single configuration
// policy can be resolved.
func DefaultConfigurationSettings() ConfigurationSettings {
	return ConfigurationSettings{
		Drivers: DriversSettings{
			ForceBuilderCli: false,
		},
	}
}

// SettingsFromPayload maps a configuration payload onto settings. A nil payload, a nil section or a
// nil field each keep the built-in default, so an unset field stays distinguishable from an
// explicit false.
func SettingsFromPayload(payload *weka.ConfigurationPayload) ConfigurationSettings {
	settings := DefaultConfigurationSettings()
	if payload == nil {
		return settings
	}

	if payload.Drivers != nil && payload.Drivers.ForceBuilderCli != nil {
		settings.Drivers.ForceBuilderCli = *payload.Drivers.ForceBuilderCli
	}

	return settings
}

type ConfigurationCacheService interface {
	// GetSettings returns the operator-wide configuration, refreshing it from the cluster when the
	// cached copy is older than the TTL. It never fails: any problem resolving the policy yields
	// the built-in defaults.
	GetSettings(ctx context.Context, c client.Client) ConfigurationSettings
	// Invalidate drops the cached copy so the next GetSettings re-reads.
	Invalidate()
}

type configurationCacheService struct {
	settings  ConfigurationSettings
	updatedAt time.Time
	ttl       time.Duration
	lock      sync.RWMutex
}

func NewConfigurationCacheService() ConfigurationCacheService {
	return &configurationCacheService{
		settings: DefaultConfigurationSettings(),
		ttl:      configurationCacheTTL,
	}
}

func (s *configurationCacheService) GetSettings(ctx context.Context, c client.Client) ConfigurationSettings {
	s.lock.RLock()
	if s.fresh() {
		settings := s.settings
		s.lock.RUnlock()
		return settings
	}
	s.lock.RUnlock()

	s.lock.Lock()
	defer s.lock.Unlock()

	// another goroutine may have refreshed while we waited for the write lock, so a burst of
	// reconciles costs a single list
	if s.fresh() {
		return s.settings
	}

	s.settings = resolveConfigurationSettings(ctx, c)
	s.updatedAt = time.Now()
	return s.settings
}

func (s *configurationCacheService) Invalidate() {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.updatedAt = time.Time{}
}

// fresh reports whether the cached copy is still within the TTL. Callers must hold the lock.
func (s *configurationCacheService) fresh() bool {
	return !s.updatedAt.IsZero() && time.Since(s.updatedAt) < s.ttl
}

// resolveConfigurationSettings reads the configuration policy from the operator's own namespace.
// These are install-wide settings, so a policy in a tenant namespace must not shadow them.
// A configuration policy carries no spec.type and is identified by its payload.
func resolveConfigurationSettings(ctx context.Context, c client.Client) ConfigurationSettings {
	namespace, err := util.GetPodNamespace()
	if err != nil {
		return DefaultConfigurationSettings()
	}

	policyList := &weka.WekaPolicyList{}
	if err := c.List(ctx, policyList, &client.ListOptions{Namespace: namespace}); err != nil {
		return DefaultConfigurationSettings()
	}

	var matches []*weka.ConfigurationPayload
	for i := range policyList.Items {
		if payload := policyList.Items[i].Spec.Payload.Configuration; payload != nil {
			matches = append(matches, payload)
		}
	}

	// none, or more than one and we will not guess which
	if len(matches) != 1 {
		return DefaultConfigurationSettings()
	}

	// SettingsFromPayload copies the values out, so nothing aliases the informer cache
	return SettingsFromPayload(matches[0])
}
