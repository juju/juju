// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms

import (
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/juju/charm/v10"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"

	apiresources "github.com/juju/juju/api/client/resources"
	commoncharm "github.com/juju/juju/api/common/charm"
	charmscommon "github.com/juju/juju/apiserver/common/charms"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	charmsinterfaces "github.com/juju/juju/apiserver/facades/client/charms/interfaces"
	"github.com/juju/juju/apiserver/facades/client/charms/services"
	"github.com/juju/juju/core/arch"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

var logger = loggo.GetLogger("juju.apiserver.charms")

// APIv6 provides the Charms API facade for version 6.
// It removes the AddCharmWithAuthorization function, as
// we no longer support macaroons.
type APIv6 struct {
	*API
}

// APIv5 provides the Charms API facade for version 5.
type APIv5 struct {
	*APIv6
}

// API implements the charms interface and is the concrete
// implementation of the API end point.
type API struct {
	charmInfoAPI       *charmscommon.CharmInfoAPI
	authorizer         facade.Authorizer
	backendState       charmsinterfaces.BackendState
	backendModel       charmsinterfaces.BackendModel
	charmhubHTTPClient facade.HTTPClient

	tag             names.ModelTag
	requestRecorder facade.RequestRecorder

	newStorage     func(modelUUID string) services.Storage
	newDownloader  func(services.CharmDownloaderConfig) (charmsinterfaces.Downloader, error)
	newRepoFactory func(services.CharmRepoFactoryConfig) corecharm.RepositoryFactory

	mu          sync.Mutex
	repoFactory corecharm.RepositoryFactory
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
		authorizer:      authorizer,
		backendState:    st,
		backendModel:    m,
		newStorage:      newStorage,
		newDownloader:   newDownloader,
		tag:             m.ModelTag(),
		requestRecorder: noopRequestRecorder{},
		repoFactory:     repoFactory,
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

// GetDownloadInfos attempts to get the bundle corresponding to the charm url
// and origin.
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

	requestedOrigin, err := ConvertParamsOrigin(charmOrigin)
	if err != nil {
		return params.DownloadInfoResult{}, apiservererrors.ServerError(err)
	}
	url, origin, err := repo.GetDownloadURL(curl, requestedOrigin)
	if err != nil {
		return params.DownloadInfoResult{}, apiservererrors.ServerError(err)
	}

	dlorigin, err := convertOrigin(origin)
	if err != nil {
		return params.DownloadInfoResult{}, errors.Trace(err)
	}
	return params.DownloadInfoResult{
		URL:    url.String(),
		Origin: dlorigin,
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
	// know nor understand what "all" means, so we need to ensure it doesn't leak
	// out.
	o := origin
	if origin.Base.Name == "all" || origin.Base.Channel == "all" {
		logger.Warningf("Release all detected, removing all from the origin. %s", origin.ID)
		o.Base = params.Base{}
	}

	if origin.Architecture == "all" || origin.Architecture == "" {
		logger.Warningf("Architecture not in expected state, found %q, using fallback architecture %q. %s", origin.Architecture, fallbackArch, origin.ID)
		o.Architecture = fallbackArch
	}

	return o, nil
}

// AddCharm adds the given charm URL (which must include revision) to the
// environment, if it does not exist yet. Local charms are not supported,
// only charm store and charm hub URLs. See also AddLocalCharm().
func (a *API) AddCharm(args params.AddCharmWithOrigin) (params.CharmOriginResult, error) {
	logger.Tracef("AddCharm %+v", args)
	return a.addCharmWithAuthorization(params.AddCharmWithAuth{
		URL:    args.URL,
		Origin: args.Origin,
		Force:  args.Force,
	})
}

// AddCharmWithAuthorization adds the given charm URL (which must include
// revision) to the environment, if it does not exist yet. Local charms are
// not supported, only charm hub URLs. See also AddLocalCharm().
//
// Since the charm macaroons are no longer supported, this is the same as
// AddCharm. We keep it for backwards compatibility in APIv5.
func (a *APIv5) AddCharmWithAuthorization(args params.AddCharmWithAuth) (params.CharmOriginResult, error) {
	logger.Tracef("AddCharmWithAuthorization %+v", args)
	return a.addCharmWithAuthorization(args)
}

func (a *API) addCharmWithAuthorization(args params.AddCharmWithAuth) (params.CharmOriginResult, error) {
	if commoncharm.OriginSource(args.Origin.Source) != commoncharm.OriginCharmHub {
		return params.CharmOriginResult{}, errors.Errorf("unknown schema for charm URL %q", args.URL)
	}

	if args.Origin.Base.Name == "" || args.Origin.Base.Channel == "" {
		return params.CharmOriginResult{}, errors.BadRequestf("base required for Charmhub charms")
	}

	if err := a.checkCanWrite(); err != nil {
		return params.CharmOriginResult{}, err
	}

	actualOrigin, err := a.queueAsyncCharmDownload(args)
	if err != nil {
		return params.CharmOriginResult{}, errors.Trace(err)
	}

	origin, err := convertOrigin(actualOrigin)
	if err != nil {
		return params.CharmOriginResult{}, errors.Trace(err)
	}
	return params.CharmOriginResult{
		Origin: origin,
	}, nil
}

func (a *API) queueAsyncCharmDownload(args params.AddCharmWithAuth) (corecharm.Origin, error) {
	charmURL, err := charm.ParseURL(args.URL)
	if err != nil {
		return corecharm.Origin{}, err
	}

	requestedOrigin, err := ConvertParamsOrigin(args.Origin)
	if err != nil {
		return corecharm.Origin{}, errors.Trace(err)
	}
	repo, err := a.getCharmRepository(requestedOrigin.Source)
	if err != nil {
		return corecharm.Origin{}, errors.Trace(err)
	}

	// Check if a charm doc already exists for this charm URL. If so, the
	// charm has already been queued for download so this is a no-op. We
	// still need to resolve and return back a suitable origin as charmhub
	// may refer to the same blob using the same revision in different
	// channels.
	//
	// We need to use GetDownloadURL instead of ResolveWithPreferredChannel
	// to ensure that the resolved origin has the ID/Hash fields correctly
	// populated.
	if _, err := a.backendState.Charm(charmURL); err == nil {
		_, resolvedOrigin, err := repo.GetDownloadURL(charmURL, requestedOrigin)
		return resolvedOrigin, errors.Trace(err)
	}

	// Fetch the essential metadata that we require to deploy the charm
	// without downloading the full archive. The remaining metadata will
	// be populated once the charm gets downloaded.
	essentialMeta, err := repo.GetEssentialMetadata(corecharm.MetadataRequest{
		CharmURL: charmURL,
		Origin:   requestedOrigin,
	})
	if err != nil {
		return corecharm.Origin{}, errors.Annotatef(err, "retrieving essential metadata for charm %q", charmURL)
	}
	metaRes := essentialMeta[0]

	_, err = a.backendState.AddCharmMetadata(state.CharmInfo{
		Charm: corecharm.NewCharmInfoAdapter(metaRes),
		ID:    charmURL,
	})
	if err != nil {
		return corecharm.Origin{}, errors.Trace(err)
	}

	return metaRes.ResolvedOrigin, nil
}

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
		result.Results[i] = a.resolveOneCharm(arg)
	}

	return result, nil
}

