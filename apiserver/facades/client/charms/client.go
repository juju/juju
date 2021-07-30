// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms

import (
	"io/ioutil"
	"os"
	"strings"
	"sync"

	"github.com/juju/charm/v9"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/http/v2"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"github.com/juju/os/v2/series"
	"gopkg.in/macaroon.v2"

	apiresources "github.com/juju/juju/api/resources"
	charmscommon "github.com/juju/juju/apiserver/common/charms"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	charmsinterfaces "github.com/juju/juju/apiserver/facades/client/charms/interfaces"
	"github.com/juju/juju/apiserver/facades/client/charms/services"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/charmhub"
	"github.com/juju/juju/core/arch"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/storage"
)

var logger = loggo.GetLogger("juju.apiserver.charms")

// API implements the charms interface and is the concrete
// implementation of the API end point.
type API struct {
	charmInfoAPI *charmscommon.CharmInfoAPI
	authorizer   facade.Authorizer
	backendState charmsinterfaces.BackendState
	backendModel charmsinterfaces.BackendModel

	tag        names.ModelTag
	httpClient http.HTTPClient

	newStorage     func(modelUUID string) services.Storage
	newDownloader  func(services.CharmDownloaderConfig) (charmsinterfaces.Downloader, error)
	newRepoFactory func(services.CharmRepoFactoryConfig) corecharm.RepositoryFactory

	mu          sync.Mutex
	repoFactory corecharm.RepositoryFactory
}

type APIv2 struct {
	*APIv3
}

type APIv3 struct {
	*API
}

// CharmInfo returns information about the requested charm.
func (a *API) CharmInfo(args params.CharmURL) (params.Charm, error) {
	return a.charmInfoAPI.CharmInfo(args)
}

func (a *API) checkCanRead() error {
	canRead, err := a.authorizer.HasPermission(permission.ReadAccess, a.tag)
	if err != nil {
		return errors.Trace(err)
	}
	if !canRead {
		return apiservererrors.ErrPerm
	}
	return nil
}

func (a *API) checkCanWrite() error {
	isAdmin, err := a.authorizer.HasPermission(permission.SuperuserAccess, a.backendState.ControllerTag())
	if err != nil {
		return errors.Trace(err)
	}

	canWrite, err := a.authorizer.HasPermission(permission.WriteAccess, a.tag)
	if err != nil {
		return errors.Trace(err)
	}
	if !canWrite && !isAdmin {
		return apiservererrors.ErrPerm
	}
	return nil
}

// NewFacadeV2 provides the signature required for facade V2 registration.
// It is unknown where V1 is.
func NewFacadeV2(ctx facade.Context) (*APIv2, error) {
	v4, err := NewFacadeV4(ctx)
	if err != nil {
		return nil, nil
	}
	return &APIv2{
		APIv3: &APIv3{
			API: v4,
		},
	}, nil
}

// NewFacadeV3 provides the signature required for facade V3 registration.
func NewFacadeV3(ctx facade.Context) (*APIv3, error) {
	api, err := NewFacadeV4(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv3{API: api}, nil
}

// NewFacadeV4 provides the signature required for facade V4 registration.
func NewFacadeV4(ctx facade.Context) (*API, error) {
	authorizer := ctx.Auth()
	if !authorizer.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}

	st := ctx.State()
	m, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}

	commonState := &charmscommon.StateShim{st}
	charmInfoAPI, err := charmscommon.NewCharmInfoAPI(commonState, authorizer)
	if err != nil {
		return nil, errors.Trace(err)
	}

	httpTransport := charmhub.RequestHTTPTransport(ctx.RequestRecorder(), charmhub.DefaultRetryPolicy())

	return &API{
		charmInfoAPI: charmInfoAPI,
		authorizer:   authorizer,
		backendState: newStateShim(st),
		backendModel: m,
		newStorage: func(modelUUID string) services.Storage {
			return storage.NewStorage(modelUUID, st.MongoSession())
		},
		newRepoFactory: func(cfg services.CharmRepoFactoryConfig) corecharm.RepositoryFactory {
			return services.NewCharmRepoFactory(cfg)
		},
		newDownloader: func(cfg services.CharmDownloaderConfig) (charmsinterfaces.Downloader, error) {
			return services.NewCharmDownloader(cfg)
		},
		tag:        m.ModelTag(),
		httpClient: httpTransport(logger),
	}, nil
}

