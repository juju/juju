// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelsecrets

import (
	"context"
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/retry"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/controller/crossmodelrelations"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/internal/secrets"
	"github.com/juju/juju/internal/secrets/provider"
	"github.com/juju/juju/rpc/params"
)

// Option is a function that can be used to configure a Client.
type Option = base.Option

// WithTracer returns an Option that configures the Client to use the
// supplied tracer.
var WithTracer = base.WithTracer

// Client provides access to the CrossModelSecrets API facade.
type Client struct {
	base.ClientFacade
	facade base.FacadeCaller

	cache *crossmodelrelations.MacaroonCache
}

// NewClient creates a new client-side CrossModelSecrets facade.
func NewClient(caller base.APICallCloser, options ...Option) *Client {
	return NewClientWithCache(caller, crossmodelrelations.NewMacaroonCache(clock.WallClock), options...)
}

// NewClientWithCache creates a new client-side CrossModelSecrets facade
// with the specified cache.
func NewClientWithCache(caller base.APICallCloser, cache *crossmodelrelations.MacaroonCache, options ...Option) *Client {
	frontend, backend := base.NewClientFacade(caller, "CrossModelSecrets", options...)
	return &Client{
		ClientFacade: frontend,
		facade:       backend,
		cache:        cache,
	}
}

var logger = loggo.GetLogger("juju.api.crossmodelsecrets")

