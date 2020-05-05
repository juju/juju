// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resourceadapters

import (
	"io"
	"time"

	charmresource "github.com/juju/charm/v7/resource"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/retry"

	"github.com/juju/juju/charmstore"
	"github.com/juju/juju/resource"
	"github.com/juju/juju/state"
)

// charmstoreEntityCache adapts between resource state and charmstore.EntityCache.
type charmstoreEntityCache struct {
	st            state.Resources
	userID        names.Tag
	unit          resource.Unit
	applicationID string
}

// GetResource implements charmstore.EntityCache.
func (cache *charmstoreEntityCache) GetResource(name string) (resource.Resource, error) {
	return cache.st.GetResource(cache.applicationID, name)
}

// SetResource implements charmstore.EntityCache.
func (cache *charmstoreEntityCache) SetResource(chRes charmresource.Resource, reader io.Reader) (resource.Resource, error) {
	return cache.st.SetResource(cache.applicationID, cache.userID.Id(), chRes, reader)
}

// OpenResource implements charmstore.EntityCache.
func (cache *charmstoreEntityCache) OpenResource(name string) (resource.Resource, io.ReadCloser, error) {
	if cache.unit == nil {
		return resource.Resource{}, nil, errors.NotImplementedf("")
	}
	return cache.st.OpenResourceForUniter(cache.unit, name)
}

type charmstoreOpener struct {
	st *state.State
}

func newCharmstoreOpener(st *state.State) *charmstoreOpener {
	return &charmstoreOpener{st}
}

func newCharmStoreClient(st *state.State) (charmstore.Client, error) {
	controllerCfg, err := st.ControllerConfig()
	if err != nil {
		return charmstore.Client{}, errors.Trace(err)
	}
	return charmstore.NewCachingClient(state.MacaroonCache{st}, controllerCfg.CharmStoreURL())
}

// NewClient opens a new charm store client.
func (cs *charmstoreOpener) NewClient() (*CSRetryClient, error) {
	// TODO(ericsnow) Use a valid charm store client.
	client, err := newCharmStoreClient(cs.st)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return newCSRetryClient(client), nil
}

// ResourceClient defines a set of functionality that a client
// needs to define to support resources.
type ResourceClient interface {
	GetResource(req charmstore.ResourceRequest) (data charmstore.ResourceData, err error)
}

// CSRetryClient is a wrapper around a Juju charm store client that
// retries GetResource() calls.
type CSRetryClient struct {
	ResourceClient
	retryArgs retry.CallArgs
}

func newCSRetryClient(client ResourceClient) *CSRetryClient {
	retryArgs := retry.CallArgs{
		// (anastasiamac 2017-05-25) This might not work as the error types
		// may be lost after a call to some clients.
		IsFatalError: func(err error) bool {
			return errors.IsNotFound(err) || errors.IsNotValid(err)
		},
		// We don't want to retry for ever.
		// If we cannot get a resource after trying a few times,
		// most likely user intervention is needed.
		Attempts: 3,
		// A one minute gives enough time for potential connection
		// issues to sort themselves out without making the caller wait
		// for an exceptional amount of time.
		Delay: 1 * time.Minute,
		Clock: clock.WallClock,
	}
	return &CSRetryClient{
		ResourceClient: client,
		retryArgs:      retryArgs,
	}
}

// GetResource returns a reader for the resource's data.
func (client CSRetryClient) GetResource(req charmstore.ResourceRequest) (charmstore.ResourceData, error) {
	args := client.retryArgs // a copy

	var data charmstore.ResourceData
	args.Func = func() error {
		var err error
		data, err = client.ResourceClient.GetResource(req)
		if err != nil {
			return errors.Trace(err)
		}
		return nil
	}

	var lastErr error
	args.NotifyFunc = func(err error, i int) {
		// Remember the error we're hiding and then retry!
		logger.Warningf("attempt %d/%d to download resource %q from charm store [channel (%v), charm (%v), resource revision (%v)] failed with error (will retry): %v",
			i, client.retryArgs.Attempts, req.Name, req.Channel, req.Charm, req.Revision, err)
		logger.Tracef("resource get error stack: %v", errors.ErrorStack(err))
		lastErr = err
	}

	err := retry.Call(args)
	if retry.IsAttemptsExceeded(err) {
		return data, errors.Annotate(lastErr, "failed after retrying")
	}
	if err != nil {
		return data, errors.Trace(err)
	}

	return data, nil
}