// NewCharmsAPI is only used for testing.
// TODO (stickupkid): We should use the latest NewFacadeV4 to better exercise
// the API.
func NewCharmsAPI(
	authorizer facade.Authorizer,
	st charmsinterfaces.BackendState,
	m charmsinterfaces.BackendModel,
	newStorage func(modelUUID string) services.Storage,
	repoFactory corecharm.RepositoryFactory,
	newDownloader func(cfg services.CharmDownloaderConfig) (charmsinterfaces.Downloader, error),
) (*API, error) {
	return &API{
		authorizer:    authorizer,
		backendState:  st,
		backendModel:  m,
		newStorage:    newStorage,
		repoFactory:   repoFactory,
		newDownloader: newDownloader,
		tag:           m.ModelTag(),
		httpClient:    charmhub.DefaultHTTPTransport(logger),
	}, nil
}

// List returns a list of charm URLs currently in the state.
// If supplied parameter contains any names, the result will
// be filtered to return only the charms with supplied names.
func (a *API) List(args params.CharmsList) (params.CharmsListResult, error) {
	logger.Tracef("List %+v", args)
	if err := a.checkCanRead(); err != nil {
		return params.CharmsListResult{}, errors.Trace(err)
	}

	charms, err := a.backendState.AllCharms()
	if err != nil {
		return params.CharmsListResult{}, errors.Annotatef(err, " listing charms ")
	}

	charmNames := set.NewStrings(args.Names...)
	checkName := !charmNames.IsEmpty()
	charmURLs := []string{}
	for _, aCharm := range charms {
		charmURL := aCharm.URL()
		if checkName {
			if !charmNames.Contains(charmURL.Name) {
				continue
			}
		}
		charmURLs = append(charmURLs, charmURL.String())
	}
	return params.CharmsListResult{CharmURLs: charmURLs}, nil
}

// GetDownloadInfos is not available via the V2 API.
func (a *APIv2) GetDownloadInfos(_ struct{}) {}

// GetDownloadInfos attempts to get the bundle corresponding to the charm url
//and origin.
func (a *API) GetDownloadInfos(args params.CharmURLAndOrigins) (params.DownloadInfoResults, error) {
	logger.Tracef("GetDownloadInfos %+v", args)

	results := params.DownloadInfoResults{
		Results: make([]params.DownloadInfoResult, len(args.Entities)),
	}
	for i, arg := range args.Entities {
		result, err := a.getDownloadInfo(arg)
		if err != nil {
			return params.DownloadInfoResults{}, errors.Trace(err)
		}
		results.Results[i] = result
	}
	return results, nil
}

func (a *API) getDownloadInfo(arg params.CharmURLAndOrigin) (params.DownloadInfoResult, error) {
	if err := a.checkCanRead(); err != nil {
		return params.DownloadInfoResult{}, apiservererrors.ServerError(err)
	}

	curl, err := charm.ParseURL(arg.CharmURL)
	if err != nil {
		return params.DownloadInfoResult{}, apiservererrors.ServerError(err)
	}

	defaultArch, err := a.getDefaultArch()
	if err != nil {
		return params.DownloadInfoResult{}, apiservererrors.ServerError(err)
	}

	charmOrigin, err := normalizeCharmOrigin(arg.Origin, defaultArch)
	if err != nil {
		return params.DownloadInfoResult{}, apiservererrors.ServerError(err)
	}

	repo, err := a.getCharmRepository(corecharm.Source(charmOrigin.Source))
	if err != nil {
		return params.DownloadInfoResult{}, apiservererrors.ServerError(err)
	}

	var macaroons macaroon.Slice
	if arg.Macaroon != nil {
		macaroons = append(macaroons, arg.Macaroon)
	}

	url, origin, err := repo.GetDownloadURL(curl, convertParamsOrigin(charmOrigin), macaroons)
	if err != nil {
		return params.DownloadInfoResult{}, apiservererrors.ServerError(err)
	}

	return params.DownloadInfoResult{
		URL:    url.String(),
		Origin: convertOrigin(origin),
	}, nil
}

