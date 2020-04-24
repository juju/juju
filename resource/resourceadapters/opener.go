// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resourceadapters

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	csclient "github.com/juju/juju/charmstore"
	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/charmstore"
	corestate "github.com/juju/juju/state"
)

// NewResourceOpener returns a new resource.Opener for the given unit.
//
// The caller owns the State provided. It is the caller's
// responsibility to close it.
//
// TODO(mjs): This is the entry point for a whole lot of untested shim
// code in this package. At some point this should be sorted out.
func NewResourceOpener(st *corestate.State, unitName string) (opener resource.Opener, err error) {
	unit, err := st.Unit(unitName)
	if err != nil {
		return nil, errors.Annotate(err, "loading unit")
	}

	resources, err := st.Resources()
	if err != nil {
		return nil, errors.Trace(err)
	}

	opener = &resourceOpener{
		st:     st,
		res:    resources,
		userID: unit.Tag(),
		unit:   unit,
	}
	return opener, nil
}

// resourceOpener is an implementation of server.ResourceOpener.
type resourceOpener struct {
	st     *corestate.State
	res    corestate.Resources
	userID names.Tag
	unit   *corestate.Unit
}

// OpenResource implements server.ResourceOpener.
func (ro *resourceOpener) OpenResource(name string) (o resource.Opened, err error) {
	if ro.unit == nil {
		return resource.Opened{}, errors.Errorf("missing unit")
	}
	app, err := ro.unit.Application()
	if err != nil {
		return resource.Opened{}, errors.Trace(err)
	}
	cURL, _ := ro.unit.CharmURL()
	id := csclient.CharmID{
		URL:     cURL,
		Channel: app.Channel(),
	}

	csOpener := newCharmstoreOpener(ro.st)
	client, err := csOpener.NewClient()
	if err != nil {
		return resource.Opened{}, errors.Trace(err)
	}

	cache := &charmstoreEntityCache{
		st:            ro.res,
		userID:        ro.userID,
		unit:          ro.unit,
		applicationID: ro.unit.ApplicationName(),
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
