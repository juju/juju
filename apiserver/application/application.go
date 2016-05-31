// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package application contains api calls for functionality
// related to deploying and managing applications and their
// related charms.
package application

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/charm.v6-unstable"
	csparams "gopkg.in/juju/charmrepo.v2-unstable/csclient/params"
	goyaml "gopkg.in/yaml.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/instance"
	jjj "github.com/juju/juju/juju"
	"github.com/juju/juju/state"
	statestorage "github.com/juju/juju/state/storage"
)

var (
	logger = loggo.GetLogger("juju.apiserver.application")

	newStateStorage = statestorage.NewStorage
)

func init() {
	common.RegisterStandardFacade("Application", 1, NewAPI)
}

// Application defines the methods on the application API end point.
type Application interface {
	SetMetricCredentials(args params.ApplicationMetricCredentials) (params.ErrorResults, error)
}

// API implements the application interface and is the concrete
// implementation of the api end point.
type API struct {
	check      *common.BlockChecker
	state      *state.State
	authorizer common.Authorizer
}

// NewAPI returns a new application API facade.
func NewAPI(
	st *state.State,
	resources *common.Resources,
	authorizer common.Authorizer,
) (*API, error) {
	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}

	return &API{
		state:      st,
		authorizer: authorizer,
		check:      common.NewBlockChecker(st),
	}, nil
}