func (a *API) getDefaultArch() (string, error) {
	cons, err := a.backendState.ModelConstraints()
	if err != nil {
		return "", errors.Trace(err)
	}
	return arch.ConstraintArch(cons, nil), nil
}

func normalizeCharmOrigin(origin params.CharmOrigin, fallbackArch string) (params.CharmOrigin, error) {
	// If the series is set to all, we need to ensure that we remove that, so
	// that we can attempt to derive it at a later stage. Juju itself doesn't
	// know nor understands what all means, so we need to ensure it doesn't leak
	// out.
	var os string
	var oSeries string
	if origin.Series == "all" {
		logger.Warningf("Release all detected, removing all from the origin. %s", origin.ID)
	} else if origin.Series != "" {
		// Always set the os from the series, so we know it's correctly
		// normalized for the rest of Juju.
		sys, err := series.GetOSFromSeries(origin.Series)
		if err != nil {
			return params.CharmOrigin{}, errors.Trace(err)
		}
		// Values passed to the api are case sensitive: ubuntu succeeds and
		// Ubuntu returns `"code": "revision-not-found"`
		os = strings.ToLower(sys.String())

		oSeries = origin.Series
	}

	arch := fallbackArch
	if origin.Architecture != "all" && origin.Architecture != "" {
		arch = origin.Architecture
	} else {
		logger.Warningf("Architecture not in expected state, found %q, using fallback architecture %q. %s", origin.Architecture, arch, origin.ID)
	}

	o := origin
	o.OS = os
	o.Series = oSeries
	o.Architecture = arch
	return o, nil
}

// AddCharm is not available via the V2 API.
func (a *APIv2) AddCharm(_ struct{}) {}

// AddCharm adds the given charm URL (which must include revision) to the
// environment, if it does not exist yet. Local charms are not supported,
// only charm store and charm hub URLs. See also AddLocalCharm().
func (a *API) AddCharm(args params.AddCharmWithOrigin) (params.CharmOriginResult, error) {
	logger.Tracef("AddCharm %+v", args)
	return a.addCharmWithAuthorization(params.AddCharmWithAuth{
		URL:                args.URL,
		Origin:             args.Origin,
		CharmStoreMacaroon: nil,
		Force:              args.Force,
		Series:             args.Series,
	})
}

// AddCharmWithAuthorization is not available via the V2 API.
func (a *APIv2) AddCharmWithAuthorization(_ struct{}) {}

// AddCharmWithAuthorization adds the given charm URL (which must include
// revision) to the environment, if it does not exist yet. Local charms are
// not supported, only charm store and charm hub URLs. See also AddLocalCharm().
//
// The authorization macaroon, args.CharmStoreMacaroon, may be
// omitted, in which case this call is equivalent to AddCharm.
func (a *API) AddCharmWithAuthorization(args params.AddCharmWithAuth) (params.CharmOriginResult, error) {
	logger.Tracef("AddCharmWithAuthorization %+v", args)
	return a.addCharmWithAuthorization(args)
}

