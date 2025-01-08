// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

import (
	"context"
	"fmt"
	"io"

	"github.com/im7mortal/kmutex"
	jujuerrors "github.com/juju/errors"

	coreapplication "github.com/juju/juju/core/application"
	coreresource "github.com/juju/juju/core/resource"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/resource"
	resourceerrors "github.com/juju/juju/domain/resource/errors"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/state"
)

// ResourceOpenerArgs are common arguments for the 2
// types of ResourceOpeners: for unit and for application.
type ResourceOpenerArgs struct {
	State              *state.State
	ResourceService    ResourceService
	ModelConfigService ModelConfigService
	ApplicationService ApplicationService
}

// NewResourceOpener returns a new resource.Opener for the given unit.
//
// The caller owns the State provided. It is the caller's
// responsibility to close it.
func NewResourceOpener(
	ctx context.Context,
	args ResourceOpenerArgs,
	resourceDownloadLimiterFunc func() ResourceDownloadLock,
	unitName coreunit.Name,
) (opener coreresource.Opener, err error) {
	applicationID, err := args.ApplicationService.GetApplicationIDByUnitName(ctx, unitName)
	if err != nil {
		return nil, errors.Errorf("loading application ID for unit %s: %w", unitName, err)
	}

	unitUUID, err := args.ApplicationService.GetUnitUUID(ctx, unitName)
	if err != nil {
		return nil, errors.Errorf("loading application ID for unit %s: %w", unitName, err)
	}

	// TODO(aflynn): we still get the charm URL from state since functionality
	// for this has not yet been implemented in the domain. When it has, we need
	// to get the charm specifically for the unit here, not the application, as
	// it could be on an older version.
	unit, err := args.State.Unit(unitName.String())
	if err != nil {
		return nil, errors.Errorf("loading unit from state: %w", err)
	}

	applicationName := unit.ApplicationName()
	application, err := args.State.Application(applicationName)
	if err != nil {
		return nil, errors.Capture(err)
	}

	chURLStr := unit.CharmURL()
	if chURLStr == nil {
		return nil, errors.Errorf("missing charm URL for %q", unitName)
	}

	charmURL, err := charm.ParseURL(*chURLStr)
	if err != nil {
		return nil, errors.Capture(err)
	}

	return &ResourceOpener{
		resourceService:      args.ResourceService,
		modelUUID:            args.State.ModelUUID(),
		resourceClientGetter: newClientGetter(charmURL, args.ModelConfigService),
		retrievedBy:          unitName.String(),
		retrievedByType:      resource.Unit,
		setResourceFunc: func(ctx context.Context, resourceUUID coreresource.UUID) error {
			return args.ResourceService.SetUnitResource(ctx, resourceUUID, unitUUID)
		},
		charmURL:                    charmURL,
		charmOrigin:                 *application.CharmOrigin(),
		appName:                     applicationName,
		appID:                       applicationID,
		resourceDownloadLimiterFunc: resourceDownloadLimiterFunc,
	}, nil
}

