// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms

import (
	"context"
	"net/http"
	"net/url"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/collections/transform"
	"github.com/juju/errors"
	"github.com/juju/names/v6"

	apiresources "github.com/juju/juju/api/client/resources"
	commoncharm "github.com/juju/juju/api/common/charm"
	"github.com/juju/juju/apiserver/authentication"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	charmsinterfaces "github.com/juju/juju/apiserver/facades/client/charms/interfaces"
	charmscommon "github.com/juju/juju/apiserver/internal/charms"
	"github.com/juju/juju/core/arch"
	corecharm "github.com/juju/juju/core/charm"
	corelogger "github.com/juju/juju/core/logger"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/architecture"
	applicationcharm "github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/charm/repository"
	"github.com/juju/juju/rpc/params"
)

// APIv7 provides the Charms API facade for version 7.
type APIv7 struct {
	*API
}

// API implements the charms interface and is the concrete
// implementation of the API end point.
type API struct {
	charmInfoAPI       *charmscommon.CharmInfoAPI
	authorizer         facade.Authorizer
	backendState       charmsinterfaces.BackendState
	charmhubHTTPClient facade.HTTPClient

	modelTag        names.ModelTag
	controllerTag   names.ControllerTag
	requestRecorder facade.RequestRecorder

	newCharmHubRepository func(repository.CharmHubRepositoryConfig) (corecharm.Repository, error)

	logger corelogger.Logger

	modelConfigService ModelConfigService
	applicationService ApplicationService
	machineService     MachineService
}

// CharmInfo returns information about the requested charm.
func (a *API) CharmInfo(ctx context.Context, args params.CharmURL) (params.Charm, error) {
	return a.charmInfoAPI.CharmInfo(ctx, args)
}

func (a *API) checkCanRead(ctx context.Context) error {
	err := a.authorizer.HasPermission(ctx, permission.ReadAccess, a.modelTag)
	return err
}

func (a *API) checkCanWrite(ctx context.Context) error {
	err := a.authorizer.HasPermission(ctx, permission.SuperuserAccess, a.controllerTag)
	if err != nil && !errors.Is(err, authentication.ErrorEntityMissingPermission) {
		return errors.Trace(err)
	}

	if err == nil {
		return nil
	}

	return a.authorizer.HasPermission(ctx, permission.WriteAccess, a.modelTag)
}

// List returns a list of charm URLs currently in the state. If supplied
// parameter contains any names, the result will be filtered to return only the
// charms with supplied names. The order of the charms is not guaranteed to be
// the same as the order of the names passed in.
func (a *API) List(ctx context.Context, args params.CharmsList) (params.CharmsListResult, error) {
	a.logger.Tracef(context.TODO(), "List %+v", args)
	if err := a.checkCanRead(ctx); err != nil {
		return params.CharmsListResult{}, errors.Trace(err)
	}

	// Select all the charms from state. If no names are passed, all the charms
	// will be returned.
	names := set.NewStrings(args.Names...).SortedValues()
	charms, err := a.applicationService.ListCharmLocators(ctx, names...)
	if err != nil {
		return params.CharmsListResult{}, errors.Annotatef(err, "listing charms")
	}

	var charmURLs []string
	for _, aCharm := range charms {
		curl, err := charmURLFromLocator(aCharm.Name, aCharm)
		if err != nil {
			return params.CharmsListResult{}, errors.Trace(err)
		}

		charmURLs = append(charmURLs, curl)
	}
	return params.CharmsListResult{CharmURLs: charmURLs}, nil
}

// GetDownloadInfos attempts to get the bundle corresponding to the charm url
// and origin.
func (a *API) GetDownloadInfos(ctx context.Context, args params.CharmURLAndOrigins) (params.DownloadInfoResults, error) {
	a.logger.Tracef(context.TODO(), "GetDownloadInfos %+v", args)

	results := params.DownloadInfoResults{
		Results: make([]params.DownloadInfoResult, len(args.Entities)),
	}
	for i, arg := range args.Entities {
		result, err := a.getDownloadInfo(ctx, arg)
		if err != nil {
			return params.DownloadInfoResults{}, errors.Trace(err)
		}
		results.Results[i] = result
	}
	return results, nil
}

