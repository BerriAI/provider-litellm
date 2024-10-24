/*
Copyright 2022 The Crossplane Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package key

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane/crossplane-runtime/pkg/connection"
	"github.com/crossplane/crossplane-runtime/pkg/controller"
	"github.com/crossplane/crossplane-runtime/pkg/event"
	"github.com/crossplane/crossplane-runtime/pkg/ratelimiter"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"

	"github.com/crossplane/provider-litellm/apis/key/v1alpha1"
	apisv1alpha1 "github.com/crossplane/provider-litellm/apis/v1alpha1"
	"github.com/crossplane/provider-litellm/internal/features"
)

const (
	errNotKey       = "managed resource is not a Key custom resource"
	errTrackPCUsage = "cannot track ProviderConfig usage"
	errGetPC        = "cannot get ProviderConfig"
	errGetCreds     = "cannot get credentials"

	errNewClient = "cannot create new Service"
)

// A NoOpService does nothing.
type NoOpService struct{}

var (
	newNoOpService = func(_ []byte) (interface{}, error) { return &NoOpService{}, nil }
)

// Setup adds a controller that reconciles Key managed resources.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	name := managed.ControllerName(v1alpha1.KeyGroupKind)

	cps := []managed.ConnectionPublisher{managed.NewAPISecretPublisher(mgr.GetClient(), mgr.GetScheme())}
	if o.Features.Enabled(features.EnableAlphaExternalSecretStores) {
		cps = append(cps, connection.NewDetailsManager(mgr.GetClient(), apisv1alpha1.StoreConfigGroupVersionKind))
	}

	r := managed.NewReconciler(mgr,
		resource.ManagedKind(v1alpha1.KeyGroupVersionKind),
		managed.WithExternalConnecter(&connector{
			kube:         mgr.GetClient(),
			usage:        resource.NewProviderConfigUsageTracker(mgr.GetClient(), &apisv1alpha1.ProviderConfigUsage{}),
			newServiceFn: newNoOpService}),
		managed.WithLogger(o.Logger.WithValues("controller", name)),
		managed.WithPollInterval(o.PollInterval),
		managed.WithRecorder(event.NewAPIRecorder(mgr.GetEventRecorderFor(name))),
		managed.WithConnectionPublishers(cps...))

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(o.ForControllerRuntime()).
		WithEventFilter(resource.DesiredStateChanged()).
		For(&v1alpha1.Key{}).
		Complete(ratelimiter.NewReconciler(name, r, o.GlobalRateLimiter))
}

// A connector is expected to produce an ExternalClient when its Connect method
// is called.
type connector struct {
	kube         client.Client
	usage        resource.Tracker
	newServiceFn func(creds []byte) (interface{}, error)
}

// Connect typically produces an ExternalClient by:
// 1. Tracking that the managed resource is using a ProviderConfig.
// 2. Getting the managed resource's ProviderConfig.
// 3. Getting the credentials specified by the ProviderConfig.
// 4. Using the credentials to form a client.
func (c *connector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	cr, ok := mg.(*v1alpha1.Key)
	if !ok {
		return nil, errors.New(errNotKey)
	}

	if err := c.usage.Track(ctx, mg); err != nil {
		return nil, errors.Wrap(err, errTrackPCUsage)
	}

	pc := &apisv1alpha1.ProviderConfig{}
	if err := c.kube.Get(ctx, types.NamespacedName{Name: cr.GetProviderConfigReference().Name}, pc); err != nil {
		return nil, errors.Wrap(err, errGetPC)
	}

	cd := pc.Spec.Credentials
	data, err := resource.CommonCredentialExtractor(ctx, cd.Source, c.kube, cd.CommonCredentialSelectors)
	if err != nil {
		return nil, errors.Wrap(err, errGetCreds)
	}

	svc, err := c.newServiceFn(data)
	if err != nil {
		return nil, errors.Wrap(err, errNewClient)
	}

	apiBase := pc.Spec.APIBase
	apiKey := string(data[xpv1.ResourceCredentialsSecretAPIKeyKey])

	return &external{
		service: svc,
		client:  &http.Client{},
		apiBase: apiBase,
		apiKey:  apiKey,
	}, nil
}

// An ExternalClient observes, then either creates, updates, or deletes an
// external resource to ensure it reflects the managed resource's desired state.
type external struct {
	// A 'client' used to connect to the external resource API. In practice this
	// would be something like an AWS SDK client.
	service interface{}
	client  *http.Client
	apiBase string
	apiKey  string
}

func (c *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	cr, ok := mg.(*v1alpha1.Key)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotKey)
	}

	// These fmt statements should be removed in the real implementation.
	fmt.Printf("Observing: %+v", cr)

	return managed.ExternalObservation{
		// Return false when the external resource does not exist. This lets
		// the managed resource reconciler know that it needs to call Create to
		// (re)create the resource, or that it has successfully been deleted.
		ResourceExists: true,

		// Return false when the external resource exists, but it not up to date
		// with the desired managed resource state. This lets the managed
		// resource reconciler know that it needs to call Update.
		ResourceUpToDate: true,

		// Return any details that may be required to connect to the external
		// resource. These will be stored as the connection secret.
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}
func (c *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*v1alpha1.Key)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotKey)
	}

	// Prepare the request payload
	payload := map[string]interface{}{
		"duration":        cr.Spec.ForProvider.Duration,
		"key_alias":       cr.Spec.ForProvider.KeyAlias,
		"key":             cr.Spec.ForProvider.Key,
		"team_id":         cr.Spec.ForProvider.TeamID,
		"user_id":         cr.Spec.ForProvider.UserID,
		"models":          cr.Spec.ForProvider.Models,
		"max_budget":      cr.Spec.ForProvider.MaxBudget,
		"budget_duration": cr.Spec.ForProvider.BudgetDuration,
		"metadata":        cr.Spec.ForProvider.Metadata,
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, "failed to marshal payload")
	}

	// Make the API call to /key/generate
	req, err := http.NewRequest("POST", c.apiBase+"/key/generate", bytes.NewBuffer(jsonPayload))
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, "failed to create request")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.client.Do(req)
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, "failed to create key")
	}

	// Parse the response
	var keyResponse struct {
		Key     string    `json:"key"`
		Expires time.Time `json:"expires"`
		UserID  string    `json:"user_id"`
		Status  string    `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&keyResponse); err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, "failed to decode key response")
	}

	// Update the resource status
	cr.Status.AtProvider.Key = keyResponse.Key
	cr.Status.AtProvider.Expires = metav1.Time{Time: keyResponse.Expires}
	cr.Status.AtProvider.UserID = keyResponse.UserID
	cr.Status.AtProvider.Status = keyResponse.Status

	return managed.ExternalCreation{
		ConnectionDetails: managed.ConnectionDetails{
			"key": []byte(keyResponse.Key),
		},
	}, nil
}
func (c *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	cr, ok := mg.(*v1alpha1.Key)
	if !ok {
		return managed.ExternalUpdate{}, errors.New(errNotKey)
	}

	fmt.Printf("Updating: %+v", cr)

	return managed.ExternalUpdate{
		// Optionally return any details that may be required to connect to the
		// external resource. These will be stored as the connection secret.
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}

func (c *external) Delete(ctx context.Context, mg resource.Managed) error {
	cr, ok := mg.(*v1alpha1.Key)
	if !ok {
		return errors.New(errNotKey)
	}

	fmt.Printf("Deleting: %+v", cr)

	return nil
}
