// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

import (
	"context"
	"fmt"
	"io"

	"github.com/im7mortal/kmutex"
	jujuerrors "github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/resource"
	"github.com/juju/juju/internal/charm"
	charmresource "github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/state"
)

// ResourceOpenerArgs are common arguments for the 2
// types of ResourceOpeners: for unit and for application.
type ResourceOpenerArgs struct {
	State              *state.State
	ModelConfigService ModelConfigService
	Store              objectstore.ObjectStore
}

// NewResourceOpener returns a new resource.Opener for the given unit.
//
// The caller owns the State provided. It is the caller's
// responsibility to close it.
func NewResourceOpener(
	args ResourceOpenerArgs,
	resourceDownloadLimiterFunc func() ResourceDownloadLock,
	unitName string,
) (opener resource.Opener, err error) {
	// Disable opening resources while the new resource service is
	// being wired up. The old state methods have been removed.
	// TODO: replace error return with newResourceOpener.
	return nil, jujuerrors.NotImplementedf("not implemented")
}

var _ = newResourceOpener

func newResourceOpener(
	args ResourceOpenerArgs,
	resourceDownloadLimiterFunc func() ResourceDownloadLock,
	unitName string,
) (opener resource.Opener, err error) {
	unit, err := args.State.Unit(unitName)
	if err != nil {
		return nil, errors.Errorf("loading unit: %w", err)
	}

	applicationName := unit.ApplicationName()
	application, err := args.State.Application(applicationName)
	if err != nil {
		return nil, errors.Capture(err)
	}

	chURLStr := unit.CharmURL()
	if chURLStr == nil {
		return nil, errors.Errorf("missing charm URL for %q", applicationName)
	}

	charmURL, err := charm.ParseURL(*chURLStr)
	if err != nil {
		return nil, errors.Capture(err)
	}

	return &ResourceOpener{
		state:                       nil, // TODO: provide resource service
		modelUUID:                   args.State.ModelUUID(),
		resourceClientGetter:        newClientGetter(charmURL, args.ModelConfigService),
		retrievedBy:                 unit.Tag(),
		charmURL:                    charmURL,
		charmOrigin:                 *application.CharmOrigin(),
		appName:                     applicationName,
		unitName:                    unitName,
		resourceDownloadLimiterFunc: resourceDownloadLimiterFunc,
	}, nil
}

// NewResourceOpenerForApplication returns a new resource.Opener for the given app.
//
// The caller owns the State provided. It is the caller's
// responsibility to close it.
func NewResourceOpenerForApplication(
	args ResourceOpenerArgs,
	applicationName string,
) (opener resource.Opener, err error) {
	// Disable opening resources while the new resource service is
	// being wired up. The old state methods have been removed.
	// TODO: replace error return with newResourceOpenerForApplication.
	return nil, jujuerrors.NotImplementedf("not implemented")
}

var _ = newResourceOpenerForApplication

func newResourceOpenerForApplication(
	args ResourceOpenerArgs,
	applicationName string,
) (opener resource.Opener, err error) {
	application, err := args.State.Application(applicationName)
	if err != nil {
		return nil, errors.Capture(err)
	}

	chURLStr, _ := application.CharmURL()
	if chURLStr == nil {
		return nil, errors.Errorf("missing charm URL for %q", applicationName)
	}

	charmURL, err := charm.ParseURL(*chURLStr)
	if err != nil {
		return nil, errors.Capture(err)
	}

	return &ResourceOpener{
		state:                nil, // TODO: provide resource service
		modelUUID:            args.State.ModelUUID(),
		resourceClientGetter: newClientGetter(charmURL, args.ModelConfigService),
		retrievedBy:          application.Tag(),
		charmURL:             charmURL,
		charmOrigin:          *application.CharmOrigin(),
		appName:              applicationName,
		unitName:             "",
		resourceDownloadLimiterFunc: func() ResourceDownloadLock {
			return noopDownloadResourceLocker{}
		},
	}, nil
}

func newClientGetter(charmURL *charm.URL, modelConfigService ModelConfigService) resourceClientGetterFunc {
	var clientGetter resourceClientGetterFunc
	switch {
	case charm.CharmHub.Matches(charmURL.Schema):
		clientGetter = newCharmHubOpener(modelConfigService)
	default:
		// Use the no-op opener that returns a not-found error when called.
		clientGetter = newNoopOpener()
	}
	return clientGetter
}

// noopDownloadResourceLocker is a no-op download resource locker.
type noopDownloadResourceLocker struct{}

// Acquire grabs the lock for a given application so long as the
// per-application limit is not exceeded and total across all
// applications does not exceed the global limit.
func (noopDownloadResourceLocker) Acquire(string) {}

// Release releases the lock for the given application.
func (noopDownloadResourceLocker) Release(appName string) {}

type resourceClientGetterFunc func(ctx context.Context) (*ResourceRetryClient, error)

// ResourceOpener is a ResourceOpener for charmhub. It will first look on the
// controller for the requested resource.
type ResourceOpener struct {
	modelUUID   string
	state       DeprecatedResourcesState
	retrievedBy names.Tag
	charmURL    *charm.URL
	charmOrigin state.CharmOrigin
	appName     string
	unitName    string

	resourceClientGetter        resourceClientGetterFunc
	resourceDownloadLimiterFunc func() ResourceDownloadLock
}

