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
	corelogger "github.com/juju/juju/core/logger"
	coreresource "github.com/juju/juju/core/resource"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/resource"
	resourceerrors "github.com/juju/juju/domain/resource/errors"
	"github.com/juju/juju/internal/charm"
	charmresource "github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/errors"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/internal/resource/charmhub"
	"github.com/juju/juju/state"
)

var resourceLogger = internallogger.GetLogger("juju.resource")

// ResourceOpenerArgs are common arguments for the 2
// types of ResourceOpeners: for unit and for application.
type ResourceOpenerArgs struct {
	State                *state.State
	ResourceService      ResourceService
	ModelConfigService   ModelConfigService
	ApplicationService   ApplicationService
	CharmhubClientGetter ResourceClientGetter
}

// NewResourceOpenerForUnit returns a new resource.Opener for the given unit.
//
// The caller owns the State provided. It is the caller's
// responsibility to close it.
func NewResourceOpenerForUnit(
	ctx context.Context,
	args ResourceOpenerArgs,
	resourceDownloadLimiterFunc func() ResourceDownloadLock,
	unitName coreunit.Name,
) (opener coreresource.Opener, err error) {
	return newResourceOpenerForUnit(
		ctx,
		stateShim{args.State},
		args,
		resourceDownloadLimiterFunc,
		unitName,
	)
}

func newResourceOpenerForUnit(
	ctx context.Context,
	state DeprecatedState,
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
	unit, err := state.Unit(unitName.String())
	if err != nil {
		return nil, errors.Errorf("loading unit from state: %w", err)
	}

	applicationName := unit.ApplicationName()
	application, err := state.Application(applicationName)
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
		modelUUID:            state.ModelUUID(),
		resourceClientGetter: newClientGetter(charmURL, args.CharmhubClientGetter),
		retrievedBy:          unitName.String(),
		retrievedByType:      coreresource.Unit,
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
	return newResourceOpenerForApplication(
		ctx,
		stateShim{args.State},
		args,
		applicationName,
	)
}

func newResourceOpenerForApplication(
	ctx context.Context,
	state DeprecatedState,
	args ResourceOpenerArgs,
	applicationName string,
) (opener coreresource.Opener, err error) {
	// TODO(aflynn): we still get the charm URL from state since functionality
	// for this has not yet been implemented in the domain.
	application, err := state.Application(applicationName)
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
		modelUUID:            state.ModelUUID(),
		resourceClientGetter: newClientGetter(charmURL, args.CharmhubClientGetter),
		retrievedBy:          applicationName,
		retrievedByType:      coreresource.Application,
		setResourceFunc: func(ctx context.Context, resourceUUID coreresource.UUID) error {
			// noop
			return nil
		},
		charmURL:    charmURL,
		charmOrigin: *application.CharmOrigin(),
		appName:     applicationName,
		appID:       applicationID,
		resourceDownloadLimiterFunc: func() ResourceDownloadLock {
			return noopDownloadResourceLocker{}
		},
	}, nil
}

func newClientGetter(
	charmURL *charm.URL,
	charmhubClientGetter ResourceClientGetter,
) ResourceClientGetter {
	var clientGetter ResourceClientGetter
	switch {
	case charm.CharmHub.Matches(charmURL.Schema):
		clientGetter = charmhubClientGetter
	default:
		// Use the no-op opener that returns a not-found error when called.
		clientGetter = newNoopOpener()
	}
	return clientGetter
}

// ResourceOpener is a ResourceOpener for charmhub. It will first look on the
// controller for the requested resource.
type ResourceOpener struct {
	modelUUID       string
	resourceService ResourceService
	retrievedBy     string
	retrievedByType coreresource.RetrievedByType
	setResourceFunc func(ctx context.Context, resourceUUID coreresource.UUID) error
	charmURL        *charm.URL
	charmOrigin     state.CharmOrigin
	appName         string
	appID           coreapplication.ID

	resourceClientGetter        ResourceClientGetter
	resourceDownloadLimiterFunc func() ResourceDownloadLock
}