// NewResourceOpenerForApplication returns a new resource.Opener for the given app.
//
// The caller owns the State provided. It is the caller's
// responsibility to close it.
func NewResourceOpenerForApplication(
	ctx context.Context,
	args ResourceOpenerArgs,
	applicationName string,
) (opener coreresource.Opener, err error) {
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

	applicationID, err := args.ApplicationService.GetApplicationIDByName(ctx, applicationName)
	if err != nil {
		return nil, errors.Errorf("getting ID of application %s: %w", applicationName, err)
	}

	return &ResourceOpener{
		resourceService:      args.ResourceService,
		modelUUID:            args.State.ModelUUID(),
		resourceClientGetter: newClientGetter(charmURL, args.ModelConfigService),
		retrievedBy:          applicationName,
		retrievedByType:      resource.Application,
		setResourceFunc:      args.ResourceService.SetApplicationResource,
		charmURL:             charmURL,
		charmOrigin:          *application.CharmOrigin(),
		appName:              applicationName,
		appID:                applicationID,
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

type resourceClientGetterFunc func(ctx context.Context) (*ResourceRetryClient, error)

// noopDownloadResourceLocker is a no-op download resource locker.
type noopDownloadResourceLocker struct{}

// Acquire grabs the lock for a given application so long as the
// per-application limit is not exceeded and total across all
// applications does not exceed the global limit.
func (noopDownloadResourceLocker) Acquire(string) {}

// Release releases the lock for the given application.
func (noopDownloadResourceLocker) Release(appName string) {}

// ResourceOpener is a ResourceOpener for charmhub. It will first look on the
// controller for the requested resource.
type ResourceOpener struct {
	modelUUID       string
	resourceService ResourceService
	retrievedBy     string
	retrievedByType resource.RetrievedByType
	setResourceFunc func(ctx context.Context, resourceUUID coreresource.UUID) error
	charmURL        *charm.URL
	charmOrigin     state.CharmOrigin
	appName         string
	appID           coreapplication.ID

	resourceClientGetter        resourceClientGetterFunc
	resourceDownloadLimiterFunc func() ResourceDownloadLock
}

// OpenResource implements server.ResourceOpener.
func (ro ResourceOpener) OpenResource(ctx context.Context, name string) (opener coreresource.Opened, err error) {
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
func (ro ResourceOpener) getResource(
	ctx context.Context,
	resName string,
	done func(),
) (opened coreresource.Opened, err error) {
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

	resourceUUID, err := ro.resourceService.GetResourceUUID(ctx, resource.GetResourceUUIDArgs{
		ApplicationID: ro.appID,
		Name:          resName,
	})
	if err != nil {
		return coreresource.Opened{}, errors.Errorf("getting UUID of resource %s for application %s: %w", resName, ro.appName, err)
	}

	res, reader, err := ro.resourceService.OpenResource(ctx, resourceUUID)
	if err != nil && !errors.Is(err, resourceerrors.StoredResourceNotFound) {
		return coreresource.Opened{}, errors.Capture(err)
	} else if err == nil {
		// If the resource was stored on the controller, return immediately.
		return coreresource.Opened{
			Resource: coreresource.Resource{
				Resource: res.Resource,
			},
			ReadCloser: &resourceAccess{
				ReadCloser: reader,
				done:       done,
			},
		}, nil
	}

	// The resource could not be opened, so may not be stored on the controller,
	// get the resource info and download from charmhub.
	res, err = ro.resourceService.GetResource(ctx, resourceUUID)
	if err != nil {
		return coreresource.Opened{}, errors.Capture(err)
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
		return coreresource.Opened{}, errors.Errorf("getting resource from charmhub: %w", err)
	}
	if err != nil {
		return coreresource.Opened{}, errors.Capture(err)
	}
	res, reader, err = ro.store(ctx, resourceUUID, data)
	if err != nil {
		return coreresource.Opened{}, errors.Capture(err)
	}

	return coreresource.Opened{
		Resource: coreresource.Resource{
			Resource: res.Resource,
		},
		ReadCloser: &resourceAccess{
			ReadCloser: reader,
			done:       done,
		},
	}, nil
}

// store stores the resource info and data on the controller.
// Note that the returned reader may or may not be the same one that was passed
// in.
func (ro ResourceOpener) store(ctx context.Context, resourceUUID coreresource.UUID, reader io.ReadCloser) (_ resource.Resource, _ io.ReadCloser, err error) {
	defer func() {
		if err != nil {
			// With no err, the reader was closed down in unitSetter Read().
			// Closing here with no error leads to a panic in Read, and the
			// unit's resource doc is never cleared of it's pending status.
			_ = reader.Close()
		}
	}()

	err = ro.resourceService.StoreResource(ctx, resource.StoreResourceArgs{
		ResourceUUID:    resourceUUID,
		Reader:          reader,
		RetrievedBy:     ro.retrievedBy,
		RetrievedByType: ro.retrievedByType,
	})
	if err != nil {
		return resource.Resource{}, nil, errors.Capture(err)
	}

	// Make sure to use the potentially updated resource details.
	res, reader, err := ro.resourceService.OpenResource(ctx, resourceUUID)
	if err != nil {
		return resource.Resource{}, nil, errors.Capture(err)
	}

	return res, reader, nil
}

// SetResource records that the resource is currently in use.
func (ro ResourceOpener) SetResource(ctx context.Context, resName string) error {
	resourceUUID, err := ro.resourceService.GetResourceUUID(ctx, resource.GetResourceUUIDArgs{
		ApplicationID: ro.appID,
		Name:          resName,
	})
	if err != nil {
		return errors.Errorf("getting UUID of resource %s for application %s: %w", resName, ro.appName, err)
	}

	err = ro.setResourceFunc(ctx, resourceUUID)
	if err != nil {
		return errors.Errorf("setting resource %s on application %s: %w", resName, ro.appName, err)
	}

	return nil
}

// resourceAccess wraps the reader for the resource calling the done function on
// Close.
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