// OpenResource implements server.ResourceOpener.
func (ro ResourceOpener) OpenResource(ctx context.Context, name string) (opener resource.Opened, err error) {
	appKey := fmt.Sprintf("%s:%s", ro.modelUUID, ro.appName)
	lock := ro.resourceDownloadLimiterFunc()
	lock.Acquire(appKey)

	done := func() {
		lock.Release(appKey)
	}

	return ro.getResource(ctx, name, done)
}

var resourceMutex = kmutex.New()

// getResource returns a reader for the resource's data.
//
// If the resource is already stored on to the controller then the resource is
// read from there. Otherwise, it is downloaded from charmhub and saved on the
// controller. If the resource cannot be found by name then [errors.NotFound] is
// returned.
func (ro ResourceOpener) getResource(ctx context.Context, resName string, done func()) (opened resource.Opened, err error) {
	defer func() {
		// Call done if not returning a ReadCloser that calls done on Close.
		if err != nil {
			done()
		}
	}()

	lockName := fmt.Sprintf("%s/%s/%s", ro.modelUUID, ro.appName, resName)
	locker := resourceMutex.Locker(lockName)
	locker.Lock()
	defer locker.Unlock()

	// Try and open the resource.
	res, reader, err := ro.open(resName)
	if err != nil && !errors.Is(err, jujuerrors.NotFound) {
		return resource.Opened{}, errors.Capture(err)
	} else if err == nil {
		// If the resource was stored on the controller, return immediately.
		return resource.Opened{
			Resource: res,
			ReadCloser: &resourceAccess{
				ReadCloser: reader,
				done:       done,
			},
		}, nil
	}

	// The resource could not be opened, so may not be stored on the controller,
	// get the resource info and download from charmhub.
	res, err = ro.state.GetResource(ro.appName, resName)
	if err != nil {
		return resource.Opened{}, errors.Capture(err)
	}

	id := CharmID{
		URL:    ro.charmURL,
		Origin: ro.charmOrigin,
	}
	req := ResourceRequest{
		CharmID:  id,
		Name:     res.Name,
		Revision: res.Revision,
	}

	client, err := ro.resourceClientGetter(ctx)
	data, err := client.GetResource(req)
	if errors.Is(err, jujuerrors.NotFound) {
		// A NotFound error might not be detectable from some clients as the
		// error types may be lost after call, for example http. For these
		// cases, the next block will return un-annotated error.
		return resource.Opened{}, errors.Errorf("getting resource from charmhub: %w", err)
	}
	if err != nil {
		return resource.Opened{}, errors.Capture(err)
	}
	res, reader, err = ro.set(data.Resource, data)
	if err != nil {
		return resource.Opened{}, errors.Capture(err)
	}

	return resource.Opened{
		Resource: res,
		ReadCloser: &resourceAccess{
			ReadCloser: reader,
			done:       done,
		},
	}, nil
}

func (ro ResourceOpener) open(resName string) (resource.Resource, io.ReadCloser, error) {
	if ro.unitName == "" {
		return ro.state.OpenResource(ro.appName, resName)
	}
	return ro.state.OpenResourceForUniter(ro.unitName, resName)
}

// set stores the resource info and data on the controller.
// Note that the returned reader may or may not be the same one that was passed
// in.
func (ro ResourceOpener) set(chRes charmresource.Resource, reader io.ReadCloser) (_ resource.Resource, _ io.ReadCloser, err error) {
	defer func() {
		if err != nil {
			// With no err, the reader was closed down in unitSetter Read().
			// Closing here with no error leads to a panic in Read, and the
			// unit's resource doc is never cleared of it's pending status.
			_ = reader.Close()
		}
	}()
	res, err := ro.state.SetResource(ro.appName, ro.retrievedBy.Id(), chRes, reader, false)
	if err != nil {
		return resource.Resource{}, nil, errors.Capture(err)
	}

	// Make sure to use the potentially updated resource details.
	res, reader, err = ro.open(res.Name)
	if err != nil {
		return resource.Resource{}, nil, errors.Capture(err)
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

// noopOpener is a type for creating no resource requests for accessing local
// charm resources.
type noopOpener struct{}

// newNoopOpener creates a new noopOpener that creates a new resourceClient. The new
// noopClient performs no operations for getting resources.
func newNoopOpener() resourceClientGetterFunc {
	no := &noopOpener{}
	return no.NewClient
}

// NewClient opens a new charmhub resourceClient.
func (o *noopOpener) NewClient(context.Context) (*ResourceRetryClient, error) {
	return newRetryClient(noopClient{}), nil
}

// noopClient implements a resourceClient for accessing resources from a given store,
// except this implementation performs no operations and instead returns a
// not-found error. This ensures that no outbound requests are used for
// scenarios covering local charms.
type noopClient struct{}

// GetResource is a no-op resourceClient implementation of a ResourceGetter. The
// implementation expects to never call the underlying resourceClient and instead
// returns a not-found error straight away.
func (noopClient) GetResource(req ResourceRequest) (ResourceData, error) {
	return ResourceData{}, jujuerrors.NotFoundf("resource %q", req.Name)
}
