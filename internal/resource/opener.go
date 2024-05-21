// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

import (
	"fmt"
	"io"

	"github.com/im7mortal/kmutex"
	"github.com/juju/errors"
	"github.com/juju/juju/internal/charm"
	charmresource "github.com/juju/juju/internal/charm/resource"
	"github.com/juju/names/v5"

	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/resources"
	"github.com/juju/juju/state"
)

// NewResourceOpener returns a new resource.Opener for the given unit.
//
// The caller owns the State provided. It is the caller's
// responsibility to close it.
func NewResourceOpener(
	st *state.State, store objectstore.ObjectStore,
	resourceDownloadLimiterFunc func() ResourceDownloadLock, unitName string,
) (opener resources.Opener, err error) {
	return newInternalResourceOpener(st, store, resourceDownloadLimiterFunc, unitName, "")
}

// NewResourceOpenerForApplication returns a new resource.Opener for the given app.
//
// The caller owns the State provided. It is the caller's
// responsibility to close it.
func NewResourceOpenerForApplication(
	st *state.State, store objectstore.ObjectStore,
	applicationName string,
) (opener resources.Opener, err error) {
	return newInternalResourceOpener(st, store, func() ResourceDownloadLock {
		return noopDownloadResourceLocker{}
	}, "", applicationName)
}

// noopDownloadResourceLocker is a no-op download resource locker.
type noopDownloadResourceLocker struct{}

// Acquire grabs the lock for a given application so long as the
// per application limit is not exceeded and total across all
// applications does not exceed the global limit.
func (noopDownloadResourceLocker) Acquire(string) {}

// Release releases the lock for the given application.
func (noopDownloadResourceLocker) Release(appName string) {}

func newInternalResourceOpener(
	st *state.State, store objectstore.ObjectStore,
	resourceDownloadLimiterFunc func() ResourceDownloadLock,
	unitName string, appName string,
) (opener resources.Opener, err error) {
	var unit *state.Unit
	if unitName != "" {
		unit, err = st.Unit(unitName)
		if err != nil {
			return nil, errors.Annotate(err, "loading unit")
		}
	}

	if appName == "" {
		if unit == nil {
			return nil, errors.Errorf("missing both unit and application")
		}
		appName = unit.ApplicationName()
	}
	application, err := st.Application(appName)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var resourceClientGetter interface {
		NewClient() (*ResourceRetryClient, error)
	}

	var chURLStr *string
	if unit != nil {
		chURLStr = unit.CharmURL()
	} else {
		chURLStr, _ = application.CharmURL()
	}
	if chURLStr == nil {
		return nil, errors.Errorf("missing charm URL for %q", appName)
	}
	charmURL, err := charm.ParseURL(*chURLStr)
	if err != nil {
		return nil, errors.Trace(err)
	}
	switch {
	case charm.CharmHub.Matches(charmURL.Schema):
		resourceClientGetter = newCharmHubOpener(st)
	default:
		// Use the nop opener that performs no store side requests. Instead it
		// will resort to using the state package only. Any thing else will call
		// a not-found error.
		resourceClientGetter = newNopOpener()
	}

	resourceClient, err := resourceClientGetter.NewClient()
	if err != nil {
		return nil, errors.Trace(err)
	}

	var userID names.Tag
	if unit != nil {
		userID = unit.Tag()
	} else {
		userID = application.Tag()
	}

	return &ResourceOpener{
		resourceCache:               st.Resources(store),
		modelUUID:                   st.ModelUUID(),
		resourceClient:              resourceClient,
		user:                        userID,
		charmURL:                    charmURL,
		charmOrigin:                 *application.CharmOrigin(),
		appName:                     appName,
		unitName:                    unitName,
		resourceDownloadLimiterFunc: resourceDownloadLimiterFunc,
	}, nil
}

// ResourceOpener is a ResourceOpener for charmhub.
// It will first look in the supplied cache for the
// requested resource.
type ResourceOpener struct {
	modelUUID     string
	resourceCache Resources
	user          names.Tag
	charmURL      *charm.URL
	charmOrigin   state.CharmOrigin
	appName       string
	unitName      string

	resourceClient              ResourceGetter
	resourceDownloadLimiterFunc func() ResourceDownloadLock
}

// OpenResource implements server.ResourceOpener.
func (ro *ResourceOpener) OpenResource(name string) (o resources.Opened, err error) {
	if ro.appName == "" {
		return resources.Opened{}, errors.Errorf("missing application")
	}

	appKey := fmt.Sprintf("%s:%s", ro.modelUUID, ro.appName)
	limiter := ro.resourceDownloadLimiterFunc()
	limiter.Acquire(appKey)

	done := func() {
		limiter.Release(appKey)
	}
	res, reader, err := ro.getResource(name, done)
	if err != nil {
		return resources.Opened{}, errors.Trace(err)
	}

	opened := resources.Opened{
		Resource:   res,
		ReadCloser: reader,
	}
	return opened, nil
}

// TODO(juju3): use raft to lock the resource for writes.
var resourceMutex = kmutex.New()