func (a *API) addCharmWithAuthorization(args params.AddCharmWithAuth) (params.CharmOriginResult, error) {
	if args.Origin.Source != "charm-hub" && args.Origin.Source != "charm-store" {
		return params.CharmOriginResult{}, errors.Errorf("unknown schema for charm URL %q", args.URL)
	}

	if args.Origin.Source == "charm-hub" && args.Origin.Series == "" {
		return params.CharmOriginResult{}, errors.BadRequestf("series required for charm-hub charms")
	}

	if err := a.checkCanWrite(); err != nil {
		return params.CharmOriginResult{}, err
	}

	charmURL, err := charm.ParseURL(args.URL)
	if err != nil {
		return params.CharmOriginResult{}, err
	}

	ctrlCfg, err := a.backendState.ControllerConfig()
	if err != nil {
		return params.CharmOriginResult{}, err
	}

	// TODO(achilleasa): This escape hatch allows us to test the asynchronous
	// charm download code-path without breaking the existing deploy logic.
	//
	// It will be removed once the new universal deploy facade is into place.
	if ctrlCfg.Features().Contains(feature.AsynchronousCharmDownloads) {
		actualOrigin, err := a.queueAsyncCharmDownload(args)
		if err != nil {
			return params.CharmOriginResult{}, errors.Trace(err)
		}

		return params.CharmOriginResult{
			Origin: convertOrigin(actualOrigin),
		}, nil
	}

	downloader, err := a.newDownloader(services.CharmDownloaderConfig{
		Logger:         logger,
		Transport:      a.httpClient,
		StorageFactory: a.newStorage,
		StateBackend:   a.backendState,
		ModelBackend:   a.backendModel,
	})
	if err != nil {
		return params.CharmOriginResult{}, errors.Trace(err)
	}

	var macaroons macaroon.Slice
	if args.CharmStoreMacaroon != nil {
		macaroons = append(macaroons, args.CharmStoreMacaroon)
	}

	actualOrigin, err := downloader.DownloadAndStore(charmURL, convertParamsOrigin(args.Origin), macaroons, args.Force)
	if err != nil {
		return params.CharmOriginResult{}, errors.Trace(err)
	}

	return params.CharmOriginResult{
		Origin: convertOrigin(actualOrigin),
	}, nil
}

func (a *API) queueAsyncCharmDownload(args params.AddCharmWithAuth) (corecharm.Origin, error) {
	charmURL, err := charm.ParseURL(args.URL)
	if err != nil {
		return corecharm.Origin{}, err
	}

	// Fetch the charm metadata and add charm entry pending to be downloaded.
	requestedOrigin := convertParamsOrigin(args.Origin)
	repo, err := a.getCharmRepository(requestedOrigin.Source)
	if err != nil {
		return corecharm.Origin{}, errors.Trace(err)
	}

	var macaroons macaroon.Slice
	if args.CharmStoreMacaroon != nil {
		macaroons = append(macaroons, args.CharmStoreMacaroon)
	}

	// Check if a charm doc already exists for this charm URL. If so, the
	// charm has already been queued for download so this is a no-op. We
	// still need to resolve and return back a suitable origin as charmhub
	// may refer to the same blob using the same revision in different
	// channels.
	if _, err := a.backendState.Charm(charmURL); err == nil {
		_, resolvedOrigin, _, err := repo.ResolveWithPreferredChannel(charmURL, requestedOrigin, macaroons)
		return resolvedOrigin, errors.Trace(err)
	}

	// TODO(achilleasa):
	// At this stage we are only interested in the charm metadata and lxd
	// profile. However, the repo does not (yet) support metadata-only
	// lookups so we will use the download API and extract the metadata
	// we need.
	//
	// This will be rectified as part of the work for the new deploy facade.
	tmpFile, err := ioutil.TempFile("", "charm-")
	if err != nil {
		return corecharm.Origin{}, errors.Trace(err)
	}
	_ = tmpFile.Close()
	defer func() { _ = os.Remove(tmpFile.Name()) }()

	chArchive, resolvedOrigin, err := repo.DownloadCharm(charmURL, requestedOrigin, macaroons, tmpFile.Name())
	if err != nil {
		return corecharm.Origin{}, errors.Trace(err)
	}

	_, err = a.backendState.AddCharmMetadata(state.CharmInfo{
		Charm:    chArchive,
		ID:       charmURL,
		Macaroon: macaroons,
		Version:  chArchive.Version(),
	})
	if err != nil {
		return corecharm.Origin{}, errors.Trace(err)
	}

	return resolvedOrigin, nil
}

