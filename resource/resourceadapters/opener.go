// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resourceadapters

import (
	"io"

	"github.com/juju/charm/v9"
	charmresource "github.com/juju/charm/v9/resource"
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/charmstore"
	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/repositories"
	corestate "github.com/juju/juju/state"
)

// NewResourceOpener returns a new resource.Opener for the given unit.
//
// The caller owns the State provided. It is the caller's
// responsibility to close it.
func NewResourceOpener(st ResourceOpenerState, unitName string) (opener resource.Opener, err error) {
	return newInternalResourceOpener(st, unitName, "")
}

// NewResourceOpenerForApplication returns a new resource.Opener for the given app.
//
// The caller owns the State provided. It is the caller's
// responsibility to close it.
func NewResourceOpenerForApplication(st ResourceOpenerState, applicationName string) (opener resource.Opener, err error) {
	return newInternalResourceOpener(st, "", applicationName)
}

// NewResourceOpenerState wraps a State pointer for passing into NewResourceOpener/NewResourceOpenerForApplication.
func NewResourceOpenerState(st *corestate.State) ResourceOpenerState {
	return &stateShim{st}
}

func newInternalResourceOpener(st ResourceOpenerState, unitName string, applicationName string) (opener resource.Opener, err error) {
	unit := Unit(nil)
	if unitName != "" {
		unit, err = st.Unit(unitName)
		if err != nil {
			return nil, errors.Annotate(err, "loading unit")
		}
	}

	if applicationName == "" {
		if unit == nil {
			return nil, errors.Errorf("missing both unit and application")
		}
		applicationName = unit.ApplicationName()
	}
	application, err := st.Application(applicationName)
	if err != nil {
		return nil, errors.Trace(err)
	}

	resources, err := st.Resources()
	if err != nil {
		return nil, errors.Trace(err)
	}

	var resourceClient ResourceRetryClientGetterFn

	charmURL := (*charm.URL)(nil)
	if unit != nil {
		charmURL, _ = unit.CharmURL()
	} else {
		charmURL, _ = application.CharmURL()
	}
	switch {
	case charm.CharmHub.Matches(charmURL.Schema):
		resourceClient = func(st ResourceOpenerState) ResourceRetryClientGetter {
			return newCharmHubOpener(st)
		}
	case charm.CharmStore.Matches(charmURL.Schema):
		resourceClient = func(st ResourceOpenerState) ResourceRetryClientGetter {
			return newCharmStoreOpener(st)
		}
	default:
		// Use the nop opener that performs no store side requests. Instead it
		// will resort to using the state package only. Any thing else will call
		// a not-found error.
		resourceClient = func(st ResourceOpenerState) ResourceRetryClientGetter {
			return newNopOpener()
		}
	}

	var userID names.Tag
	if unit != nil {
		userID = unit.Tag()
	} else {
		userID = application.Tag()
	}

	return &ResourceOpener{
		st:                st,
		res:               resources,
		userID:            userID,
		unit:              unit,
		application:       application,
		newResourceOpener: resourceClient,
	}, nil
}

// ResourceOpener is a ResourceOpener for the charm store.
type ResourceOpener struct {
	st          ResourceOpenerState
	res         Resources
	userID      names.Tag
	unit        Unit
	application Application

	newResourceOpener func(st ResourceOpenerState) ResourceRetryClientGetter
}

// OpenResource implements server.ResourceOpener.
func (ro *ResourceOpener) OpenResource(name string) (o resource.Opened, err error) {
	if ro.application == nil {
		return resource.Opened{}, errors.Errorf("missing application")
	}

	var charmURL *charm.URL
	if ro.unit != nil {
		charmURL, _ = ro.unit.CharmURL()
	} else {
		charmURL, _ = ro.application.CharmURL()
	}

	id := repositories.CharmID{
		URL:    charmURL,
		Origin: *ro.application.CharmOrigin(),
	}

	client, err := ro.newResourceOpener(ro.st).NewClient()
	if err != nil {
		return resource.Opened{}, errors.Trace(err)
	}

	st := &resourceState{
		st:          ro.res,
		userID:      ro.userID,
		unit:        ro.unit,
		application: ro.application,
	}

	res, reader, err := repositories.GetResource(repositories.GetResourceArgs{
		Client:     client,
		Repository: st,
		CharmID:    id,
		Name:       name,
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

// resourceState adapts between resource state and charmstore.EntityCache.
type resourceState struct {
	st          Resources
	userID      names.Tag
	unit        resource.Unit
	application Application
}

// GetResource implements charmstore.EntityCache.
func (s *resourceState) GetResource(name string) (resource.Resource, error) {
	return s.st.GetResource(s.application.Name(), name)
}

// SetResource implements charmstore.EntityCache.
func (s *resourceState) SetResource(chRes charmresource.Resource, reader io.Reader, incrementCharmModifiedVersion corestate.IncrementCharmModifiedVersionType) (resource.Resource, error) {
	return s.st.SetResource(s.application.Name(), s.userID.Id(), chRes, reader, incrementCharmModifiedVersion)
}

// OpenResource implements charmstore.EntityCache.
func (s *resourceState) OpenResource(name string) (resource.Resource, io.ReadCloser, error) {
	if s.unit == nil {
		return s.st.OpenResource(s.application.Name(), name)
	}
	return s.st.OpenResourceForUniter(s.unit, name)
}

// nopOpener is a type for creating no resource requests for accessing local
// charm resources.
type nopOpener struct{}

// newNopOpener creates a new nopOpener that creates a new client. The new
// nopClient performs no operations for getting resources.
func newNopOpener() *nopOpener {
	return &nopOpener{}
}

// NewClient opens a new charm store client.
func (o *nopOpener) NewClient() (*ResourceRetryClient, error) {
	return newRetryClient(nopClient{}), nil
}

// nopClient implements a client for accessing resources from a given store,
// except this implementation performs no operations and instead returns a
// not-found error. This ensures that no outbound requests are used for
// scenarios covering local charms.
type nopClient struct{}

// GetResource is a no-op client implementation of a ResourceClient. The
// implementation expects to never call the underlying client and instead
// returns a not-found error straight away.
func (nopClient) GetResource(req repositories.ResourceRequest) (charmstore.ResourceData, error) {
	return charmstore.ResourceData{}, errors.NotFoundf("resource %q", req.Name)
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

func (s *stateShim) Application(applicationName string) (Application, error) {
	return s.State.Application(applicationName)
}

type unitShim struct {
	*corestate.Unit
}

func (u *unitShim) Application() (Application, error) {
	return u.Unit.Application()
}
