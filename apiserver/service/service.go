// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package service contains api calls for functionality
// related to deploying and managing services and their
// related charms.
package service

import (
	"regexp"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/crossmodel"
	"github.com/juju/juju/apiserver/params"
	jjj "github.com/juju/juju/juju"
	jujucrossmodel "github.com/juju/juju/model/crossmodel"
	"github.com/juju/juju/state"
	statestorage "github.com/juju/juju/state/storage"
)

var (
	logger = loggo.GetLogger("juju.apiserver.service")

	newStateStorage = statestorage.NewStorage
)

func init() {
	common.RegisterStandardFacade("Service", 2, NewAPI)
}

// Service defines the methods on the service API end point.
type Service interface {
	// SetMetricCredentials sets credentials on the service.
	SetMetricCredentials(args params.ServiceMetricCredentials) (params.ErrorResults, error)

	// ServicesDeploy fetches the charms from the charm store and deploys them.
	ServicesDeploy(args params.ServicesDeploy) (params.ErrorResults, error)

	// ServicesDeployWithPlacement fetches the charms from the charm store and deploys them
	// using the specified placement directives.
	ServicesDeployWithPlacement(args params.ServicesDeploy) (params.ErrorResults, error)

	// ServiceUpdate updates the service attributes, including charm URL,
	// minimum number of units, settings and constraints.
	// All parameters in params.ServiceUpdate except the service name are optional.
	ServiceUpdate(args params.ServiceUpdate) error

	// ServiceSetCharm sets the charm for a given service.
	ServiceSetCharm(args params.ServiceSetCharm) error

	// ServiceGetCharmURL returns the charm URL the given service is
	// running at present.
	ServiceGetCharmURL(args params.ServiceGet) (params.StringResult, error)

	// AddRelation adds a relation between the specified endpoints and returns the relation info.
	AddRelation(args params.AddRelation) (params.AddRelationResults, error)

	// DestroyRelation removes the relation between the specified endpoints.
	DestroyRelation(args params.DestroyRelation) error
}

// API implements the service interface and is the concrete
// implementation of the api end point.
type API struct {
	check                   *common.BlockChecker
	state                   *state.State
	serviceOffersAPIFactory crossmodel.ServiceOffersAPIFactory
	authorizer              common.Authorizer
}

// NewAPI returns a new service API facade.
func NewAPI(
	st *state.State,
	resources *common.Resources,
	authorizer common.Authorizer,
) (Service, error) {
	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}
	apiFactory := resources.Get("serviceOffersApiFactory").(crossmodel.ServiceOffersAPIFactory)
	return &API{
		state:                   st,
		authorizer:              authorizer,
		serviceOffersAPIFactory: apiFactory,
		check: common.NewBlockChecker(st),
	}, nil
}

// SetMetricCredentials sets credentials on the service.
func (api *API) SetMetricCredentials(args params.ServiceMetricCredentials) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Creds)),
	}
	if len(args.Creds) == 0 {
		return result, nil
	}
	for i, a := range args.Creds {
		service, err := api.state.Service(a.ServiceName)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		err = service.SetMetricCredentials(a.MetricCredentials)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
		}
	}
	return result, nil
}

// ServicesDeploy fetches the charms from the charm store and deploys them.
func (api *API) ServicesDeploy(args params.ServicesDeploy) (params.ErrorResults, error) {
	return api.ServicesDeployWithPlacement(args)
}