// OpenResource implements server.ResourceOpener.
func (ro ResourceOpener) OpenResource(ctx context.Context, name string) (opener coreresource.Opened, err error) {
	lock := ro.resourceDownloadLimiterFunc()

	appKey := fmt.Sprintf("%s:%s", ro.modelUUID, ro.appName)
	if err := lock.Acquire(ctx, appKey); err != nil {
		return coreresource.Opened{}, errors.Errorf("acquiring resource download lock for %s: %w", appKey, err)
	}

	return ro.getResource(ctx, name, func() {
		lock.Release(appKey)
	})
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

	resourceUUID, err := ro.resourceService.GetApplicationResourceID(ctx, resource.GetApplicationResourceIDArgs{
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

	id := charmhub.CharmID{
		URL:    ro.charmURL,
		Origin: ro.charmOrigin,
	}
	req := charmhub.ResourceRequest{
		CharmID:  id,
		Name:     res.Name,
		Revision: res.Revision,
	}

	client, err := ro.resourceClientGetter.GetResourceClient(ctx, resourceLogger)
	if err != nil {
		return coreresource.Opened{}, errors.Capture(err)
	}
	data, err := client.GetResource(ctx, req)
	if errors.Is(err, jujuerrors.NotFound) {
		// A NotFound error might not be detectable from some clients as the
		// error types may be lost after call, for example http. For these
		// cases, the next block will return un-annotated error.
		return coreresource.Opened{}, errors.Errorf("getting resource from charmhub: %w", err)
	}
	if err != nil {
		return coreresource.Opened{}, errors.Capture(err)
	}
	defer data.ReadCloser.Close()

	res, reader, err = ro.store(
		ctx,
		resourceUUID,
		data.ReadCloser,
		data.Resource.Size,
		data.Resource.Fingerprint,
	)
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
func (ro ResourceOpener) store(
	ctx context.Context,
	resourceUUID coreresource.UUID,
	reader io.Reader,
	size int64,
	fingerprint charmresource.Fingerprint,
) (_ coreresource.Resource, _ io.ReadCloser, err error) {
	err = ro.resourceService.StoreResource(
		ctx, resource.StoreResourceArgs{
			ResourceUUID:    resourceUUID,
			Reader:          reader,
			Size:            size,
			Fingerprint:     fingerprint,
			RetrievedBy:     ro.retrievedBy,
			RetrievedByType: ro.retrievedByType,
		},
	)
	if err != nil {
		return coreresource.Resource{}, nil, errors.Capture(err)
	}

	// Make sure to use the potentially updated resource details.
	res, opened, err := ro.resourceService.OpenResource(ctx, resourceUUID)
	if err != nil {
		return coreresource.Resource{}, nil, errors.Capture(err)
	}

	return res, opened, nil
}

// SetResourceUsed records that the resource is currently in use.
func (ro ResourceOpener) SetResourceUsed(ctx context.Context, resName string) error {
	resourceUUID, err := ro.resourceService.GetApplicationResourceID(ctx, resource.GetApplicationResourceIDArgs{
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

// noopOpener is a type for creating no resource requests for accessing local
// charm resources.
type noopOpener struct{}

type noopResourceClientGetter func(ctx context.Context, logger corelogger.Logger) (charmhub.ResourceClient, error)

func (rcg noopResourceClientGetter) GetResourceClient(ctx context.Context, logger corelogger.Logger) (charmhub.ResourceClient, error) {
	return rcg(ctx, logger)
}

// newNoopOpener creates a new noopOpener that creates a new resourceClient. The new
// noopClient performs no operations for getting resources.
func newNoopOpener() noopResourceClientGetter {
	no := &noopOpener{}
	return no.NewClient
}

// NewClient opens a new charmhub resourceClient.
func (o *noopOpener) NewClient(_ context.Context, logger corelogger.Logger) (charmhub.ResourceClient, error) {
	return charmhub.NewRetryClient(noopClient{}, logger), nil
}

// noopClient implements a resourceClient for accessing resources from a given store,
// except this implementation performs no operations and instead returns a
// not-found error. This ensures that no outbound requests are used for
// scenarios covering local charms.
type noopClient struct{}

// GetResource is a no-op resourceClient implementation of a ResourceGetter. The
// implementation expects to never call the underlying resourceClient and instead
// returns a not-found error straight away.
func (noopClient) GetResource(_ context.Context, req charmhub.ResourceRequest) (charmhub.ResourceData, error) {
	return charmhub.ResourceData{}, jujuerrors.NotFoundf("resource %q", req.Name)
}

type stateShim struct {
	*state.State
}

func (s stateShim) Unit(name string) (DeprecatedStateUnit, error) {
	u, err := s.State.Unit(name)
	if err != nil {
		return nil, err
	}
	return &unitShim{Unit: u}, nil
}

func (s stateShim) ModelUUID() string {
	return s.State.ModelUUID()
}

func (s stateShim) Application(name string) (DeprecatedStateApplication, error) {
	a, err := s.State.Application(name)
	if err != nil {
		return nil, err
	}
	return &applicationShim{Application: a}, nil
}

type applicationShim struct {
	*state.Application
}

type unitShim struct {
	*state.Unit
}
