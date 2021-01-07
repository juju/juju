// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms

import (
	"fmt"
	"io"
	"strings"

	"github.com/juju/charm/v8"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/juju/state"
	"github.com/juju/names/v4"
	"github.com/juju/os/v2/series"
	"github.com/juju/utils/v2"
	"gopkg.in/macaroon.v2"
	"gopkg.in/mgo.v2"

	charmscommon "github.com/juju/juju/apiserver/common/charms"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	charmsinterfaces "github.com/juju/juju/apiserver/facades/client/charms/interfaces"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/charmhub"
	"github.com/juju/juju/core/arch"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/permission"
	stateerrors "github.com/juju/juju/state/errors"
	"github.com/juju/juju/state/storage"
	jujuversion "github.com/juju/juju/version"
)

// API implements the charms interface and is the concrete
// implementation of the API end point.
type API struct {
	*charmscommon.CharmsAPI
	authorizer   facade.Authorizer
	backendState charmsinterfaces.BackendState
	backendModel charmsinterfaces.BackendModel

	csResolverGetterFunc CSResolverGetterFunc
	getStrategyFunc      func(source string) StrategyFunc
	newStorage           func(modelUUID string, session *mgo.Session) storage.Storage
	tag                  names.ModelTag
}

type APIv2 struct {
	*APIv3
}

type APIv3 struct {
	*API
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

	commonCharmsAPI, err := charmscommon.NewCharmsAPI(st, authorizer)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return &API{
		CharmsAPI:            commonCharmsAPI,
		authorizer:           authorizer,
		backendState:         newStateShim(st),
		backendModel:         m,
		csResolverGetterFunc: csResolverGetter,
		getStrategyFunc:      getStrategyFunc,
		newStorage:           storage.NewStorage,
		tag:                  m.ModelTag(),
	}, nil
}

func NewCharmsAPI(
	authorizer facade.Authorizer,
	st charmsinterfaces.BackendState,
	m charmsinterfaces.BackendModel,
	csResolverFunc CSResolverGetterFunc,
	getStrategyFunc func(source string) StrategyFunc,
	newStorage func(modelUUID string, session *mgo.Session) storage.Storage,
) (*API, error) {
	return &API{
		authorizer:           authorizer,
		backendState:         st,
		backendModel:         m,
		csResolverGetterFunc: csResolverFunc,
		getStrategyFunc:      getStrategyFunc,
		newStorage:           newStorage,
		tag:                  m.ModelTag(),
	}, nil
}

// CharmInfo returns information about the requested charm.
// NOTE: thumper 2016-06-29, this is not a bulk call and probably should be.
func (a *API) CharmInfo(args params.CharmURL) (params.Charm, error) {
	logger.Tracef("CharmInfo 1 %+v", args)
	if err := a.checkCanRead(); err != nil {
		return params.Charm{}, errors.Trace(err)
	}

	curl, err := charm.ParseURL(args.URL)
	if err != nil {
		return params.Charm{}, errors.Trace(err)
	}
	aCharm, err := a.backendState.Charm(curl)
	if err != nil {
		return params.Charm{}, errors.Trace(err)
	}
	info := params.Charm{
		Revision: aCharm.Revision(),
		URL:      curl.String(),
		Config:   params.ToCharmOptionMap(aCharm.Config()),
		Meta:     convertCharmMeta(aCharm.Meta()),
		Actions:  convertCharmActions(aCharm.Actions()),
		Metrics:  convertCharmMetrics(aCharm.Metrics()),
	}

	// we don't need to check that this is a charm.LXDProfiler, as we can
	// state that the function exists.
	if profile := aCharm.LXDProfile(); profile != nil && !profile.Empty() {
		info.LXDProfile = convertCharmLXDProfile(profile)
	}

	return info, nil
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

	repo, err := a.repository(charmOrigin, arg.Macaroon)
	if err != nil {
		return params.DownloadInfoResult{}, apiservererrors.ServerError(err)
	}

	url, origin, err := repo.FindDownloadURL(curl, convertParamsOrigin(charmOrigin))
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

	if cons.HasArch() {
		return *cons.Arch, nil
	}

	return arch.DefaultArchitecture, nil
}

