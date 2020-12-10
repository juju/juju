// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resourceadapters

import (
	"github.com/juju/charm/v8"
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/repositories"
	corestate "github.com/juju/juju/state"
)

// NewResourceOpener returns a new resource.Opener for the given unit.
//
// The caller owns the State provided. It is the caller's
// responsibility to close it.

func NewResourceOpener(st *corestate.State, unitName string) (opener resource.Opener, err error) {
	return newInternalResourceOpener(&stateShim{st}, unitName)
}

func newInternalResourceOpener(st ResourceOpenerState, unitName string) (opener resource.Opener, err error) {
	unit, err := st.Unit(unitName)
	if err != nil {
		return nil, errors.Annotate(err, "loading unit")
	}

	resources, err := st.Resources()
	if err != nil {
		return nil, errors.Trace(err)
	}

	curl, _ := unit.CharmURL()
	switch {
	case charm.CharmStore.Matches(curl.Schema):
		opener = &ResourceOpener{
			st:                st,
			res:               resources,
			userID:            unit.Tag(),
			unit:              unit,
			newResourceOpener: func(st ResourceOpenerState) ResourceRetryClientGetter { return newCharmStoreOpener(st) },
		}
	case charm.CharmHub.Matches(curl.Schema):
		opener = &ResourceOpener{
			st:                st,
			res:               resources,
			userID:            unit.Tag(),
			unit:              unit,
			newResourceOpener: func(st ResourceOpenerState) ResourceRetryClientGetter { return newCharmHubOpener(st) },
		}
	}
	return opener, nil
}

// ResourceOpener is a ResourceOpener for the charm store.
type ResourceOpener struct {
	st                ResourceOpenerState
	res               Resources
	userID            names.Tag
	unit              Unit
	newResourceOpener func(st ResourceOpenerState) ResourceRetryClientGetter
}

// OpenResource implements server.ResourceOpener.
func (ro *ResourceOpener) OpenResource(name string) (o resource.Opened, err error) {
	if ro.unit == nil {
		return resource.Opened{}, errors.Errorf("missing unit")
	}
	app, err := ro.unit.Application()
	if err != nil {
		return resource.Opened{}, errors.Trace(err)
	}
	cURL, _ := ro.unit.CharmURL()
	id := repositories.CharmID{
		URL:    cURL,
		Origin: *app.CharmOrigin(),
	}

	client, err := ro.newResourceOpener(ro.st).NewClient()
	if err != nil {
		return resource.Opened{}, errors.Trace(err)
	}

	cache := &resourceCache{
		st:      ro.res,
		userID:  ro.userID,
		unit:    ro.unit,
		appName: ro.unit.ApplicationName(),
	}

	res, reader, err := repositories.GetResource(repositories.GetResourceArgs{
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

type stateShim struct {
	*corestate.State
}

func (s *stateShim) Model() (Model, error) {
	return s.State.Model()
}

func (s *stateShim) Unit(name string) (Unit, error) {
	u, err := s.State.Unit(name)
	if err != nil {
		return nil, err
	}
	return &unitShim{Unit: u}, nil
}

func (s *stateShim) Resources() (Resources, error) {
	return s.State.Resources()
}

type unitShim struct {
	*corestate.Unit
}

func (u *unitShim) Application() (Application, error) {
	return u.Unit.Application()
}
