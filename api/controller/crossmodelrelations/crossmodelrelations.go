// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelrelations

import (
	"context"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/api/base"
	apiwatcher "github.com/juju/juju/api/watcher"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/watcher"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/rpc/params"
)

// Option is a function that can be used to configure a Client.
type Option = base.Option

// WithTracer returns an Option that configures the Client to use the
// supplied tracer.
var WithTracer = base.WithTracer

var logger = internallogger.GetLogger("juju.api.crossmodelrelations")

// Client provides access to the crossmodelrelations api facade.
type Client struct {
	base.ClientFacade
	facade base.FacadeCaller

	cache *MacaroonCache
}

// NewClient creates a new client-side CrossModelRelations facade.
func NewClient(caller base.APICallCloser, options ...Option) *Client {
	return NewClientWithCache(caller, NewMacaroonCache(clock.WallClock), options...)
}

// NewClientWithCache creates a new client-side CrossModelRelations facade
// with the specified cache.
func NewClientWithCache(caller base.APICallCloser, cache *MacaroonCache, options ...Option) *Client {
	frontend, backend := base.NewClientFacade(caller, "CrossModelRelations", options...)
	return &Client{
		ClientFacade: frontend,
		facade:       backend,
		cache:        cache,
	}
}

// handleError is used to process an error obtained when making a facade call.
// If the error indicates that a macaroon discharge is required, this is done
// and the resulting discharge macaroons passed back so the api call can be retried.
func (c *Client) handleError(ctx context.Context, apiErr error) (macaroon.Slice, error) {
	if params.ErrCode(apiErr) != params.CodeDischargeRequired {
		return nil, apiErr
	}
	errResp := errors.Cause(apiErr).(*params.Error)
	if errResp.Info == nil {
		return nil, errors.Annotatef(apiErr, "no error info found in discharge-required response error")
	}
	logger.Debugf(context.TODO(), "attempting to discharge macaroon due to error: %v", apiErr)
	var info params.DischargeRequiredErrorInfo
	if errUnmarshal := errResp.UnmarshalInfo(&info); errUnmarshal != nil {
		return nil, errors.Annotatef(apiErr, "unable to extract macaroon details from discharge-required response error")
	}

	// Prefer the new bakery macaroon.
	m := info.BakeryMacaroon
	if m == nil {
		var err error
		m, err = bakery.NewLegacyMacaroon(info.Macaroon)
		if err != nil {
			return nil, errors.Wrap(apiErr, err)
		}
	}
	ms, err := c.facade.RawAPICaller().BakeryClient().DischargeAll(ctx, m)
	if err == nil && logger.IsLevelEnabled(corelogger.TRACE) {
		logger.Tracef(context.TODO(), "discharge macaroon ids:")
		for _, m := range ms {
			logger.Tracef(context.TODO(), "  - %v", m.Id())
		}
	}
	if err != nil {
		return nil, errors.Wrap(apiErr, err)
	}
	return ms, err
}

func (c *Client) getCachedMacaroon(opName, token string) (macaroon.Slice, bool) {
	ms, ok := c.cache.Get(token)
	if ok {
		logger.Debugf(context.TODO(), "%s using cached macaroons for %s", opName, token)
		if logger.IsLevelEnabled(corelogger.TRACE) {
			for _, m := range ms {
				logger.Tracef(context.TODO(), "  - %v", m.Id())
			}
		}
	}
	return ms, ok
}

// PublishRelationChange publishes relation changes to the
// model hosting the remote application involved in the relation.
func (c *Client) PublishRelationChange(ctx context.Context, change params.RemoteRelationChangeEvent) error {
	args := params.RemoteRelationsChanges{
		Changes: []params.RemoteRelationChangeEvent{change},
	}
	// Use any previously cached discharge macaroons.
	if ms, ok := c.getCachedMacaroon("publish relation changed", change.RelationToken); ok {
		args.Changes[0].Macaroons = ms
		args.Changes[0].BakeryVersion = bakery.LatestVersion
	}

	apiCall := func() error {
		var results params.ErrorResults
		if err := c.facade.FacadeCall(ctx, "PublishRelationChanges", args, &results); err != nil {
			return errors.Trace(err)
		}
		err := results.OneError()
		if params.IsCodeNotFound(err) {
			return errors.NotFoundf("relation for event %v", change)
		}
		return err
	}
	// Make the api call the first time.
	err := apiCall()
	if err == nil || errors.Is(err, errors.NotFound) {
		return errors.Trace(err)
	}

	// On error, possibly discharge the macaroon and retry.
	mac, err2 := c.handleError(ctx, err)
	if err2 != nil {
		return errors.Trace(err2)
	}
	args.Changes[0].Macaroons = mac
	args.Changes[0].BakeryVersion = bakery.LatestVersion
	c.cache.Upsert(args.Changes[0].RelationToken, mac)
	return apiCall()
}