func (c *Client) handleDischargeError(ctx context.Context, apiErr error) (macaroon.Slice, error) {
	errResp := errors.Cause(apiErr).(*params.Error)
	if errResp.Info == nil {
		return nil, errors.Annotatef(apiErr, "no error info found in discharge-required response error")
	}
	logger.Debugf("attempting to discharge macaroon due to error: %v", apiErr)
	var info params.DischargeRequiredErrorInfo
	if errUnmarshal := errResp.UnmarshalInfo(&info); errUnmarshal != nil {
		return nil, errors.Annotatef(apiErr, "unable to extract macaroon details from discharge-required response error")
	}

	m := info.BakeryMacaroon
	ms, err := c.facade.RawAPICaller().BakeryClient().DischargeAll(ctx, m)
	if err == nil && logger.IsTraceEnabled() {
		logger.Tracef("discharge macaroon ids:")
		for _, m := range ms {
			logger.Tracef("  - %v", m.Id())
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
		logger.Debugf("%s using cached macaroons for %s", opName, token)
		if logger.IsTraceEnabled() {
			for _, m := range ms {
				logger.Tracef("  - %v", m.Id())
			}
		}
	}
	return ms, ok
}

// Patch for testing.

var Clock retry.Clock = clock.WallClock

// Account for the fact that the remote controller might be starting up.
const (
	numRetries = 3
	retryDelay = 3 * time.Second
)

// GetSecretAccessScope gets access details for a secret from a cross model controller.
func (c *Client) GetSecretAccessScope(uri *coresecrets.URI, appToken string, unitId int) (string, error) {
	if uri == nil {
		return "", errors.NotValidf("empty secret URI")
	}

	arg := params.GetRemoteSecretAccessArg{
		ApplicationToken: appToken,
		UnitId:           unitId,
		URI:              uri.String(),
	}
	args := params.GetRemoteSecretAccessArgs{Args: []params.GetRemoteSecretAccessArg{arg}}

	apiCall := func() (string, error) {
		var results params.StringResults

		if err := c.facade.FacadeCall(
			context.TODO(),
			"GetSecretAccessScope", args, &results,
		); err != nil {
			return "", errors.Trace(err)
		}
		if n := len(results.Results); n != 1 {
			return "", errors.Errorf("expected 1 result, got %d", n)
		}

		if err := results.Results[0].Error; err != nil {
			return "", apiservererrors.RestoreError(err)
		}
		return results.Results[0].Result, nil
	}

	var (
		scopeTag string
		apiErr   error
	)
	err := retry.Call(retry.CallArgs{
		Func: func() error {
			scopeTag, apiErr = apiCall()
			return apiErr
		},
		IsFatalError: func(err error) bool {
			if errors.Is(err, errors.NotFound) || errors.Is(err, apiservererrors.ErrPerm) {
				return true
			}
			return false
		},
		Delay:    retryDelay,
		Clock:    Clock,
		Attempts: numRetries,
	})
	return scopeTag, errors.Trace(err)
}

// GetRemoteSecretContentInfo gets secret content from a cross model controller.
func (c *Client) GetRemoteSecretContentInfo(ctx context.Context, uri *coresecrets.URI, revision int, refresh, peek bool, sourceControllerUUID, appToken string, unitId int, macs macaroon.Slice) (
	*secrets.ContentParams, *provider.ModelBackendConfig, int, bool, error,
) {
	if uri == nil {
		return nil, nil, 0, false, errors.NotValidf("empty secret URI")
	}

	arg := params.GetRemoteSecretContentArg{
		SourceControllerUUID: sourceControllerUUID,
		ApplicationToken:     appToken,
		UnitId:               unitId,
		URI:                  uri.String(),
		Refresh:              refresh,
		Peek:                 peek,
		Macaroons:            macs,
		BakeryVersion:        bakery.LatestVersion,
	}
	if revision > 0 {
		arg.Revision = &revision
	}

	args := params.GetRemoteSecretContentArgs{Args: []params.GetRemoteSecretContentArg{arg}}
	// Use any previously cached discharge macaroons.
	if ms, ok := c.getCachedMacaroon("get remote secret content info", appToken); ok {
		args.Args[0].Macaroons = ms
		args.Args[0].BakeryVersion = bakery.LatestVersion
	}

	apiCall := func() (*secrets.ContentParams, *provider.ModelBackendConfig, int, bool, error) {
		var results params.SecretContentResults

		if err := c.facade.FacadeCall(
			context.TODO(),
			"GetSecretContentInfo", args, &results,
		); err != nil {
			return nil, nil, 0, false, errors.Trace(err)
		}
		if n := len(results.Results); n != 1 {
			return nil, nil, 0, false, errors.Errorf("expected 1 result, got %d", n)
		}

		if err := results.Results[0].Error; err != nil {
			return nil, nil, 0, false, apiservererrors.RestoreError(err)
		}
		content := &secrets.ContentParams{}
		result := results.Results[0].Content
		if result.ValueRef != nil {
			content.ValueRef = &coresecrets.ValueRef{
				BackendID:  result.ValueRef.BackendID,
				RevisionID: result.ValueRef.RevisionID,
			}
		}
		if len(result.Data) > 0 {
			content.SecretValue = coresecrets.NewSecretValue(result.Data)
		}
		var (
			backendCfg *provider.ModelBackendConfig
			draining   bool
		)
		if cfg := results.Results[0].BackendConfig; cfg != nil {
			backendCfg = &provider.ModelBackendConfig{
				ControllerUUID: cfg.ControllerUUID,
				ModelUUID:      cfg.ModelUUID,
				ModelName:      cfg.ModelName,
				BackendConfig: provider.BackendConfig{
					BackendType: cfg.Config.BackendType,
					Config:      cfg.Config.Params,
				},
			}
			draining = cfg.Draining
		}
		var latestRevision int
		if rev := results.Results[0].LatestRevision; rev != nil {
			latestRevision = *rev
		}
		return content, backendCfg, latestRevision, draining, nil
	}

	var (
		content        *secrets.ContentParams
		backend        *provider.ModelBackendConfig
		latestRevision int
		draining       bool
		apiErr         error
	)
	err := retry.Call(retry.CallArgs{
		Func: func() error {
			content, backend, latestRevision, draining, apiErr = apiCall()
			return apiErr
		},
		IsFatalError: func(err error) bool {
			if errors.Is(err, errors.NotFound) || errors.Is(err, apiservererrors.ErrPerm) {
				return true
			}
			if params.ErrCode(apiErr) != params.CodeDischargeRequired {
				return false
			}
			// On error, possibly discharge the macaroon and retry.
			var mac macaroon.Slice
			mac, apiErr = c.handleDischargeError(ctx, err)
			if apiErr != nil {
				return true
			}
			args.Args[0].Macaroons = mac
			args.Args[0].BakeryVersion = bakery.LatestVersion
			return false
		},
		Delay:    retryDelay,
		Clock:    Clock,
		Attempts: numRetries,
	})
	return content, backend, latestRevision, draining, errors.Trace(err)
}
