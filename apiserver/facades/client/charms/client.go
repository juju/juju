// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms

import (
	"github.com/juju/charm/v8"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"gopkg.in/macaroon.v2"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/charmhub"
	"github.com/juju/juju/controller"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
)

type BackendState interface {
	Charm(curl *charm.URL) (*state.Charm, error)
	ControllerConfig() (controller.Config, error)
	AllCharms() ([]*state.Charm, error)
}

type BackendModel interface {
	Config() (*config.Config, error)
	ModelTag() names.ModelTag
}

// API implements the charms interface and is the concrete
// implementation of the API end point.
type API struct {
	authorizer   facade.Authorizer
	backendState BackendState
	backendModel BackendModel

	csResolverGetterFunc CSResolverGetterFunc
	tag                  names.ModelTag
}

type APIv2 struct {
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

// NewFacadeV3 provides the signature required for facade V3 registration.
func NewFacadeV3(ctx facade.Context) (*API, error) {
	authorizer := ctx.Auth()
	if !authorizer.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}

	st := ctx.State()
	m, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}

	return &API{
		authorizer:           authorizer,
		backendState:         st,
		backendModel:         m,
		csResolverGetterFunc: csResolverGetter,
		tag:                  m.ModelTag(),
	}, nil
}

// NewFacade provides the signature required for facade V2 registration.
// It is unknown where V1 is.
func NewFacade(ctx facade.Context) (*APIv2, error) {
	v3, err := NewFacadeV3(ctx)
	if err != nil {
		return nil, nil
	}
	return &APIv2{v3}, nil
}

func NewCharmsAPI(authorizer facade.Authorizer, st BackendState, m BackendModel, csResolverFunc CSResolverGetterFunc) (*API, error) {
	return &API{
		authorizer:           authorizer,
		backendState:         st,
		backendModel:         m,
		csResolverGetterFunc: csResolverFunc,
		tag:                  m.ModelTag(),
	}, nil
}

// CharmInfo returns information about the requested charm.
// NOTE: thumper 2016-06-29, this is not a bulk call and probably should be.
func (a *API) CharmInfo(args params.CharmURL) (params.Charm, error) {
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
// If supplied parameter contains any names, the result will be filtered
// to return only the charms with supplied names.
func (a *API) List(args params.CharmsList) (params.CharmsListResult, error) {
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

// ResolveCharms is not available via the V2 API.
func (a *APIv2) ResolveCharms(_ struct{}) {}

// ResolveCharms resolves the given charm URLs with an optionally specified
// preferred channel.  Channel provided via CharmOrigin.
func (a *API) ResolveCharms(args params.ResolveCharmsWithChannel) (params.ResolveCharmWithChannelResults, error) {
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

// Repository is the part of charmrepo.Charmstore that we need to
// resolve a charm url and get a charm archive.
type Repository interface {
	// Get reads the charm referenced by curl into a file
	// with the given path, which will be created if needed. Note that
	// the path's parent directory must already exist.
	Get(curl *charm.URL, archivePath string) (*charm.CharmArchive, error)
	ResolveWithPreferredChannel(*charm.URL, params.CharmOrigin) (*charm.URL, params.CharmOrigin, []string, error)
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
	result.Origin = origin
	switch {
	case resultURL.Series != "" && len(supportedSeries) == 0:
		result.SupportedSeries = []string{resultURL.Series}
	default:
		result.SupportedSeries = supportedSeries
	}

	return result
}

func (a *API) repository(origin params.CharmOrigin, mac *macaroon.Macaroon) (Repository, error) {
	switch origin.Source {
	case corecharm.CharmHub.String():
		return a.charmHubRepository()
	case corecharm.CharmStore.String():
		return a.charmStoreRepository(origin, mac)
	}
	return nil, errors.BadRequestf("Not charm hub nor charm store charm")
}

func (a *API) charmStoreRepository(origin params.CharmOrigin, mac *macaroon.Macaroon) (Repository, error) {
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

func (a *API) charmHubRepository() (Repository, error) {
	cfg, err := a.backendModel.Config()
	if err != nil {
		return nil, errors.Trace(err)
	}
	var chCfg charmhub.Config
	chURL, ok := cfg.CharmHubURL()
	if ok {
		chCfg = charmhub.CharmHubConfigFromURL(chURL)
	} else {
		chCfg = charmhub.CharmHubConfig()
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