// SetMetricCredentials sets credentials on the application.
func (api *API) SetMetricCredentials(args params.ApplicationMetricCredentials) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Creds)),
	}
	if len(args.Creds) == 0 {
		return result, nil
	}
	for i, a := range args.Creds {
		service, err := api.state.Application(a.ApplicationName)
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

// Deploy fetches the charms from the charm store and deploys them
// using the specified placement directives.
func (api *API) Deploy(args params.ApplicationsDeploy) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Applications)),
	}
	if err := api.check.ChangeAllowed(); err != nil {
		return result, errors.Trace(err)
	}
	owner := api.authorizer.GetAuthTag().String()
	for i, arg := range args.Applications {
		err := deployService(api.state, owner, arg)
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// deployService fetches the charm from the charm store and deploys it.
// The logic has been factored out into a common function which is called by
// both the legacy API on the client facade, as well as the new service facade.
func deployService(st *state.State, owner string, args params.ApplicationDeploy) error {
	curl, err := charm.ParseURL(args.CharmUrl)
	if err != nil {
		return errors.Trace(err)
	}
	if curl.Revision < 0 {
		return errors.Errorf("charm url must include revision")
	}

	// Do a quick but not complete validation check before going any further.
	for _, p := range args.Placement {
		if p.Scope != instance.MachineScope {
			continue
		}
		_, err = st.Machine(p.Directive)
		if err != nil {
			return errors.Annotatef(err, `cannot deploy "%v" to machine %v`, args.ApplicationName, p.Directive)
		}
	}

	// Try to find the charm URL in state first.
	ch, err := st.Charm(curl)
	// TODO(wallyworld) - remove for 2.0 beta4
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

	if err := checkMinVersion(ch); err != nil {
		return errors.Trace(err)
	}

	var settings charm.Settings
	if len(args.ConfigYAML) > 0 {
		settings, err = ch.Config().ParseSettingsYAML([]byte(args.ConfigYAML), args.ApplicationName)
	} else if len(args.Config) > 0 {
		// Parse config in a compatible way (see function comment).
		settings, err = parseSettingsCompatible(ch, args.Config)
	}
	if err != nil {
		return errors.Trace(err)
	}

	channel := csparams.Channel(args.Channel)

	_, err = jjj.DeployService(st,
		jjj.DeployApplicationParams{
			ApplicationName: args.ApplicationName,
			Series:          args.Series,
			// TODO(dfc) ApplicationOwner should be a tag
			ApplicationOwner: owner,
			Charm:            ch,
			Channel:          channel,
			NumUnits:         args.NumUnits,
			ConfigSettings:   settings,
			Constraints:      args.Constraints,
			Placement:        args.Placement,
			Storage:          args.Storage,
			EndpointBindings: args.EndpointBindings,
			Resources:        args.Resources,
		})
	return errors.Trace(err)
}

// ServiceSetSettingsStrings updates the settings for the given service,
// taking the configuration from a map of strings.
func ServiceSetSettingsStrings(service *state.Application, settings map[string]string) error {
	ch, _, err := service.Charm()
	if err != nil {
		return errors.Trace(err)
	}
	// Parse config in a compatible way (see function comment).
	changes, err := parseSettingsCompatible(ch, settings)
	if err != nil {
		return errors.Trace(err)
	}
	return service.UpdateConfigSettings(changes)
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

// Update updates the service attributes, including charm URL,
// minimum number of units, settings and constraints.
// All parameters in params.ServiceUpdate except the service name are optional.
func (api *API) Update(args params.ApplicationUpdate) error {
	if !args.ForceCharmUrl {
		if err := api.check.ChangeAllowed(); err != nil {
			return errors.Trace(err)
		}
	}
	svc, err := api.state.Application(args.ApplicationName)
	if err != nil {
		return errors.Trace(err)
	}
	// Set the charm for the given service.
	if args.CharmUrl != "" {
		// For now we do not support changing the channel through Update().
		// TODO(ericsnow) Support it?
		channel := svc.Channel()
		if err = api.applicationSetCharm(svc, args.CharmUrl, channel, args.ForceSeries, args.ForceCharmUrl, nil); err != nil {
			return errors.Trace(err)
		}
	}
	// Set the minimum number of units for the given service.
	if args.MinUnits != nil {
		if err = svc.SetMinUnits(*args.MinUnits); err != nil {
			return errors.Trace(err)
		}
	}
	// Set up service's settings.
	if args.SettingsYAML != "" {
		if err = serviceSetSettingsYAML(svc, args.SettingsYAML); err != nil {
			return errors.Annotate(err, "setting configuration from YAML")
		}
	} else if len(args.SettingsStrings) > 0 {
		if err = ServiceSetSettingsStrings(svc, args.SettingsStrings); err != nil {
			return errors.Trace(err)
		}
	}
	// Update service's constraints.
	if args.Constraints != nil {
		return svc.SetConstraints(*args.Constraints)
	}
	return nil
}

// SetCharm sets the charm for a given for the application.
func (api *API) SetCharm(args params.ApplicationSetCharm) error {
	// when forced units in error, don't block
	if !args.ForceUnits {
		if err := api.check.ChangeAllowed(); err != nil {
			return errors.Trace(err)
		}
	}
	service, err := api.state.Application(args.ApplicationName)
	if err != nil {
		return errors.Trace(err)
	}
	channel := csparams.Channel(args.Channel)
	return api.applicationSetCharm(service, args.CharmUrl, channel, args.ForceSeries, args.ForceUnits, args.ResourceIDs)
}

// serviceSetCharm sets the charm for the given for the application.
func (api *API) applicationSetCharm(application *state.Application, url string, channel csparams.Channel, forceSeries, forceUnits bool, resourceIDs map[string]string) error {
	curl, err := charm.ParseURL(url)
	if err != nil {
		return errors.Trace(err)
	}
	sch, err := api.state.Charm(curl)
	if err != nil {
		return errors.Trace(err)
	}
	cfg := state.SetCharmConfig{
		Charm:       sch,
		Channel:     channel,
		ForceSeries: forceSeries,
		ForceUnits:  forceUnits,
		ResourceIDs: resourceIDs,
	}
	return application.SetCharm(cfg)
}

// settingsYamlFromGetYaml will parse a yaml produced by juju get and generate
// charm.Settings from it that can then be sent to the service.
func settingsFromGetYaml(yamlContents map[string]interface{}) (charm.Settings, error) {
	onlySettings := charm.Settings{}
	settingsMap, ok := yamlContents["settings"].(map[interface{}]interface{})
	if !ok {
		return nil, errors.New("unknown format for settings")
	}

	for setting := range settingsMap {
		s, ok := settingsMap[setting].(map[interface{}]interface{})
		if !ok {
			return nil, errors.Errorf("unknown format for settings section %v", setting)
		}
		// some keys might not have a value, we don't care about those.
		v, ok := s["value"]
		if !ok {
			continue
		}
		stringSetting, ok := setting.(string)
		if !ok {
			return nil, errors.Errorf("unexpected setting key, expected string got %T", setting)
		}
		onlySettings[stringSetting] = v
	}
	return onlySettings, nil
}

// serviceSetSettingsYAML updates the settings for the given service,
// taking the configuration from a YAML string.
func serviceSetSettingsYAML(service *state.Application, settings string) error {
	b := []byte(settings)
	var all map[string]interface{}
	if err := goyaml.Unmarshal(b, &all); err != nil {
		return errors.Annotate(err, "parsing settings data")
	}
	// The file is already in the right format.
	if _, ok := all[service.Name()]; !ok {
		changes, err := settingsFromGetYaml(all)
		if err != nil {
			return errors.Annotate(err, "processing YAML generated by get")
		}
		return errors.Annotate(service.UpdateConfigSettings(changes), "updating settings with application YAML")
	}

	ch, _, err := service.Charm()
	if err != nil {
		return errors.Annotate(err, "obtaining charm for this application")
	}

	changes, err := ch.Config().ParseSettingsYAML(b, service.Name())
	if err != nil {
		return errors.Annotate(err, "creating config from YAML")
	}
	return errors.Annotate(service.UpdateConfigSettings(changes), "updating settings")
}

// GetCharmURL returns the charm URL the given service is
// running at present.
func (api *API) GetCharmURL(args params.ApplicationGet) (params.StringResult, error) {
	service, err := api.state.Application(args.ApplicationName)
	if err != nil {
		return params.StringResult{}, err
	}
	charmURL, _ := service.CharmURL()
	return params.StringResult{Result: charmURL.String()}, nil
}

// Set implements the server side of Service.Set.
// It does not unset values that are set to an empty string.
// Unset should be used for that.
func (api *API) Set(p params.ApplicationSet) error {
	if err := api.check.ChangeAllowed(); err != nil {
		return errors.Trace(err)
	}
	svc, err := api.state.Application(p.ApplicationName)
	if err != nil {
		return err
	}
	ch, _, err := svc.Charm()
	if err != nil {
		return err
	}
	// Validate the settings.
	changes, err := ch.Config().ParseSettingsStrings(p.Options)
	if err != nil {
		return err
	}

	return svc.UpdateConfigSettings(changes)

}

// Unset implements the server side of Client.Unset.
func (api *API) Unset(p params.ApplicationUnset) error {
	if err := api.check.ChangeAllowed(); err != nil {
		return errors.Trace(err)
	}
	svc, err := api.state.Application(p.ApplicationName)
	if err != nil {
		return err
	}
	settings := make(charm.Settings)
	for _, option := range p.Options {
		settings[option] = nil
	}
	return svc.UpdateConfigSettings(settings)
}

// CharmRelations implements the server side of Service.CharmRelations.
func (api *API) CharmRelations(p params.ApplicationCharmRelations) (params.ApplicationCharmRelationsResults, error) {
	var results params.ApplicationCharmRelationsResults
	service, err := api.state.Application(p.ApplicationName)
	if err != nil {
		return results, err
	}
	endpoints, err := service.Endpoints()
	if err != nil {
		return results, err
	}
	results.CharmRelations = make([]string, len(endpoints))
	for i, endpoint := range endpoints {
		results.CharmRelations[i] = endpoint.Relation.Name
	}
	return results, nil
}

// Expose changes the juju-managed firewall to expose any ports that
// were also explicitly marked by units as open.
func (api *API) Expose(args params.ApplicationExpose) error {
	if err := api.check.ChangeAllowed(); err != nil {
		return errors.Trace(err)
	}
	svc, err := api.state.Application(args.ApplicationName)
	if err != nil {
		return err
	}
	return svc.SetExposed()
}

// Unexpose changes the juju-managed firewall to unexpose any ports that
// were also explicitly marked by units as open.
func (api *API) Unexpose(args params.ApplicationUnexpose) error {
	if err := api.check.ChangeAllowed(); err != nil {
		return errors.Trace(err)
	}
	svc, err := api.state.Application(args.ApplicationName)
	if err != nil {
		return err
	}
	return svc.ClearExposed()
}

// addServiceUnits adds a given number of units to a service.
func addServiceUnits(st *state.State, args params.AddApplicationUnits) ([]*state.Unit, error) {
	service, err := st.Application(args.ApplicationName)
	if err != nil {
		return nil, err
	}
	if args.NumUnits < 1 {
		return nil, errors.New("must add at least one unit")
	}
	return jjj.AddUnits(st, service, args.NumUnits, args.Placement)
}

// AddUnits adds a given number of units to a service.
func (api *API) AddUnits(args params.AddApplicationUnits) (params.AddApplicationUnitsResults, error) {
	if err := api.check.ChangeAllowed(); err != nil {
		return params.AddApplicationUnitsResults{}, errors.Trace(err)
	}
	units, err := addServiceUnits(api.state, args)
	if err != nil {
		return params.AddApplicationUnitsResults{}, err
	}
	unitNames := make([]string, len(units))
	for i, unit := range units {
		unitNames[i] = unit.String()
	}
	return params.AddApplicationUnitsResults{Units: unitNames}, nil
}

// DestroyUnits removes a given set of service units.
func (api *API) DestroyUnits(args params.DestroyApplicationUnits) error {
	if err := api.check.RemoveAllowed(); err != nil {
		return errors.Trace(err)
	}
	var errs []string
	for _, name := range args.UnitNames {
		unit, err := api.state.Unit(name)
		switch {
		case errors.IsNotFound(err):
			err = errors.Errorf("unit %q does not exist", name)
		case err != nil:
		case unit.Life() != state.Alive:
			continue
		case unit.IsPrincipal():
			err = unit.Destroy()
		default:
			err = errors.Errorf("unit %q is a subordinate", name)
		}
		if err != nil {
			errs = append(errs, err.Error())
		}
	}
	return common.DestroyErr("units", args.UnitNames, errs)
}

// Destroy destroys a given service.
func (api *API) Destroy(args params.ApplicationDestroy) error {
	if err := api.check.RemoveAllowed(); err != nil {
		return errors.Trace(err)
	}
	svc, err := api.state.Application(args.ApplicationName)
	if err != nil {
		return err
	}
	return svc.Destroy()
}

// GetConstraints returns the constraints for a given service.
func (api *API) GetConstraints(args params.GetApplicationConstraints) (params.GetConstraintsResults, error) {
	svc, err := api.state.Application(args.ApplicationName)
	if err != nil {
		return params.GetConstraintsResults{}, err
	}
	cons, err := svc.Constraints()
	return params.GetConstraintsResults{cons}, err
}

// SetConstraints sets the constraints for a given service.
func (api *API) SetConstraints(args params.SetConstraints) error {
	if err := api.check.ChangeAllowed(); err != nil {
		return errors.Trace(err)
	}
	svc, err := api.state.Application(args.ApplicationName)
	if err != nil {
		return err
	}
	return svc.SetConstraints(args.Constraints)
}

// AddRelation adds a relation between the specified endpoints and returns the relation info.
func (api *API) AddRelation(args params.AddRelation) (params.AddRelationResults, error) {
	if err := api.check.ChangeAllowed(); err != nil {
		return params.AddRelationResults{}, errors.Trace(err)
	}
	inEps, err := api.state.InferEndpoints(args.Endpoints...)
	if err != nil {
		return params.AddRelationResults{}, err
	}
	rel, err := api.state.AddRelation(inEps...)
	if err != nil {
		return params.AddRelationResults{}, err
	}
	outEps := make(map[string]charm.Relation)
	for _, inEp := range inEps {
		outEp, err := rel.Endpoint(inEp.ApplicationName)
		if err != nil {
			return params.AddRelationResults{}, err
		}
		outEps[inEp.ApplicationName] = outEp.Relation
	}
	return params.AddRelationResults{Endpoints: outEps}, nil
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
