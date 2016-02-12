// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resourceadapters

import (
	"io"

	"github.com/juju/errors"
	"github.com/juju/names"
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
	// TODO(ericsnow) Do something with the charm URL.
	return &charmstoreOpener{}
}

// NewClient implements charmstore.NewOperationsDeps.
func (cs *charmstoreOpener) NewClient() (charmstore.Client, error) {
	// TODO(ericsnow) finish
	return nil, errors.NotImplementedf("")
}