// ResolveCharms is not available via the V2 API.
func (a *APIv2) ResolveCharms(_ struct{}) {}

// ResolveCharms resolves the given charm URLs with an optionally specified
// preferred channel.  Channel provided via CharmOrigin.
func (a *API) ResolveCharms(args params.ResolveCharmsWithChannel) (params.ResolveCharmWithChannelResults, error) {
	logger.Tracef("ResolveCharms %+v", args)
	if err := a.checkCanRead(); err != nil {
		return params.ResolveCharmWithChannelResults{}, errors.Trace(err)
	}
	result := params.ResolveCharmWithChannelResults{
		Results: make([]params.ResolveCharmWithChannelResult, len(args.Resolve)),
	}
	for i, arg := range args.Resolve {
		result.Results[i] = a.resolveOneCharm(arg, args.Macaroon)
	}

	return result, nil
}

func (a *API) resolveOneCharm(arg params.ResolveCharmWithChannel, mac *macaroon.Macaroon) params.ResolveCharmWithChannelResult {
	result := params.ResolveCharmWithChannelResult{}
	curl, err := charm.ParseURL(arg.Reference)
	if err != nil {
		result.Error = apiservererrors.ServerError(err)
		return result
	}
	if !charm.CharmHub.Matches(curl.Schema) && !charm.CharmStore.Matches(curl.Schema) {
		result.Error = apiservererrors.ServerError(errors.Errorf("unknown schema for charm URL %q", curl.String()))
		return result
	}

	// Validate the origin passed in.
	if err := validateOrigin(arg.Origin, curl.Schema, arg.SwitchCharm); err != nil {
		result.Error = apiservererrors.ServerError(err)
		return result
	}

	repo, err := a.getCharmRepository(corecharm.Source(arg.Origin.Source))
	if err != nil {
		result.Error = apiservererrors.ServerError(err)
		return result
	}

	var macaroons macaroon.Slice
	if mac != nil {
		macaroons = append(macaroons, mac)
	}

	resultURL, origin, supportedSeries, err := repo.ResolveWithPreferredChannel(curl, convertParamsOrigin(arg.Origin), macaroons)
	if err != nil {
		result.Error = apiservererrors.ServerError(err)
		return result
	}
	result.URL = resultURL.String()

	apiOrigin := convertOrigin(origin)

	// The charmhub API can return "all" for architecture as it's not a real
	// arch we don't know how to correctly model it. "all " doesn't mean use the
	// default arch, it means use any arch which isn't quite the same. So if we
	// do get "all" we should see if there is a clean way to resolve it.
	archOrigin := apiOrigin
	if apiOrigin.Architecture == "all" {
		cons, err := a.backendState.ModelConstraints()
		if err != nil {
			result.Error = apiservererrors.ServerError(err)
			return result
		}
		archOrigin.Architecture = arch.ConstraintArch(cons, nil)
	}

	result.Origin = archOrigin

	switch {
	case resultURL.Series != "" && len(supportedSeries) == 0:
		result.SupportedSeries = []string{resultURL.Series}
	default:
		result.SupportedSeries = supportedSeries
	}

	return result
}

func validateOrigin(origin params.CharmOrigin, schema string, switchCharm bool) error {
	// If we are switching to a different charm we can skip the following
	// origin check; doing so allows us to switch from a charmstore charm
	// to the equivalent charmhub charm.
	if !switchCharm {
		if (corecharm.Local.Matches(origin.Source) && !charm.Local.Matches(schema)) ||
			(corecharm.CharmStore.Matches(origin.Source) && !charm.CharmStore.Matches(schema)) ||
			(corecharm.CharmHub.Matches(origin.Source) && !charm.CharmHub.Matches(schema)) {
			return errors.NotValidf("origin source %q with schema", origin.Source)
		}
	}

	if corecharm.CharmHub.Matches(origin.Source) && origin.Architecture == "" {
		return errors.NotValidf("empty architecture")
	}
	return nil
}