func (a *API) getDownloadInfo(ctx context.Context, arg params.CharmURLAndOrigin) (params.DownloadInfoResult, error) {
	if err := a.checkCanRead(ctx); err != nil {
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

	charmOrigin, err := normalizeCharmOrigin(arg.Origin, defaultArch, a.logger)
	if err != nil {
		return params.DownloadInfoResult{}, apiservererrors.ServerError(err)
	}

	repo, err := a.getCharmRepository(ctx)
	if err != nil {
		return params.DownloadInfoResult{}, apiservererrors.ServerError(err)
	}

	requestedOrigin, err := ConvertParamsOrigin(charmOrigin)
	if err != nil {
		return params.DownloadInfoResult{}, apiservererrors.ServerError(err)
	}
	url, origin, err := repo.GetDownloadURL(ctx, curl.Name, requestedOrigin)
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
	// TODO(CodingCookieRookie): Retrieve the model constraint architecture from dqlite and use it
	// as the first arg in constraints.ArchOrDefault instead of DefaultArchitecture
	return arch.DefaultArchitecture, nil
}

// AddCharm adds the given charm URL (which must include revision) to the
// environment, if it does not exist yet. Local charms are not supported,
// only charm store and charm hub URLs. See also AddLocalCharm().
func (a *API) AddCharm(ctx context.Context, args params.AddCharmWithOrigin) (params.CharmOriginResult, error) {
	if err := a.checkCanWrite(ctx); err != nil {
		return params.CharmOriginResult{}, err
	}

	a.logger.Debugf(context.TODO(), "AddCharm request: %+v", args)
	if commoncharm.OriginSource(args.Origin.Source) != commoncharm.OriginCharmHub {
		return params.CharmOriginResult{}, errors.Errorf("unknown schema for charm URL %q", args.URL)
	}

	if args.Origin.Base.Name == "" || args.Origin.Base.Channel == "" {
		return params.CharmOriginResult{}, errors.BadRequestf("base required for Charmhub charms")
	}

	actualOrigin, err := a.addCharm(ctx, args)
	if err != nil {
		return params.CharmOriginResult{}, errors.Trace(err)
	}

	origin, err := convertOrigin(actualOrigin)
	if err != nil {
		return params.CharmOriginResult{}, errors.Trace(err)
	}

	a.logger.Debugf(context.TODO(), "AddCharm result: %+v", origin)

	return params.CharmOriginResult{
		Origin: origin,
	}, nil
}

func (a *API) addCharm(ctx context.Context, args params.AddCharmWithOrigin) (corecharm.Origin, error) {
	charmURL, err := charm.ParseURL(args.URL)
	if err != nil {
		return corecharm.Origin{}, err
	}

	requestedOrigin, err := ConvertParamsOrigin(args.Origin)
	if err != nil {
		return corecharm.Origin{}, errors.Trace(err)
	}
	repo, err := a.getCharmRepository(ctx)
	if err != nil {
		return corecharm.Origin{}, errors.Trace(err)
	}

	// Fetch the essential metadata that we require to deploy the charm
	// without downloading the full archive. The remaining metadata will
	// be populated once the charm gets downloaded.
	resolved, err := repo.ResolveForDeploy(ctx, corecharm.CharmID{
		URL:    charmURL,
		Origin: requestedOrigin,
	})
	if err != nil {
		return corecharm.Origin{}, errors.Annotatef(err, "retrieving essential metadata for charm %q", charmURL)
	}

	essentialMetadata := resolved.EssentialMetadata

	revision, err := makeCharmRevision(essentialMetadata.ResolvedOrigin, args.URL)
	if err != nil {
		return corecharm.Origin{}, errors.Annotatef(err, "making revision for charm %q", args.URL)
	}

	if _, warnings, err := a.applicationService.SetCharm(ctx, applicationcharm.SetCharmArgs{
		Charm:         corecharm.NewCharmInfoAdaptor(essentialMetadata),
		Source:        requestedOrigin.Source,
		ReferenceName: charmURL.Name,
		Revision:      revision,
		Hash:          essentialMetadata.ResolvedOrigin.Hash,
		Architecture:  essentialMetadata.ResolvedOrigin.Platform.Architecture,
		DownloadInfo: &applicationcharm.DownloadInfo{
			Provenance:         applicationcharm.ProvenanceDownload,
			CharmhubIdentifier: essentialMetadata.DownloadInfo.CharmhubIdentifier,
			DownloadURL:        essentialMetadata.DownloadInfo.DownloadURL,
			DownloadSize:       essentialMetadata.DownloadInfo.DownloadSize,
		},
	}); err != nil && !errors.Is(err, applicationerrors.CharmAlreadyExists) {
		return corecharm.Origin{}, errors.Annotatef(err, "setting charm %q", args.URL)
	} else if len(warnings) > 0 {
		a.logger.Infof(context.TODO(), "setting charm %q: %v", args.URL, warnings)
	}

	return essentialMetadata.ResolvedOrigin, nil
}

func makeCharmRevision(origin corecharm.Origin, url string) (int, error) {
	if origin.Revision != nil && *origin.Revision >= 0 {
		return *origin.Revision, nil
	}

	curl, err := charm.ParseURL(url)
	if err != nil {
		return -1, errors.Annotatef(err, "parsing charm URL %q", url)
	}
	return curl.Revision, nil
}

// ResolveCharms resolves the given charm URLs with an optionally specified
// preferred channel.  Channel provided via CharmOrigin.
func (a *API) ResolveCharms(ctx context.Context, args params.ResolveCharmsWithChannel) (params.ResolveCharmWithChannelResults, error) {
	a.logger.Tracef(context.TODO(), "ResolveCharms %+v", args)
	if err := a.checkCanRead(ctx); err != nil {
		return params.ResolveCharmWithChannelResults{}, errors.Trace(err)
	}
	result := params.ResolveCharmWithChannelResults{
		Results: make([]params.ResolveCharmWithChannelResult, len(args.Resolve)),
	}
	for i, arg := range args.Resolve {
		result.Results[i] = a.resolveOneCharm(ctx, arg)
	}

	return result, nil
}

func (a *API) resolveOneCharm(ctx context.Context, arg params.ResolveCharmWithChannel) params.ResolveCharmWithChannelResult {
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

	repo, err := a.getCharmRepository(ctx)
	if err != nil {
		result.Error = apiservererrors.ServerError(err)
		return result
	}

	resolved, err := repo.ResolveWithPreferredChannel(ctx, curl.Name, requestedOrigin)
	if err != nil {
		result.Error = apiservererrors.ServerError(err)
		return result
	}

	resultURL, origin, resolvedBases := resolved.URL, resolved.Origin, resolved.Platform

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
		// TODO(CodingCookieRookie): Retrieve the model constraint architecture from dqlite and use it
		// as the first arg in constraints.ArchOrDefault instead of DefaultArchitecture
		archOrigin.Architecture = arch.DefaultArchitecture
	}

	result.Origin = archOrigin
	result.SupportedBases = transform.Slice(resolvedBases, convertCharmBase)

	return result
}