func (c *Client) PublishIngressNetworkChange(ctx context.Context, change params.IngressNetworksChangeEvent) error {
	args := params.IngressNetworksChanges{
		Changes: []params.IngressNetworksChangeEvent{change},
	}
	// Use any previously cached discharge macaroons.
	if ms, ok := c.getCachedMacaroon("publish ingress network change", change.RelationToken); ok {
		args.Changes[0].Macaroons = ms
		args.Changes[0].BakeryVersion = bakery.LatestVersion
	}

	apiCall := func() error {
		var results params.ErrorResults
		if err := c.facade.FacadeCall(ctx, "PublishIngressNetworkChanges", args, &results); err != nil {
			return errors.Trace(err)
		}
		return results.OneError()
	}

	// Make the api call the first time.
	err := apiCall()
	if err == nil {
		return nil
	}

	// On error, possibly discharge the macaroon and retry.
	mac, err2 := c.handleError(ctx, err)
	if err2 != nil {
		return errors.Trace(err2)
	}
	args.Changes[0].Macaroons = mac
	args.Changes[0].BakeryVersion = bakery.LatestVersion
	c.cache.Upsert(args.Changes[0].RelationToken, mac)
	return apiCall()
}

// RegisterRemoteRelations sets up the remote model to participate
// in the specified relations.
func (c *Client) RegisterRemoteRelations(ctx context.Context, relations ...params.RegisterRemoteRelationArg) ([]params.RegisterRemoteRelationResult, error) {
	var (
		args         params.RegisterRemoteRelationArgs
		retryIndices []int
	)

	args = params.RegisterRemoteRelationArgs{Relations: relations}
	// Use any previously cached discharge macaroons.
	for i, arg := range relations {
		if ms, ok := c.getCachedMacaroon("register remote relation", arg.RelationToken); ok {
			newArg := arg
			newArg.Macaroons = ms
			newArg.BakeryVersion = bakery.LatestVersion
			args.Relations[i] = newArg
		}
	}

	var results params.RegisterRemoteRelationResults
	apiCall := func() error {
		// Reset the results struct before each api call.
		results = params.RegisterRemoteRelationResults{}
		err := c.facade.FacadeCall(ctx, "RegisterRemoteRelations", args, &results)
		if err != nil {
			return errors.Trace(err)
		}
		if len(results.Results) != len(args.Relations) {
			return errors.Errorf("expected %d result(s), got %d", len(args.Relations), len(results.Results))
		}
		return nil
	}

	// Make the api call the first time.
	if err := apiCall(); err != nil {
		return nil, errors.Trace(err)
	}
	// On error, possibly discharge the macaroon and retry.
	result := results.Results
	args = params.RegisterRemoteRelationArgs{}
	// Separate the successful calls from those needing a retry.
	for i, res := range results.Results {
		if res.Error == nil {
			continue
		}
		mac, err := c.handleError(ctx, res.Error)
		if err != nil {
			resCopy := res
			resCopy.Error.Message = err.Error()
			result[i] = resCopy
			continue
		}
		retryArg := relations[i]
		retryArg.Macaroons = mac
		retryArg.BakeryVersion = bakery.LatestVersion
		args.Relations = append(args.Relations, retryArg)
		retryIndices = append(retryIndices, i)
		c.cache.Upsert(retryArg.RelationToken, mac)
	}
	// Nothing to retry so return the original result.
	if len(args.Relations) == 0 {
		return result, nil
	}

	if err := apiCall(); err != nil {
		return nil, errors.Trace(err)
	}
	// After a retry, insert the results into the original result slice.
	for j, res := range results.Results {
		resCopy := res
		result[retryIndices[j]] = resCopy
	}
	return result, nil
}

