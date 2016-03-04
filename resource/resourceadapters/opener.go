// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resourceadapters

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/charmstore"
	corestate "github.com/juju/juju/state"
)

// resourceOpener is an implementation of server.ResourceOpener.
type resourceOpener struct {
	st     corestate.Resources
	userID names.Tag
	unit   resource.Unit
}

// OpenResource implements server.ResourceOpener.
func (ro *resourceOpener) OpenResource(name string) (resource.Opened, error) {
	if ro.unit == nil {
		return resource.Opened{}, errors.Errorf("missing unit")
	}
	cURL, _ := ro.unit.CharmURL()

	csOpener := newCharmstoreOpener(cURL)
	client, err := csOpener.NewClient()
	if err != nil {
		return resource.Opened{}, errors.Trace(err)
	}
	defer client.Close()

	cache := &charmstoreEntityCache{
		st:        ro.st,
		userID:    ro.userID,
		unit:      ro.unit,
		serviceID: ro.unit.ServiceName(),
	}

	res, reader, err := charmstore.GetResource(charmstore.GetResourceArgs{
		Client:   client,
		Cache:    cache,
		CharmURL: cURL,
		Name:     name,
	})
	if err != nil {
		return resource.Opened{}, errors.Trace(err)
	}

	opened := resource.Opened{
		Resource:   res,
		ReadCloser: reader,
	}
	return opened, nil
}