func (a *API) getCharmRepository(src corecharm.Source) (corecharm.Repository, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.repoFactory == nil {
		a.repoFactory = a.newRepoFactory(services.CharmRepoFactoryConfig{
			Logger:       logger,
			Transport:    a.httpClient,
			StateBackend: a.backendState,
			ModelBackend: a.backendModel,
		})
	}

	return a.repoFactory.GetCharmRepository(src)
}

// IsMetered returns whether or not the charm is metered.
func (a *API) IsMetered(args params.CharmURL) (params.IsMeteredResult, error) {
	if err := a.checkCanRead(); err != nil {
		return params.IsMeteredResult{}, errors.Trace(err)
	}

	curl, err := charm.ParseURL(args.URL)
	if err != nil {
		return params.IsMeteredResult{Metered: false}, errors.Trace(err)
	}
	aCharm, err := a.backendState.Charm(curl)
	if err != nil {
		return params.IsMeteredResult{Metered: false}, errors.Trace(err)
	}
	if aCharm.Metrics() != nil && len(aCharm.Metrics().Metrics) > 0 {
		return params.IsMeteredResult{Metered: true}, nil
	}
	return params.IsMeteredResult{Metered: false}, nil
}

// CheckCharmPlacement isn't on the v13 API.
func (a *APIv3) CheckCharmPlacement(_, _ struct{}) {}

// CheckCharmPlacement checks if a charm is allowed to be placed with in a
// given application.
func (a *API) CheckCharmPlacement(args params.ApplicationCharmPlacements) (params.ErrorResults, error) {
	if err := a.checkCanRead(); err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}

	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Placements)),
	}
	for i, placement := range args.Placements {
		result, err := a.checkCharmPlacement(placement)
		if err != nil {
			return params.ErrorResults{}, errors.Trace(err)
		}
		results.Results[i] = result
	}

	return results, nil
}

func (a *API) checkCharmPlacement(arg params.ApplicationCharmPlacement) (params.ErrorResult, error) {
	curl, err := charm.ParseURL(arg.CharmURL)
	if err != nil {
		return params.ErrorResult{
			Error: apiservererrors.ServerError(err),
		}, nil
	}

	// The placement logic below only cares about charmhub charms. Once we have
	// multiple architecture support for charmhub, we can remove the placement
	// check.
	if !charm.CharmHub.Matches(curl.Schema) {
		return params.ErrorResult{}, nil
	}

	// Get the application. If it's not found, just return without an error as
	// the charm can be placed in the application once it's created.
	app, err := a.backendState.Application(arg.Application)
	if errors.IsNotFound(err) {
		return params.ErrorResult{}, nil
	} else if err != nil {
		return params.ErrorResult{
			Error: apiservererrors.ServerError(err),
		}, nil
	}

	// We don't care for subordinates here.
	if !app.IsPrincipal() {
		return params.ErrorResult{}, nil
	}

	constraints, err := app.Constraints()
	if err != nil && !errors.IsNotFound(err) {
		return params.ErrorResult{
			Error: apiservererrors.ServerError(err),
		}, nil
	}

	// If the application has an existing architecture constraint then we're
	// happy that the constraint logic will prevent heterogenous application
	// units.
	if constraints.HasArch() {
		return params.ErrorResult{}, nil
	}

	// Unfortunately we now have to check instance data for all units to
	// validate that we have a homogeneous setup.
	units, err := app.AllUnits()
	if err != nil {
		return params.ErrorResult{
			Error: apiservererrors.ServerError(err),
		}, nil
	}

	arches := set.NewStrings()
	for _, unit := range units {
		machineID, err := unit.AssignedMachineId()
		if errors.IsNotAssigned(err) {
			continue
		} else if err != nil {
			return params.ErrorResult{
				Error: apiservererrors.ServerError(err),
			}, nil
		}

		machine, err := a.backendState.Machine(machineID)
		if errors.IsNotFound(err) {
			continue
		} else if err != nil {
			return params.ErrorResult{
				Error: apiservererrors.ServerError(err),
			}, nil
		}

		machineArch, err := a.getMachineArch(machine)
		if err != nil {
			return params.ErrorResult{
				Error: apiservererrors.ServerError(err),
			}, nil
		}

		if machineArch == "" {
			arches.Add(arch.DefaultArchitecture)
		} else {
			arches.Add(machineArch)
		}
	}

	if arches.Size() > 1 {
		// It is expected that charmhub charms form a homogeneous workload,
		// so that each unit is the same architecture.
		err := errors.Errorf("charm can not be placed in a heterogeneous environment")
		return params.ErrorResult{
			Error: apiservererrors.ServerError(err),
		}, nil
	}

	return params.ErrorResult{}, nil
}

