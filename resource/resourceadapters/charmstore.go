// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resourceadapters

import (
	"io"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/retry"
	"github.com/juju/utils/clock"
	"gopkg.in/juju/charm.v6-unstable"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"

	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/charmstore"
	corestate "github.com/juju/juju/state"
)

// charmstoreEntityCache adapts between resource state and charmstore.EntityCache.
type charmstoreEntityCache struct {
	st        corestate.Resources
	userID    names.Tag
	unit      resource.Unit
	serviceID string
}

// GetResource implements charmstore.EntityCache.
func (cache *charmstoreEntityCache) GetResource(name string) (resource.Resource, error) {
	return cache.st.GetResource(cache.serviceID, name)
}

// SetResource implements charmstore.EntityCache.
func (cache *charmstoreEntityCache) SetResource(chRes charmresource.Resource, reader io.Reader) (resource.Resource, error) {
	return cache.st.SetResource(cache.serviceID, cache.userID.Id(), chRes, reader)
}

// OpenResource implements charmstore.EntityCache.
func (cache *charmstoreEntityCache) OpenResource(name string) (resource.Resource, io.ReadCloser, error) {
	if cache.unit == nil {
		return resource.Resource{}, nil, errors.NotImplementedf("")
	}
	return cache.st.OpenResourceForUniter(cache.unit, name)
}

type charmstoreOpener struct {
	// TODO(ericsnow) What do we need?
}

func newCharmstoreOpener(cURL *charm.URL) *charmstoreOpener {
	// TODO(ericsnow) Extract the charm store URL from the charm URL.
	return &charmstoreOpener{}
}

// NewClient opens a new charm store client.
func (cs *charmstoreOpener) NewClient() (charmstore.Client, error) {
	// TODO(ericsnow) Return an actual charm store client.
	client := newFakeCharmStoreClient(nil)
	return newCSRetryClient(client), nil
}

type csRetryClient struct {
	charmstore.Client
	retryArgs retry.CallArgs
}

func newCSRetryClient(client charmstore.Client) *csRetryClient {
	retryArgs := retry.CallArgs{
		// The only error that stops the retry loop should be "not found".
		IsFatalError: errors.IsNotFound,
		// We want to retry until the charm store either gives us the
		// resource (and we cache it) or the resource isn't found in the
		// charm store.
		Attempts: -1, // retry forever...
		// A one minute gives enough time for potential connection
		// issues to sort themselves out without making the caller wait
		// for an exceptional amount of time.
		Delay: 1 * time.Minute,
		Clock: clock.WallClock,
	}
	return &csRetryClient{
		Client:    client,
		retryArgs: retryArgs,
	}
}

// GetResource returns a reader for the resource's data.
func (client csRetryClient) GetResource(cURL *charm.URL, resourceName string, revision int) (io.ReadCloser, error) {
	args := client.retryArgs // a copy

	var reader io.ReadCloser
	args.Func = func() error {
		csReader, err := client.Client.GetResource(cURL, resourceName, revision)
		if err != nil {
			return errors.Trace(err)
		}
		reader = csReader
		return nil
	}

	var lastErr error
	args.NotifyFunc = func(err error, i int) {
		// Remember the error we're hiding and then retry!
		logger.Debugf("(attempt %d) retrying resource download from charm store due to error: %v", i, err)
		lastErr = err
	}

	err := retry.Call(args)
	if retry.IsAttemptsExceeded(err) {
		return nil, errors.Annotate(lastErr, "failed after retrying")
	}
	if err != nil {
		return nil, errors.Trace(err)
	}

	return reader, nil
}