func (a *API) resolveOneCharm(arg params.ResolveCharmWithChannel) params.ResolveCharmWithChannelResult {
	result := params.ResolveCharmWithChannelResult{}
	curl, err := charm.ParseURL(arg.Reference)
	if err != nil {
		result.Error = apiservererrors.ServerError(err)
		return result
	}
	if !charm.CharmHub.Matches(curl.Schema) {
		result.Error = apiservererrors.ServerError(errors.Errorf("unknown schema for charm URL %q", curl.String()))
		return result
	}

	requestedOrigin, err := ConvertParamsOrigin(arg.Origin)
	if err != nil {
		result.Error = apiservererrors.ServerError(err)
		return result
	}

	// Validate the origin passed in.
	if err := validateOrigin(requestedOrigin, curl, arg.SwitchCharm); err != nil {
		result.Error = apiservererrors.ServerError(err)
		return result
	}

	repo, err := a.getCharmRepository(corecharm.Source(arg.Origin.Source))
	if err != nil {
		result.Error = apiservererrors.ServerError(err)
		return result
	}

	resultURL, origin, supportedSeries, err := repo.ResolveWithPreferredChannel(curl, requestedOrigin)
	if err != nil {
		result.Error = apiservererrors.ServerError(err)
		return result
	}
	result.URL = resultURL.String()

	apiOrigin, err := convertOrigin(origin)
	if err != nil {
		result.Error = apiservererrors.ServerError(err)
		return result
	}

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

func validateOrigin(origin corecharm.Origin, curl *charm.URL, switchCharm bool) error {
	if !charm.CharmHub.Matches(curl.Schema) {
		return errors.Errorf("unknown schema for charm URL %q", curl.String())
	}
	// If we are switching to a different charm we can skip the following
	// origin check; doing so allows us to switch from a charmstore charm
	// to the equivalent charmhub charm.
	if !switchCharm {
		schema := curl.Schema
		if (corecharm.Local.Matches(origin.Source.String()) && !charm.Local.Matches(schema)) ||
			(corecharm.CharmHub.Matches(origin.Source.String()) && !charm.CharmHub.Matches(schema)) {
			return errors.NotValidf("origin source %q with schema", origin.Source)
		}
	}

	if corecharm.CharmHub.Matches(origin.Source.String()) && origin.Platform.Architecture == "" {
		return errors.NotValidf("empty architecture")
	}
	return nil
}

func (a *API) getCharmRepository(src corecharm.Source) (corecharm.Repository, error) {
	// The following is only required for testing, as we generate a new http
	// client here for production.
	a.mu.Lock()
	if a.repoFactory != nil {
		defer a.mu.Unlock()
		return a.repoFactory.GetCharmRepository(src)
	}
	a.mu.Unlock()

	repoFactory := a.newRepoFactory(services.CharmRepoFactoryConfig{
		Logger:             logger,
		CharmhubHTTPClient: a.charmhubHTTPClient,
		StateBackend:       a.backendState,
		ModelBackend:       a.backendModel,
	})

	return repoFactory.GetCharmRepository(src)
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
	repo, err := a.getCharmRepository(corecharm.Source(charmOrigin.Source))
	if err != nil {
		return nil, apiservererrors.ServerError(err)
	}

	requestedOrigin, err := ConvertParamsOrigin(charmOrigin)
	if err != nil {
		return nil, apiservererrors.ServerError(err)
	}
	resources, err := repo.ListResources(curl, requestedOrigin)
	if err != nil {
		return nil, apiservererrors.ServerError(err)
	}

	results := make([]params.CharmResourceResult, len(resources))
	for i, resource := range resources {
		results[i].CharmResource = apiresources.CharmResource2API(resource)
	}

	return results, nil
}

type noopRequestRecorder struct{}

// Record an outgoing request which produced an http.Response.
func (noopRequestRecorder) Record(method string, url *url.URL, res *http.Response, rtt time.Duration) {
}

// Record an outgoing request which returned back an error.
func (noopRequestRecorder) RecordError(method string, url *url.URL, err error) {}
