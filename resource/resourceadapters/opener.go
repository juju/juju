// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resourceadapters

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	csclient "github.com/juju/juju/charmstore"
	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/charmstore"
	corestate "github.com/juju/juju/state"
)

// resourceOpener is an implementation of server.ResourceOpener.
type resourceOpener struct {
	st     *corestate.State
	res    corestate.Resources
	userID names.Tag
	unit   *corestate.Unit
}

// OpenResource implements server.ResourceOpener.
func (ro *resourceOpener) OpenResource(name string) (resource.Opened, error) {
	if ro.unit == nil {
		return resource.Opened{}, errors.Errorf("missing unit")
	}
	svc, err := ro.unit.Service()
	if err != nil {
		return resource.Opened{}, errors.Trace(err)
	}
	cURL, _ := ro.unit.CharmURL()
	id := csclient.CharmID{
		URL:     cURL,
		Channel: svc.Channel(),
	}

	csOpener := newCharmstoreOpener(ro.st)
	client, err := csOpener.NewClient()
	if err != nil {
		return resource.Opened{}, errors.Trace(err)
	}

	cache := &charmstoreEntityCache{
		st:        ro.res,
		userID:    ro.userID,
		unit:      ro.unit,
		serviceID: ro.unit.ServiceName(),
	}

	res, reader, err := charmstore.GetResource(charmstore.GetResourceArgs{
		Client:  client,
		Cache:   cache,
		CharmID: id,
		Name:    name,
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