// WatchRelationChanges returns a watcher that notifies of changes to
// the units or application settings in the remote model for the
// relation with the given remote token.
func (c *Client) WatchRelationChanges(ctx context.Context, relationToken, applicationToken string, macs macaroon.Slice) (apiwatcher.RemoteRelationWatcher, error) {
	args := params.RemoteEntityArgs{Args: []params.RemoteEntityArg{{
		Token:         relationToken,
		Macaroons:     macs,
		BakeryVersion: bakery.LatestVersion,
	}}}
	// Use any previously cached discharge macaroons.
	if ms, ok := c.getCachedMacaroon("watch relation changes", relationToken); ok {
		args.Args[0].Macaroons = ms
		args.Args[0].BakeryVersion = bakery.LatestVersion
	}

	var results params.RemoteRelationWatchResults
	apiCall := func() error {
		// Reset the results struct before each api call.
		results = params.RemoteRelationWatchResults{}
		if err := c.facade.FacadeCall(ctx, "WatchRelationChanges", args, &results); err != nil {
			return errors.Trace(err)
		}
		if len(results.Results) != 1 {
			return errors.Errorf("expected 1 result, got %d", len(results.Results))
		}
		return nil
	}

	// Make the api call the first time.
	if err := apiCall(); err != nil {
		return nil, errors.Trace(err)
	}

	// On error, possibly discharge the macaroon and retry.
	result := results.Results[0]
	if result.Error != nil {
		mac, err := c.handleError(ctx, result.Error)
		if err != nil {
			result.Error.Message = err.Error()
			return nil, result.Error
		}
		args.Args[0].Macaroons = mac
		args.Args[0].BakeryVersion = bakery.LatestVersion
		c.cache.Upsert(args.Args[0].Token, mac)

		if err := apiCall(); err != nil {
			return nil, errors.Trace(err)
		}
		result = results.Results[0]
	}
	if result.Error != nil {
		return nil, result.Error
	}

	w := apiwatcher.NewRemoteRelationWatcher(c.facade.RawAPICaller(), result)
	return w, nil
}

// WatchEgressAddressesForRelation returns a watcher that notifies when addresses,
// from which connections will originate to the offering side of the relation, change.
// Each event contains the entire set of addresses which the offering side is required
// to allow for access to the other side of the relation.
func (c *Client) WatchEgressAddressesForRelation(ctx context.Context, remoteRelationArg params.RemoteEntityArg) (watcher.StringsWatcher, error) {
	args := params.RemoteEntityArgs{Args: []params.RemoteEntityArg{remoteRelationArg}}
	// Use any previously cached discharge macaroons.
	if ms, ok := c.getCachedMacaroon("watch relation egress addresses", remoteRelationArg.Token); ok {
		args.Args[0].Macaroons = ms
		args.Args[0].BakeryVersion = bakery.LatestVersion
	}

	var results params.StringsWatchResults
	apiCall := func() error {
		// Reset the results struct before each api call.
		results = params.StringsWatchResults{}
		if err := c.facade.FacadeCall(ctx, "WatchEgressAddressesForRelations", args, &results); err != nil {
			return errors.Trace(err)
		}
		if len(results.Results) != 1 {
			return errors.Errorf("expected 1 result, got %d", len(results.Results))
		}
		return nil
	}

	// Make the api call the first time.
	if err := apiCall(); err != nil {
		return nil, errors.Trace(err)
	}

	// On error, possibly discharge the macaroon and retry.
	result := results.Results[0]
	if result.Error != nil {
		mac, err := c.handleError(ctx, result.Error)
		if err != nil {
			result.Error.Message = err.Error()
			return nil, result.Error
		}
		args.Args[0].Macaroons = mac
		args.Args[0].BakeryVersion = bakery.LatestVersion
		c.cache.Upsert(args.Args[0].Token, mac)

		if err := apiCall(); err != nil {
			return nil, errors.Trace(err)
		}
		result = results.Results[0]
	}
	if result.Error != nil {
		return nil, result.Error
	}

	w := apiwatcher.NewStringsWatcher(c.facade.RawAPICaller(), result)
	return w, nil
}

// WatchRelationSuspendedStatus starts a RelationStatusWatcher for watching the life and
// suspended status of the specified relation in the remote model.
func (c *Client) WatchRelationSuspendedStatus(ctx context.Context, arg params.RemoteEntityArg) (watcher.RelationStatusWatcher, error) {
	args := params.RemoteEntityArgs{Args: []params.RemoteEntityArg{arg}}
	// Use any previously cached discharge macaroons.
	if ms, ok := c.getCachedMacaroon("watch relation status", arg.Token); ok {
		args.Args[0].Macaroons = ms
		args.Args[0].BakeryVersion = bakery.LatestVersion
	}

	var results params.RelationStatusWatchResults
	apiCall := func() error {
		// Reset the results struct before each api call.
		results = params.RelationStatusWatchResults{}
		if err := c.facade.FacadeCall(ctx, "WatchRelationsSuspendedStatus", args, &results); err != nil {
			return errors.Trace(err)
		}
		if len(results.Results) != 1 {
			return errors.Errorf("expected 1 result, got %d", len(results.Results))
		}
		return nil
	}

	// Make the api call the first time.
	if err := apiCall(); err != nil {
		return nil, errors.Trace(err)
	}

	// On error, possibly discharge the macaroon and retry.
	result := results.Results[0]
	if result.Error != nil {
		mac, err := c.handleError(ctx, result.Error)
		if err != nil {
			result.Error.Message = err.Error()
			return nil, result.Error
		}
		args.Args[0].Macaroons = mac
		args.Args[0].BakeryVersion = bakery.LatestVersion
		c.cache.Upsert(args.Args[0].Token, mac)

		if err := apiCall(); err != nil {
			return nil, errors.Trace(err)
		}
		result = results.Results[0]
	}
	if result.Error != nil {
		return nil, result.Error
	}

	w := apiwatcher.NewRelationStatusWatcher(c.facade.RawAPICaller(), result)
	return w, nil
}