func (a *API) getMachineArch(machine charmsinterfaces.Machine) (arch.Arch, error) {
	cons, err := machine.Constraints()
	if err == nil && cons.HasArch() {
		return *cons.Arch, nil
	}

	hardware, err := machine.HardwareCharacteristics()
	if errors.IsNotFound(err) {
		return "", nil
	} else if err != nil {
		return "", errors.Trace(err)
	}

	if hardware.Arch != nil {
		return *hardware.Arch, nil
	}

	return "", nil
}

// ListCharmResources is not available via the V2 API.
func (a *APIv2) ListCharmResources(_ struct{}) {}

// ListCharmResources returns a series of resources for a given charm.
func (a *API) ListCharmResources(args params.CharmURLAndOrigins) (params.CharmResourcesResults, error) {
	if err := a.checkCanRead(); err != nil {
		return params.CharmResourcesResults{}, errors.Trace(err)
	}
	results := params.CharmResourcesResults{
		Results: make([][]params.CharmResourceResult, len(args.Entities)),
	}
	for i, arg := range args.Entities {
		result, err := a.listOneCharmResources(arg)
		if err != nil {
			return params.CharmResourcesResults{}, errors.Trace(err)
		}
		results.Results[i] = result
	}
	return results, nil
}

func (a *API) listOneCharmResources(arg params.CharmURLAndOrigin) ([]params.CharmResourceResult, error) {
	// TODO (stickupkid) - remove api packages from apiserver packages.
	curl, err := charm.ParseURL(arg.CharmURL)
	if err != nil {
		return nil, apiservererrors.ServerError(err)
	}
	if !charm.CharmHub.Matches(curl.Schema) {
		return nil, apiservererrors.ServerError(errors.NotValidf("charm %q", curl.Name))
	}

	defaultArch, err := a.getDefaultArch()
	if err != nil {
		return nil, apiservererrors.ServerError(err)
	}

	charmOrigin, err := normalizeCharmOrigin(arg.Origin, defaultArch)
	if err != nil {
		return nil, apiservererrors.ServerError(err)
	}

	repo, err := a.getCharmRepository(corecharm.Source(curl.Schema))
	if err != nil {
		return nil, apiservererrors.ServerError(err)
	}

	var macaroons macaroon.Slice
	if arg.Macaroon != nil {
		macaroons = append(macaroons, arg.Macaroon)
	}

	resources, err := repo.ListResources(curl, convertParamsOrigin(charmOrigin), macaroons)
	if err != nil {
		return nil, apiservererrors.ServerError(err)
	}

	results := make([]params.CharmResourceResult, len(resources))
	for i, resource := range resources {
		results[i].CharmResource = apiresources.CharmResource2API(resource)
	}

	return results, nil
}