// ServicesDeployWithPlacement fetches the charms from the charm store and deploys them
// using the specified placement directives.
func (api *API) ServicesDeployWithPlacement(args params.ServicesDeploy) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Services)),
	}
	if err := api.check.ChangeAllowed(); err != nil {
		return result, errors.Trace(err)
	}
	owner := api.authorizer.GetAuthTag().String()
	for i, arg := range args.Services {
		err := DeployService(api.state, owner, arg)
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// DeployService fetches the charm from the charm store and deploys it.
// The logic has been factored out into a common function which is called by
// both the legacy API on the client facade, as well as the new service facade.
func DeployService(st *state.State, owner string, args params.ServiceDeploy) error {
	curl, err := charm.ParseURL(args.CharmUrl)
	if err != nil {
		return errors.Trace(err)
	}
	if curl.Revision < 0 {
		return errors.Errorf("charm url must include revision")
	}

	// Do a quick but not complete validation check before going any further.
	if len(args.Placement) == 0 && args.ToMachineSpec != "" && names.IsValidMachine(args.ToMachineSpec) {
		_, err = st.Machine(args.ToMachineSpec)
		if err != nil {
			return errors.Annotatef(err, `cannot deploy "%v" to machine %v`, args.ServiceName, args.ToMachineSpec)
		}
	}

	// Try to find the charm URL in state first.
	ch, err := st.Charm(curl)
	if errors.IsNotFound(err) {
		// Clients written to expect 1.16 compatibility require this next block.
		if curl.Schema != "cs" {
			return errors.Errorf(`charm url has unsupported schema %q`, curl.Schema)
		}
		if err = AddCharmWithAuthorization(st, params.AddCharmWithAuthorization{
			URL: args.CharmUrl,
		}); err == nil {
			ch, err = st.Charm(curl)
		}
	}
	if err != nil {
		return errors.Trace(err)
	}

	var settings charm.Settings
	if len(args.ConfigYAML) > 0 {
		settings, err = ch.Config().ParseSettingsYAML([]byte(args.ConfigYAML), args.ServiceName)
	} else if len(args.Config) > 0 {
		// Parse config in a compatible way (see function comment).
		settings, err = parseSettingsCompatible(ch, args.Config)
	}
	if err != nil {
		return errors.Trace(err)
	}
	// Convert network tags to names for any given networks.
	requestedNetworks, err := networkTagsToNames(args.Networks)
	if err != nil {
		return errors.Trace(err)
	}

	_, err = jjj.DeployService(st,
		jjj.DeployServiceParams{
			ServiceName: args.ServiceName,
			Series:      args.Series,
			// TODO(dfc) ServiceOwner should be a tag
			ServiceOwner:   owner,
			Charm:          ch,
			NumUnits:       args.NumUnits,
			ConfigSettings: settings,
			Constraints:    args.Constraints,
			ToMachineSpec:  args.ToMachineSpec,
			Placement:      args.Placement,
			Networks:       requestedNetworks,
			Storage:        args.Storage,
		})
	return err
}

// ServiceSetSettingsStrings updates the settings for the given service,
// taking the configuration from a map of strings.
func ServiceSetSettingsStrings(service *state.Service, settings map[string]string) error {
	ch, _, err := service.Charm()
	if err != nil {
		return err
	}
	// Parse config in a compatible way (see function comment).
	changes, err := parseSettingsCompatible(ch, settings)
	if err != nil {
		return err
	}
	return service.UpdateConfigSettings(changes)
}

func networkTagsToNames(tags []string) ([]string, error) {
	netNames := make([]string, len(tags))
	for i, tag := range tags {
		t, err := names.ParseNetworkTag(tag)
		if err != nil {
			return nil, err
		}
		netNames[i] = t.Id()
	}
	return netNames, nil
}

// parseSettingsCompatible parses setting strings in a way that is
// compatible with the behavior before this CL based on the issue
// http://pad.lv/1194945. Until then setting an option to an empty
// string caused it to reset to the default value. We now allow
// empty strings as actual values, but we want to preserve the API
// behavior.
func parseSettingsCompatible(ch *state.Charm, settings map[string]string) (charm.Settings, error) {
	setSettings := map[string]string{}
	unsetSettings := charm.Settings{}
	// Split settings into those which set and those which unset a value.
	for name, value := range settings {
		if value == "" {
			unsetSettings[name] = nil
			continue
		}
		setSettings[name] = value
	}
	// Validate the settings.
	changes, err := ch.Config().ParseSettingsStrings(setSettings)
	if err != nil {
		return nil, err
	}
	// Validate the unsettings and merge them into the changes.
	unsetSettings, err = ch.Config().ValidateSettings(unsetSettings)
	if err != nil {
		return nil, err
	}
	for name := range unsetSettings {
		changes[name] = nil
	}
	return changes, nil
}

// ServiceUpdate updates the service attributes, including charm URL,
// minimum number of units, settings and constraints.
// All parameters in params.ServiceUpdate except the service name are optional.
func (api *API) ServiceUpdate(args params.ServiceUpdate) error {
	if !args.ForceCharmUrl {
		if err := api.check.ChangeAllowed(); err != nil {
			return errors.Trace(err)
		}
	}
	svc, err := api.state.Service(args.ServiceName)
	if err != nil {
		return err
	}
	// Set the charm for the given service.
	if args.CharmUrl != "" {
		if err = api.serviceSetCharm(svc, args.CharmUrl, args.ForceSeries, args.ForceCharmUrl); err != nil {
			return err
		}
	}
	// Set the minimum number of units for the given service.
	if args.MinUnits != nil {
		if err = svc.SetMinUnits(*args.MinUnits); err != nil {
			return err
		}
	}
	// Set up service's settings.
	if args.SettingsYAML != "" {
		if err = serviceSetSettingsYAML(svc, args.SettingsYAML); err != nil {
			return err
		}
	} else if len(args.SettingsStrings) > 0 {
		if err = ServiceSetSettingsStrings(svc, args.SettingsStrings); err != nil {
			return err
		}
	}
	// Update service's constraints.
	if args.Constraints != nil {
		return svc.SetConstraints(*args.Constraints)
	}
	return nil
}

// ServiceSetCharm sets the charm for a given service.
func (api *API) ServiceSetCharm(args params.ServiceSetCharm) error {
	// when forced units in error, don't block
	if !args.ForceUnits {
		if err := api.check.ChangeAllowed(); err != nil {
			return errors.Trace(err)
		}
	}
	service, err := api.state.Service(args.ServiceName)
	if err != nil {
		return err
	}
	return api.serviceSetCharm(service, args.CharmUrl, args.ForceSeries, args.ForceUnits)
}

// serviceSetCharm sets the charm for the given service.
func (api *API) serviceSetCharm(service *state.Service, url string, forceSeries, forceUnits bool) error {
	curl, err := charm.ParseURL(url)
	if err != nil {
		return err
	}
	sch, err := api.state.Charm(curl)
	if err != nil {
		return err
	}
	return service.SetCharm(sch, forceSeries, forceUnits)
}

// serviceSetSettingsYAML updates the settings for the given service,
// taking the configuration from a YAML string.
func serviceSetSettingsYAML(service *state.Service, settings string) error {
	ch, _, err := service.Charm()
	if err != nil {
		return err
	}
	changes, err := ch.Config().ParseSettingsYAML([]byte(settings), service.Name())
	if err != nil {
		return err
	}
	return service.UpdateConfigSettings(changes)
}

// ServiceGetCharmURL returns the charm URL the given service is
// running at present.
func (api *API) ServiceGetCharmURL(args params.ServiceGet) (params.StringResult, error) {
	service, err := api.state.Service(args.ServiceName)
	if err != nil {
		return params.StringResult{}, err
	}
	charmURL, _ := service.CharmURL()
	return params.StringResult{Result: charmURL.String()}, nil
}

// serviceUrlEndpointParse is used to split a service url and optional
// relation name into url and relation name.
var serviceUrlEndpointParse = regexp.MustCompile("(?P<url>.*/[^:]*)(:(?P<relname>.*))?")

// AddRelation adds a relation between the specified endpoints and returns the relation info.
func (api *API) AddRelation(args params.AddRelation) (params.AddRelationResults, error) {
	if err := api.check.ChangeAllowed(); err != nil {
		return params.AddRelationResults{}, errors.Trace(err)
	}

	endpoints := make([]string, len(args.Endpoints))
	// We may have a remote service passed in as the endpoint spec.
	// We'll iterate the endpoints to check.
	for i, ep := range args.Endpoints {
		endpoints[i] = ep

		// If the endpoint is not remote, skip it.
		// We first need to strip off any relation name
		// which may have been appended to the URL, then
		// we try parsing the URL.
		possibleURL := serviceUrlEndpointParse.ReplaceAllString(ep, "$url")
		relName := serviceUrlEndpointParse.ReplaceAllString(ep, "$relname")

		// If the URL parses, we need to look up the remote service
		// details and save to state.
		url, err := jujucrossmodel.ParseServiceURL(possibleURL)
		if err != nil {
			// Not a URL.
			continue
		}
		// Save the remote service details into state.
		rs, err := saveRemoteService(api.state, api.serviceOffersAPIFactory, *url)
		if err != nil {
			return params.AddRelationResults{}, errors.Trace(err)
		}
		// The endpoint is named after the remote service name,
		// not the service name from the URL.
		endpoints[i] = rs.Name()
		if relName != "" {
			endpoints[i] = rs.Name() + ":" + relName
		}
	}

	inEps, err := api.state.InferEndpoints(endpoints...)
	if err != nil {
		return params.AddRelationResults{}, err
	}
	rel, err := api.state.AddRelation(inEps...)
	if err != nil {
		return params.AddRelationResults{}, err
	}
	outEps := make(map[string]charm.Relation)
	for _, inEp := range inEps {
		outEp, err := rel.Endpoint(inEp.ServiceName)
		if err != nil {
			return params.AddRelationResults{}, err
		}
		outEps[inEp.ServiceName] = outEp.Relation
	}
	return params.AddRelationResults{Endpoints: outEps}, nil
}

// saveRemoteService takes a remote service URL and retrieves the details of the service from
// the relevant service directory. These details are saved to the state model so relations to
// the remote service can be created.
func saveRemoteService(
	st *state.State, apiFactory crossmodel.ServiceOffersAPIFactory, url jujucrossmodel.ServiceURL,
) (*state.RemoteService, error) {
	offersAPI, err := apiFactory.ServiceOffers(url.Directory)
	if err != nil {
		return nil, errors.Trace(err)
	}
	offers, err := offersAPI.ListOffers(params.OfferFilters{
		Directory: url.Directory,
		Filters: []params.OfferFilter{
			{
				ServiceURL: url.String(),
			},
		},
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	if offers.Error != nil {
		return nil, errors.Trace(offers.Error)
	}
	// The offers query succeeded but there were no offers matching the URL.
	if len(offers.Offers) == 0 {
		return nil, errors.NotFoundf("service offer %q", url.String())
	}

	// Create a remote service entry in the model for the consumed service.
	offer := offers.Offers[0]
	rs, err := st.RemoteService(offer.ServiceName)
	if err != nil && !errors.IsNotFound(err) {
		return nil, errors.Trace(err)
	}
	if err == nil {
		// TODO (wallyworld) - update service if it exists already with any additional endpoints
		return rs, nil
	}
	remoteEps := make([]charm.Relation, len(offer.Endpoints))
	for j, ep := range offer.Endpoints {
		remoteEps[j] = charm.Relation{
			Name:      ep.Name,
			Role:      ep.Role,
			Interface: ep.Interface,
			Limit:     ep.Limit,
			Scope:     ep.Scope,
		}
	}
	rs, err = st.AddRemoteService(offer.ServiceName, url.String(), remoteEps)
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			return nil, errors.Trace(err)
		}
		// Remote service didn't exist but now there's a clash
		// trying to save it. It could be a local service with the
		// same name or a remote service with the same name but we
		// have no idea whether endpoints are compatible or not.
		// Best just to error.
		return nil, errors.Annotatef(err, "saving endpoints for service at URL %q", url.String())
	}
	return rs, nil
}

// DestroyRelation removes the relation between the specified endpoints.
func (api *API) DestroyRelation(args params.DestroyRelation) error {
	if err := api.check.RemoveAllowed(); err != nil {
		return errors.Trace(err)
	}
	eps, err := api.state.InferEndpoints(args.Endpoints...)
	if err != nil {
		return err
	}
	rel, err := api.state.EndpointsRelation(eps...)
	if err != nil {
		return err
	}
	return rel.Destroy()
}