// GetResource returns a reader for the resource's data. That data is
// streamed from charmhub.
//
// If a cache is set up then the resource is read from there. If the
// resource is not in the cache at all then errors.NotFound is returned.
// If only the resource's details are in the cache (but not the actual
// file) then the file is read from charmhub. In that case the
// cache is updated to contain the file too.
func (ro ResourceOpener) getResource(resName string, done func()) (_ resources.Resource, rdr io.ReadCloser, err error) {
	defer func() {
		if err == nil {
			rdr = &resourceAccess{
				ReadCloser: rdr,
				done:       done,
			}
		} else {
			done()
		}
	}()

	lockName := fmt.Sprintf("%s/%s/%s", ro.modelUUID, ro.appName, resName)
	locker := resourceMutex.Locker(lockName)
	locker.Lock()
	defer locker.Unlock()

	res, reader, err := ro.get(resName)
	if err != nil {
		return resources.Resource{}, nil, errors.Trace(err)
	}
	if reader != nil {
		// Both the info *and* the data were found in the cache.
		return res, reader, nil
	}

	// Otherwise, just the info was found in the cache. So we read the
	// data from charmhub through a new resourceClient and set the data
	// for the resource in the cache.

	id := CharmID{
		URL:    ro.charmURL,
		Origin: ro.charmOrigin,
	}
	req := ResourceRequest{
		CharmID:  id,
		Name:     res.Name,
		Revision: res.Revision,
	}
	data, err := ro.resourceClient.GetResource(req)
	// (anastasiamac 2017-05-25) This might not work all the time
	// as the error types may be lost after call to some clients, for example http.
	// But for these cases, the next block will bubble an un-annotated error up.
	if errors.Is(err, errors.NotFound) {
		msg := "while getting resource from charmhub"
		return resources.Resource{}, nil, errors.Annotate(err, msg)
	}
	if err != nil {
		return resources.Resource{}, nil, errors.Trace(err)
	}
	res, reader, err = ro.set(data.Resource, data, state.DoNotIncrementCharmModifiedVersion)
	if err != nil {
		return resources.Resource{}, nil, errors.Trace(err)
	}

	return res, reader, nil
}

// get retrieves the resource info and data from a repo. If only
// the info is found then the returned reader will be nil. If a
// repo is not in use then errors.NotFound is returned.
func (ro ResourceOpener) get(name string) (resources.Resource, io.ReadCloser, error) {
	if ro.resourceCache == nil {
		return resources.Resource{}, nil, errors.NotFoundf("resource %q", name)
	}

	res, reader, err := ro.open(name)
	if errors.Is(err, errors.NotFound) {
		reader = nil
		res, err = ro.resourceCache.GetResource(ro.appName, name)
	}
	if err != nil {
		return resources.Resource{}, nil, errors.Trace(err)
	}

	return res, reader, nil
}

func (ro ResourceOpener) open(resName string) (resources.Resource, io.ReadCloser, error) {
	if ro.unitName == "" {
		return ro.resourceCache.OpenResource(ro.appName, resName)
	}
	return ro.resourceCache.OpenResourceForUniter(ro.unitName, resName)
}

// set stores the resource info and data in a repo, if there is one.
// If no repo is in use then this is a no-op. Note that the returned
// reader may or may not be the same one that was passed in.
func (ro ResourceOpener) set(chRes charmresource.Resource, reader io.ReadCloser, incrementCharmModifiedVersion state.IncrementCharmModifiedVersionType) (_ resources.Resource, _ io.ReadCloser, err error) {
	if ro.resourceCache == nil {
		res := resources.Resource{
			Resource: chRes,
		}
		return res, reader, nil // a no-op
	}
	defer func() {
		if err != nil {
			// With no err, the reader was closed down in unitSetter Read().
			// Closing here with no error leads to a panic in Read, and the
			// unit's resource doc is never cleared of it's pending status.
			_ = reader.Close()
		}
	}()
	appName := ro.appName
	res, err := ro.resourceCache.SetResource(appName, ro.user.Id(), chRes, reader, incrementCharmModifiedVersion)
	if err != nil {
		return resources.Resource{}, nil, errors.Trace(err)
	}

	// Make sure to use the potentially updated resource details.
	res, reader, err = ro.open(res.Name)
	if err != nil {
		return resources.Resource{}, nil, errors.Trace(err)
	}

	return res, reader, nil
}

type resourceAccess struct {
	io.ReadCloser
	done func()
}

func (r *resourceAccess) Close() error {
	defer r.done()
	return r.ReadCloser.Close()
}

type ResourceRequest struct {
	// Channel is the channel from which to request the resource info.
	CharmID CharmID

	// Name is the name of the resource we're asking about.
	Name string

	// Revision is the specific revision of the resource we're asking about.
	Revision int
}

// CharmID represents the underlying charm for a given application. This
// includes both the URL and the origin.
type CharmID struct {

	// URL of the given charm, includes the reference name and a revision.
	// Old style charm URLs are also supported i.e. charmstore.
	URL *charm.URL

	// Origin holds the origin of a charm. This includes the source of the
	// charm, along with the revision and channel to identify where the charm
	// originated from.
	Origin state.CharmOrigin
}

// nopOpener is a type for creating no resource requests for accessing local
// charm resources.
type nopOpener struct{}

// newNopOpener creates a new nopOpener that creates a new resourceClient. The new
// nopClient performs no operations for getting resources.
func newNopOpener() *nopOpener {
	return &nopOpener{}
}

// NewClient opens a new charmhub resourceClient.
func (o *nopOpener) NewClient() (*ResourceRetryClient, error) {
	return newRetryClient(nopClient{}), nil
}

// nopClient implements a resourceClient for accessing resources from a given store,
// except this implementation performs no operations and instead returns a
// not-found error. This ensures that no outbound requests are used for
// scenarios covering local charms.
type nopClient struct{}

// GetResource is a no-op resourceClient implementation of a ResourceGetter. The
// implementation expects to never call the underlying resourceClient and instead
// returns a not-found error straight away.
func (nopClient) GetResource(req ResourceRequest) (ResourceData, error) {
	return ResourceData{}, errors.NotFoundf("resource %q", req.Name)
}