func convertCharmBase(in corecharm.Platform) params.Base {
	return params.Base{
		Name:    in.OS,
		Channel: in.Channel,
	}
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
	return origin.Validate()
}

func (a *API) getCharmRepository(ctx context.Context) (corecharm.Repository, error) {
	modelCfg, err := a.modelConfigService.ModelConfig(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	charmhubURL, _ := modelCfg.CharmHubURL()

	return a.newCharmHubRepository(repository.CharmHubRepositoryConfig{
		Logger:             a.logger,
		CharmhubHTTPClient: a.charmhubHTTPClient,
		CharmhubURL:        charmhubURL,
	})
}

// CheckCharmPlacement checks if a charm is allowed to be placed with in a
// given application.
func (a *API) CheckCharmPlacement(ctx context.Context, args params.ApplicationCharmPlacements) (params.ErrorResults, error) {
	if err := a.checkCanRead(ctx); err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}

	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Placements)),
	}
	for i, placement := range args.Placements {
		result, err := a.checkCharmPlacement(ctx, placement)
		if err != nil {
			return params.ErrorResults{}, errors.Trace(err)
		}
		results.Results[i] = result
	}

	return results, nil
}

func (a *API) checkCharmPlacement(ctx context.Context, arg params.ApplicationCharmPlacement) (params.ErrorResult, error) {
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
	if errors.Is(err, errors.NotFound) {
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

	appID, err := a.applicationService.GetApplicationIDByName(ctx, arg.Application)
	if errors.Is(err, applicationerrors.ApplicationNotFound) {
		return params.ErrorResult{}, nil
	} else if err != nil {
		return params.ErrorResult{
			Error: apiservererrors.ServerError(err),
		}, nil
	}
	cons, err := a.applicationService.GetApplicationConstraints(ctx, appID)
	if errors.Is(err, applicationerrors.ApplicationNotFound) {
		return params.ErrorResult{}, nil
	} else if err != nil {
		return params.ErrorResult{
			Error: apiservererrors.ServerError(err),
		}, nil
	}

	// If the application has an existing architecture constraint then we're
	// happy that the constraint logic will prevent heterogenous application
	// units.
	if cons.HasArch() {
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
		if errors.Is(err, errors.NotAssigned) {
			continue
		} else if err != nil {
			return params.ErrorResult{
				Error: apiservererrors.ServerError(err),
			}, nil
		}

		machine, err := a.backendState.Machine(machineID)
		if errors.Is(err, errors.NotFound) {
			continue
		} else if err != nil {
			return params.ErrorResult{
				Error: apiservererrors.ServerError(err),
			}, nil
		}

		machineArch, err := a.getMachineArch(ctx, machineID, machine)
		if errors.Is(err, machineerrors.NotProvisioned) {
			return params.ErrorResult{
				Error: apiservererrors.ServerError(errors.NotProvisioned),
			}, nil
		}
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

func (a *API) getMachineArch(ctx context.Context, machineID string, machine charmsinterfaces.Machine) (arch.Arch, error) {
	cons, err := machine.Constraints()
	if err == nil && cons.HasArch() {
		return *cons.Arch, nil
	}

	machineUUID, err := a.machineService.GetMachineUUID(ctx, coremachine.Name(machineID))
	if errors.Is(err, errors.NotFound) {
		return "", nil
	}
	if err != nil {
		return "", errors.Trace(err)
	}
	hardware, err := a.machineService.HardwareCharacteristics(ctx, machineUUID)
	if err != nil {
		return "", errors.Trace(err)
	}

	if hardware.Arch != nil {
		return *hardware.Arch, nil
	}

	return "", nil
}

// ListCharmResources returns a series of resources for a given charm.
func (a *API) ListCharmResources(ctx context.Context, args params.CharmURLAndOrigins) (params.CharmResourcesResults, error) {
	if err := a.checkCanRead(ctx); err != nil {
		return params.CharmResourcesResults{}, errors.Trace(err)
	}
	results := params.CharmResourcesResults{
		Results: make([][]params.CharmResourceResult, len(args.Entities)),
	}
	for i, arg := range args.Entities {
		result, err := a.listOneCharmResources(ctx, arg)
		if err != nil {
			return params.CharmResourcesResults{}, errors.Trace(err)
		}
		results.Results[i] = result
	}
	return results, nil
}

func (a *API) listOneCharmResources(ctx context.Context, arg params.CharmURLAndOrigin) ([]params.CharmResourceResult, error) {
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

	charmOrigin, err := normalizeCharmOrigin(arg.Origin, defaultArch, a.logger)
	if err != nil {
		return nil, apiservererrors.ServerError(err)
	}
	repo, err := a.getCharmRepository(ctx)
	if err != nil {
		return nil, apiservererrors.ServerError(err)
	}

	requestedOrigin, err := ConvertParamsOrigin(charmOrigin)
	if err != nil {
		return nil, apiservererrors.ServerError(err)
	}
	resources, err := repo.ListResources(ctx, curl.Name, requestedOrigin)
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

func normalizeCharmOrigin(origin params.CharmOrigin, fallbackArch string, logger corelogger.Logger) (params.CharmOrigin, error) {
	// If the series is set to all, we need to ensure that we remove that, so
	// that we can attempt to derive it at a later stage. Juju itself doesn't
	// know nor understand what "all" means, so we need to ensure it doesn't leak
	// out.
	o := origin
	if origin.Base.Name == "all" || origin.Base.Channel == "all" {
		logger.Warningf(context.TODO(), "Release all detected, removing all from the origin. %s", origin.ID)
		o.Base = params.Base{}
	}

	if origin.Architecture == "all" || origin.Architecture == "" {
		logger.Warningf(context.TODO(), "Architecture not in expected state, found %q, using fallback architecture %q. %s", origin.Architecture, fallbackArch, origin.ID)
		o.Architecture = fallbackArch
	}

	return o, nil
}

// charmURLFromLocator returns the charm URL for the current model.
func charmURLFromLocator(name string, locator applicationcharm.CharmLocator) (string, error) {
	schema, err := convertSource(locator.Source)
	if err != nil {
		return "", errors.Trace(err)
	}

	architecture, err := convertApplication(locator.Architecture)
	if err != nil {
		return "", errors.Trace(err)
	}

	url := charm.URL{
		Schema:       schema,
		Name:         name,
		Revision:     locator.Revision,
		Architecture: architecture,
	}
	return url.String(), nil
}

func convertSource(source applicationcharm.CharmSource) (string, error) {
	switch source {
	case applicationcharm.CharmHubSource:
		return charm.CharmHub.String(), nil
	case applicationcharm.LocalSource:
		return charm.Local.String(), nil
	default:
		return "", errors.Errorf("unsupported source %q", source)
	}
}

func convertApplication(a application.Architecture) (string, error) {
	switch a {
	case architecture.AMD64:
		return arch.AMD64, nil
	case architecture.ARM64:
		return arch.ARM64, nil
	case architecture.PPC64EL:
		return arch.PPC64EL, nil
	case architecture.S390X:
		return arch.S390X, nil
	case architecture.RISCV64:
		return arch.RISCV64, nil

	// This is a valid case if we're uploading charms and the value isn't
	// supplied.
	case architecture.Unknown:
		return "", nil
	default:
		return "", errors.Errorf("unsupported architecture %q", a)
	}
}