// WatchOfferStatus starts an OfferStatusWatcher for watching the status
// of the specified offer in the remote model.
func (c *Client) WatchOfferStatus(ctx context.Context, arg params.OfferArg) (watcher.OfferStatusWatcher, error) {
	args := params.OfferArgs{Args: []params.OfferArg{arg}}
	// Use any previously cached discharge macaroons.
	if ms, ok := c.getCachedMacaroon("watch offer status", arg.OfferUUID); ok {
		args.Args[0].Macaroons = ms
		args.Args[0].BakeryVersion = bakery.LatestVersion
	}

	var results params.OfferStatusWatchResults
	apiCall := func() error {
		// Reset the results struct before each api call.
		results = params.OfferStatusWatchResults{}
		if err := c.facade.FacadeCall(ctx, "WatchOfferStatus", args, &results); err != nil {
			return errors.Trace(err)
		}
		if len(results.Results) != 1 {
			return errors.Errorf("expected 1 result, got %d", len(results.Results))
		}
		return nil
	}

	// Make the api call the first time.
	if err := apiCall(); err != nil {
		return nil, errors.Trace(err)
	}

	// On error, possibly discharge the macaroon and retry.
	result := results.Results[0]
	if result.Error != nil {
		mac, err := c.handleError(ctx, result.Error)
		if err != nil {
			result.Error.Message = err.Error()
			return nil, result.Error
		}
		args.Args[0].Macaroons = mac
		c.cache.Upsert(args.Args[0].OfferUUID, mac)

		if err := apiCall(); err != nil {
			return nil, errors.Trace(err)
		}
		result = results.Results[0]
	}
	if result.Error != nil {
		return nil, result.Error
	}

	w := apiwatcher.NewOfferStatusWatcher(c.facade.RawAPICaller(), result)
	return w, nil
}

// WatchConsumedSecretsChanges returns a watcher which notifies of new secret revisions consumed by the
// app with the specified token.
func (c *Client) WatchConsumedSecretsChanges(ctx context.Context, applicationToken, relationToken string, mac *macaroon.Macaroon) (watcher.SecretsRevisionWatcher, error) {
	var macs macaroon.Slice
	if mac != nil {
		macs = macaroon.Slice{mac}
	}

	args := params.WatchRemoteSecretChangesArgs{Args: []params.WatchRemoteSecretChangesArg{{
		ApplicationToken: applicationToken,
		RelationToken:    relationToken,
		Macaroons:        macs,
		BakeryVersion:    bakery.LatestVersion,
	}}}

	// Use any previously cached discharge macaroons.
	if ms, ok := c.getCachedMacaroon("watch consumed secret changes", relationToken); ok {
		args.Args[0].Macaroons = ms
		args.Args[0].BakeryVersion = bakery.LatestVersion
	}

	var results params.SecretRevisionWatchResults
	apiCall := func() error {
		// Reset the results struct before each api call.
		results = params.SecretRevisionWatchResults{}
		if err := c.facade.FacadeCall(ctx, "WatchConsumedSecretsChanges", args, &results); err != nil {
			return params.TranslateWellKnownError(err)
		}
		if len(results.Results) != 1 {
			return errors.Errorf("expected 1 result, got %d", len(results.Results))
		}
		return nil
	}

	// Make the api call the first time.
	if err := apiCall(); err != nil {
		return nil, errors.Trace(err)
	}

	// On error, possibly discharge the macaroon and retry.
	result := results.Results[0]
	if result.Error != nil {
		mac, err := c.handleError(ctx, result.Error)
		if err != nil {
			result.Error.Message = err.Error()
			return nil, result.Error
		}
		args.Args[0].Macaroons = mac
		c.cache.Upsert(relationToken, mac)

		if err := apiCall(); err != nil {
			return nil, errors.Trace(err)
		}
		result = results.Results[0]
	}
	if result.Error != nil {
		return nil, params.TranslateWellKnownError(result.Error)
	}

	w := apiwatcher.NewSecretsRevisionWatcher(c.facade.RawAPICaller(), result)
	return w, nil
}
