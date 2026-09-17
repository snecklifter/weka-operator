package services

import (
	"context"
	"testing"
	"time"

	weka "github.com/weka/weka-k8s-api/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/weka/weka-operator/internal/config"
)

func boolPtr(b bool) *bool { return &b }

func TestSettingsFromPayload(t *testing.T) {
	cases := []struct {
		name    string
		payload *weka.ConfigurationPayload
		want    bool
	}{
		{"nil payload keeps the default", nil, false},
		{"nil drivers section keeps the default", &weka.ConfigurationPayload{}, false},
		{"nil field keeps the default", &weka.ConfigurationPayload{Drivers: &weka.DriversSpec{}}, false},
		{"explicit false", &weka.ConfigurationPayload{Drivers: &weka.DriversSpec{ForceBuilderCli: boolPtr(false)}}, false},
		{"explicit true", &weka.ConfigurationPayload{Drivers: &weka.DriversSpec{ForceBuilderCli: boolPtr(true)}}, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := SettingsFromPayload(tc.payload).Drivers.ForceBuilderCli
			if got != tc.want {
				t.Errorf("ForceBuilderCli = %v, want %v", got, tc.want)
			}
		})
	}
}

// countingClient records how many List calls reach the cluster, so a test can prove the cache
// served a read instead of re-listing.
type countingClient struct {
	client.Client
	lists int
}

func (c *countingClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	c.lists++
	return c.Client.List(ctx, list, opts...)
}

func configPolicy(name, namespace string, forceBuilderCli bool) *weka.WekaPolicy {
	return &weka.WekaPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: weka.WekaPolicySpec{
			Payload: weka.PolicyPayload{
				Configuration: &weka.ConfigurationPayload{
					Drivers: &weka.DriversSpec{ForceBuilderCli: boolPtr(forceBuilderCli)},
				},
			},
		},
	}
}

func testScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatalf("add client-go scheme: %v", err)
	}
	if err := weka.AddToScheme(scheme); err != nil {
		t.Fatalf("add weka scheme: %v", err)
	}
	return scheme
}

// useNamespace pins the operator namespace so util.GetPodNamespace resolves inside a test process,
// where neither the serviceaccount file nor dev mode applies.
func useNamespace(t *testing.T, namespace string) {
	t.Helper()
	previous := config.Config.OperatorPodNamespace
	config.Config.OperatorPodNamespace = namespace
	t.Cleanup(func() { config.Config.OperatorPodNamespace = previous })
}

func TestConfigurationCacheServesWithinTTL(t *testing.T) {
	useNamespace(t, "weka-operator-system")
	ctx := context.Background()

	base := fake.NewClientBuilder().
		WithScheme(testScheme(t)).
		WithObjects(configPolicy("configuration", "weka-operator-system", true)).
		Build()
	c := &countingClient{Client: base}

	svc := &configurationCacheService{settings: DefaultConfigurationSettings(), ttl: time.Minute}

	if got := svc.GetSettings(ctx, c).Drivers.ForceBuilderCli; !got {
		t.Fatalf("first read: ForceBuilderCli = false, want true")
	}
	if c.lists != 1 {
		t.Fatalf("first read issued %d lists, want 1", c.lists)
	}

	// second read inside the TTL must be served from cache
	if got := svc.GetSettings(ctx, c).Drivers.ForceBuilderCli; !got {
		t.Fatalf("cached read: ForceBuilderCli = false, want true")
	}
	if c.lists != 1 {
		t.Fatalf("cached read issued %d lists total, want 1", c.lists)
	}

	// once expired, the next read refreshes
	svc.Invalidate()
	svc.GetSettings(ctx, c)
	if c.lists != 2 {
		t.Fatalf("after invalidate: %d lists total, want 2", c.lists)
	}
}

func TestConfigurationCacheFallsBackToDefaults(t *testing.T) {
	useNamespace(t, "weka-operator-system")
	ctx := context.Background()
	scheme := testScheme(t)

	cases := []struct {
		name    string
		objects []client.Object
	}{
		{"no configuration policy", nil},
		{
			name: "policy in another namespace does not apply",
			objects: []client.Object{
				configPolicy("configuration", "tenant-a", true),
			},
		},
		{
			name: "ambiguous - two policies, do not guess",
			objects: []client.Object{
				configPolicy("configuration-a", "weka-operator-system", true),
				configPolicy("configuration-b", "weka-operator-system", true),
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(tc.objects...).Build()
			svc := &configurationCacheService{settings: DefaultConfigurationSettings(), ttl: time.Minute}

			if got := svc.GetSettings(ctx, c).Drivers.ForceBuilderCli; got {
				t.Errorf("ForceBuilderCli = true, want the default false")
			}
		})
	}
}
