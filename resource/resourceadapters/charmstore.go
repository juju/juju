// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resourceadapters

import (
	"io"
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils/retry"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"
	"gopkg.in/juju/names.v2"

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
	// TODO(ericsnow) What else do we need?
}

func newCharmstoreOpener(st *state.State) *charmstoreOpener {
	return &charmstoreOpener{st}
}

func newCharmStoreClient(st *state.State) (charmstore.Client, error) {
	return charmstore.NewCachingClient(state.MacaroonCache{st}, nil)
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

// CSRetryClient is a wrapper around a Juju charm store client that
// retries GetResource() calls.
type CSRetryClient struct {
	charmstore.Client
}

func newCSRetryClient(client charmstore.Client) *CSRetryClient {
	return &CSRetryClient{
		Client: client,
	}
}

var retryStrategy = retry.Regular{
	Delay: 1 * time.Minute,
	Total: 5 * time.Hour,
}

// GetResource returns a reader for the resource's data.
func (client CSRetryClient) GetResource(req charmstore.ResourceRequest) (charmstore.ResourceData, error) {
	i := 0
	for a := retryStrategy.Start(nil, nil); a.Next(); {
		data, err := client.Client.GetResource(req)
		if err == nil || errors.IsNotFound(err) {
			return data, errors.Trace(err)
		}
		if !a.HasNext() {
			return charmstore.ResourceData{}, errors.Annotatef(err, "failed after retrying")
		}
		i++
		logger.Debugf("(attempt %d) retrying resource download from charm store due to error: %v", i, err)
	}
	panic("unreachable")
}