func normalizeCharmOrigin(origin params.CharmOrigin, fallbackArch string) (params.CharmOrigin, error) {
	// If the series is set to all, we need to ensure that we remove that, so
	// that we can attempt to derive it at a later stage. Juju itself doesn't
	// know nor understands what all means, so we need to ensure it doesn't leak
	// out.
	var os string
	var oSeries string
	if origin.Series == "all" {
		logger.Warningf("Series all detected, removing all from the origin. %s", origin.ID)
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
		logger.Warningf("Architecture all detected, removing all from the origin. %s", origin.ID)
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

	strategy, err := a.charmStrategy(args)
	if err != nil {
		return params.CharmOriginResult{}, errors.Trace(err)
	}

	// Validate the strategy before running the download procedure.
	if err := strategy.Validate(); err != nil {
		return params.CharmOriginResult{}, errors.Trace(err)
	}

	defer func() {
		// Ensure we sign up any required clean ups.
		_ = strategy.Finish()
	}()

	// Run the strategy.
	result, alreadyExists, origin, err := strategy.Run(a.backendState, versionValidator{}, convertParamsOrigin(args.Origin))
	if err != nil {
		return params.CharmOriginResult{}, errors.Trace(err)
	} else if alreadyExists {
		// Nothing to do here, as it already exists in state.
		// However we still need the origin with ID and hash for
		// CharmHub charms.
		return params.CharmOriginResult{
			Origin: convertOrigin(origin),
		}, nil
	}

	ca := CharmArchive{
		ID:           strategy.CharmURL(),
		Charm:        result.Charm,
		Data:         result.Data,
		Size:         result.Size,
		SHA256:       result.SHA256,
		CharmVersion: result.Charm.Version(),
	}

	if args.CharmStoreMacaroon != nil {
		ca.Macaroon = macaroon.Slice{args.CharmStoreMacaroon}
	}

	OriginResult := params.CharmOriginResult{
		Origin: convertOrigin(origin),
	}

	// Store the charm archive in environment storage.
	if err = a.storeCharmArchive(ca); err != nil {
		OriginResult.Error = apiservererrors.ServerError(err)
	}

	return OriginResult, nil
}

type versionValidator struct{}

func (versionValidator) Validate(meta *charm.Meta) error {
	return jujuversion.CheckJujuMinVersion(meta.MinJujuVersion, jujuversion.Current)
}

// CharmArchive is the data that needs to be stored for a charm archive in
// state.
type CharmArchive struct {
	// ID is the charm URL for which we're storing the archive.
	ID *charm.URL

	// Charm is the metadata about the charm for the archive.
	Charm charm.Charm

	// Data contains the bytes of the archive.
	Data io.Reader

	// Size is the number of bytes in Data.
	Size int64

	// SHA256 is the hash of the bytes in Data.
	SHA256 string

	// Macaroon is the authorization macaroon for accessing the charmstore.
	Macaroon macaroon.Slice

	// Charm Version contains semantic version of charm, typically the output of git describe.
	CharmVersion string
}

// storeCharmArchive stores a charm archive in environment storage.
//
// TODO: (hml) 2020-09-01
// This is a duplicate of application.StoreCharmArchive.  Once use
// is transferred to this facade, it can be marked deprecated.
func (a *API) storeCharmArchive(archive CharmArchive) error {
	logger.Tracef("storeCharmArchive %q", archive.ID)
	storage := a.newStorage(a.backendState.ModelUUID(), a.backendState.MongoSession())
	storagePath, err := charmArchiveStoragePath(archive.ID)
	if err != nil {
		return errors.Annotate(err, "cannot generate charm archive name")
	}
	if err := storage.Put(storagePath, archive.Data, archive.Size); err != nil {
		return errors.Annotate(err, "cannot add charm to storage")
	}

	info := state.CharmInfo{
		Charm:       archive.Charm,
		ID:          archive.ID,
		StoragePath: storagePath,
		SHA256:      archive.SHA256,
		Macaroon:    archive.Macaroon,
		Version:     archive.CharmVersion,
	}

	// Now update the charm data in state and mark it as no longer pending.
	_, err = a.backendState.UpdateUploadedCharm(info)
	if err != nil {
		alreadyUploaded := err == stateerrors.ErrCharmRevisionAlreadyModified ||
			errors.Cause(err) == stateerrors.ErrCharmRevisionAlreadyModified ||
			stateerrors.IsCharmAlreadyUploadedError(err)
		if err := storage.Remove(storagePath); err != nil {
			if alreadyUploaded {
				logger.Errorf("cannot remove duplicated charm archive from storage: %v", err)
			} else {
				logger.Errorf("cannot remove unsuccessfully recorded charm archive from storage: %v", err)
			}
		}
		if alreadyUploaded {
			// Somebody else managed to upload and update the charm in
			// state before us. This is not an error.
			return nil
		}
		return errors.Trace(err)
	}
	return nil
}

// charmArchiveStoragePath returns a string that is suitable as a
// storage path, using a random UUID to avoid colliding with concurrent
// uploads.
func charmArchiveStoragePath(curl *charm.URL) (string, error) {
	uuid, err := utils.NewUUID()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("charms/%s-%s", curl.String(), uuid), nil
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

	// Viladate the origin passed in.
	if err := validateOrigin(arg.Origin, curl.Schema); err != nil {
		result.Error = apiservererrors.ServerError(err)
		return result
	}

	// If we can guarantee that each charm to be resolved uses the
	// same url source and channel, there is no need to get a new repository
	// each time.
	resolver, err := a.repository(arg.Origin, mac)
	if err != nil {
		result.Error = apiservererrors.ServerError(err)
		return result
	}

	resultURL, origin, supportedSeries, err := resolver.ResolveWithPreferredChannel(curl, arg.Origin)
	if err != nil {
		result.Error = apiservererrors.ServerError(err)
		return result
	}
	result.URL = resultURL.String()

	// The charmhub API can return "all" for architecture as it's not a real
	// arch we don't know how to correctly model it. "all " doesn't mean use the
	// default arch, it means use any arch which isn't quite the same. So if we
	// do get "all" we should see if there is a clean way to resolve it.
	archOrigin := origin
	if origin.Architecture == "all" {
		cons, err := a.backendState.ModelConstraints()
		if err != nil {
			result.Error = apiservererrors.ServerError(err)
			return result
		}
		if cons.HasArch() {
			archOrigin.Architecture = *cons.Arch
		} else {
			archOrigin.Architecture = arch.DefaultArchitecture
		}
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

func validateOrigin(origin params.CharmOrigin, schema string) error {
	if (corecharm.Local.Matches(origin.Source) && !charm.Local.Matches(schema)) ||
		(corecharm.CharmStore.Matches(origin.Source) && !charm.CharmStore.Matches(schema)) ||
		(corecharm.CharmHub.Matches(origin.Source) && !charm.CharmHub.Matches(schema)) {
		return errors.NotValidf("origin source %q with schema", origin.Source)
	}
	if origin.Architecture == "" {
		return errors.NotValidf("empty architecture")
	}
	return nil
}

func (a *API) charmStrategy(args params.AddCharmWithAuth) (Strategy, error) {
	repo, err := a.repository(args.Origin, args.CharmStoreMacaroon)
	if err != nil {
		return nil, err
	}
	fn := a.getStrategyFunc(args.Origin.Source)
	return fn(repo, args.URL, args.Force)
}

// StrategyFunc defines a function for executing a strategy for downloading a
// charm.
type StrategyFunc func(charmRepo corecharm.Repository, url string, force bool) (Strategy, error)

func getStrategyFunc(source string) StrategyFunc {
	if source == "charm-store" {
		return func(charmRepo corecharm.Repository, url string, force bool) (Strategy, error) {
			return corecharm.DownloadFromCharmStore(logger.Child("strategy"), charmRepo, url, force)
		}
	}
	return func(charmRepo corecharm.Repository, url string, force bool) (Strategy, error) {
		return corecharm.DownloadFromCharmHub(logger.Child("strategy"), charmRepo, url, force)
	}
}

func (a *API) repository(origin params.CharmOrigin, mac *macaroon.Macaroon) (corecharm.Repository, error) {
	switch origin.Source {
	case corecharm.CharmHub.String():
		return a.charmHubRepository()
	case corecharm.CharmStore.String():
		return a.charmStoreRepository(origin, mac)
	}
	return nil, errors.BadRequestf("Not charm hub nor charm store charm")
}

func (a *API) charmStoreRepository(origin params.CharmOrigin, mac *macaroon.Macaroon) (corecharm.Repository, error) {
	controllerCfg, err := a.backendState.ControllerConfig()
	if err != nil {
		return nil, errors.Trace(err)
	}
	client, err := a.csResolverGetterFunc(
		ResolverGetterParams{
			CSURL:              controllerCfg.CharmStoreURL(),
			Channel:            origin.Risk,
			CharmStoreMacaroon: mac,
		})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &csRepo{repo: client}, nil
}

func (a *API) charmHubRepository() (corecharm.Repository, error) {
	cfg, err := a.backendModel.Config()
	if err != nil {
		return nil, errors.Trace(err)
	}
	var chCfg charmhub.Config
	chURL, ok := cfg.CharmHubURL()
	if ok {
		chCfg, err = charmhub.CharmHubConfigFromURL(chURL, logger.Child("client"))
	} else {
		chCfg, err = charmhub.CharmHubConfig(logger.Child("client"))
	}
	if err != nil {
		return nil, errors.Trace(err)
	}

	chClient, err := charmhub.NewClient(chCfg)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &chRepo{chClient}, nil
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
