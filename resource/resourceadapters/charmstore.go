// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resourceadapters

import (
	"io"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/utils"
	"gopkg.in/juju/charm.v6-unstable"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"

	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/charmstore"
	corestate "github.com/juju/juju/state"
)

// EntityState adapts between resource state and charmstore.EntityCache.
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
	if cache.unit != nil {
		return cache.st.OpenResourceForUnit(cache.unit, name)
	}
	return cache.st.OpenResource(cache.serviceID, name)
}

type charmstoreOpener struct {
	// TODO(ericsnow) What do we need?
}

func newCharmstoreOpener(cURL *charm.URL) *charmstoreOpener {
	// TODO(ericsnow) Extract the charm store URL from the charm URL.
	return &charmstoreOpener{}
}

// NewClient implements charmstore.NewOperationsDeps.
func (cs *charmstoreOpener) NewClient() (charmstore.Client, error) {
	// TODO(ericsnow) Return an actual charm store client.
	client := newFakeCharmStoreClient(nil)
	return newCSRetryClient(client), nil
}

type csRetryClient struct {
	charmstore.Client
	strategy utils.AttemptStrategy
}

func newCSRetryClient(client charmstore.Client) *csRetryClient {
	strategy := utils.AttemptStrategy{
		Delay: 1 * time.Minute,
		Min:   4, // max 5 tries
	}
	return &csRetryClient{
		Client:   client,
		strategy: strategy,
	}
}

// GetResource returns a reader for the resource's data.
func (client csRetryClient) GetResource(cURL *charm.URL, resourceName string, revision int) (io.ReadCloser, error) {
	retries := client.strategy.Start()
	var lastErr error
	for retries.Next() {
		reader, err := client.Client.GetResource(cURL, resourceName, revision)
		if err == nil {
			return reader, nil
		}
		if errorShouldNotRetry(err) {
			return nil, errors.Trace(err)
		}
		// Otherwise, remember the error we're hiding and then retry!
		logger.Errorf("retrying resource download from charm store due to error: %v", err)
		lastErr = err
	}
	return nil, errors.Annotate(lastErr, "failed after retrying")
}

func errorShouldNotRetry(err error) bool {
	if errors.IsNotFound(err) {
		return true
	}
	return false
}
