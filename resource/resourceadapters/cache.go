// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resourceadapters

import (
	"io"

	charmresource "github.com/juju/charm/v8/resource"
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/resource"
)

// resourceCache adapts between resource state and charmstore.EntityCache.
type resourceCache struct {
	st      Resources
	userID  names.Tag
	unit    resource.Unit
	appName string
}

// GetResource implements charmstore.EntityCache.
func (cache *resourceCache) GetResource(name string) (resource.Resource, error) {
	return cache.st.GetResource(cache.appName, name)
}

// SetResource implements charmstore.EntityCache.
func (cache *resourceCache) SetResource(chRes charmresource.Resource, reader io.Reader) (resource.Resource, error) {
	return cache.st.SetResource(cache.appName, cache.userID.Id(), chRes, reader)
}

// OpenResource implements charmstore.EntityCache.
func (cache *resourceCache) OpenResource(name string) (resource.Resource, io.ReadCloser, error) {
	if cache.unit == nil {
		return resource.Resource{}, nil, errors.NotImplementedf("")
	}
	return cache.st.OpenResourceForUniter(cache.unit, name)
}
