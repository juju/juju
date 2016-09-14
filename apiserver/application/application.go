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
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/instance"
	jjj "github.com/juju/juju/juju"
	"github.com/juju/juju/permission"
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
	authorizer facade.Authorizer
}

// NewAPI returns a new application API facade.
func NewAPI(
	st *state.State,
	resources facade.Resources,
	authorizer facade.Authorizer,
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

func (api *API) checkCanRead() error {
	canRead, err := api.authorizer.HasPermission(permission.ReadAccess, api.state.ModelTag())
	if err != nil {
		return errors.Trace(err)
	}
	if !canRead {
		return common.ErrPerm
	}
	return nil
}

func (api *API) checkCanWrite() error {
	canWrite, err := api.authorizer.HasPermission(permission.WriteAccess, api.state.ModelTag())
	if err != nil {
		return errors.Trace(err)
	}
	if !canWrite {
		return common.ErrPerm
	}
	return nil
}

// SetMetricCredentials sets credentials on the application.
func (api *API) SetMetricCredentials(args params.ApplicationMetricCredentials) (params.ErrorResults, error) {
	if err := api.checkCanWrite(); err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Creds)),
	}
	if len(args.Creds) == 0 {
		return result, nil
	}
	for i, a := range args.Creds {
		application, err := api.state.Application(a.ApplicationName)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		err = application.SetMetricCredentials(a.MetricCredentials)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
		}
	}
	return result, nil
}

// Deploy fetches the charms from the charm store and deploys them
// using the specified placement directives.
func (api *API) Deploy(args params.ApplicationsDeploy) (params.ErrorResults, error) {
	if err := api.checkCanWrite(); err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Applications)),
	}
	if err := api.check.ChangeAllowed(); err != nil {
		return result, errors.Trace(err)
	}
	for i, arg := range args.Applications {
		err := deployApplication(api.state, arg)
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// deployApplication fetches the charm from the charm store and deploys it.
// The logic has been factored out into a common function which is called by
// both the legacy API on the client facade, as well as the new application facade.
func deployApplication(st *state.State, args params.ApplicationDeploy) error {
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

	_, err = jjj.DeployApplication(st,
		jjj.DeployApplicationParams{
			ApplicationName:  args.ApplicationName,
			Series:           args.Series,
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

// ApplicationSetSettingsStrings updates the settings for the given application,
// taking the configuration from a map of strings.
func ApplicationSetSettingsStrings(application *state.Application, settings map[string]string) error {
	ch, _, err := application.Charm()
	if err != nil {
		return errors.Trace(err)
	}
	// Parse config in a compatible way (see function comment).
	changes, err := parseSettingsCompatible(ch, settings)
	if err != nil {
		return errors.Trace(err)
	}
	return application.UpdateConfigSettings(changes)
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
		return nil, errors.Trace(err)
	}
	// Validate the unsettings and merge them into the changes.
	unsetSettings, err = ch.Config().ValidateSettings(unsetSettings)
	if err != nil {
		return nil, errors.Trace(err)
	}
	for name := range unsetSettings {
		changes[name] = nil
	}
	return changes, nil
}

// Update updates the application attributes, including charm URL,
// minimum number of units, settings and constraints.
// All parameters in params.ApplicationUpdate except the application name are optional.
func (api *API) Update(args params.ApplicationUpdate) error {
	if err := api.checkCanWrite(); err != nil {
		return err
	}
	if !args.ForceCharmUrl {
		if err := api.check.ChangeAllowed(); err != nil {
			return errors.Trace(err)
		}
	}
	svc, err := api.state.Application(args.ApplicationName)
	if err != nil {
		return errors.Trace(err)
	}
	// Set the charm for the given application.
	if args.CharmUrl != "" {
		// For now we do not support changing the channel through Update().
		// TODO(ericsnow) Support it?
		channel := svc.Channel()
		if err = api.applicationSetCharm(svc, args.CharmUrl, channel, args.ForceSeries, args.ForceCharmUrl, nil); err != nil {
			return errors.Trace(err)
		}
	}
	// Set the minimum number of units for the given application.
	if args.MinUnits != nil {
		if err = svc.SetMinUnits(*args.MinUnits); err != nil {
			return errors.Trace(err)
		}
	}
	// Set up application's settings.
	if args.SettingsYAML != "" {
		if err = applicationSetSettingsYAML(svc, args.SettingsYAML); err != nil {
			return errors.Annotate(err, "setting configuration from YAML")
		}
	} else if len(args.SettingsStrings) > 0 {
		if err = ApplicationSetSettingsStrings(svc, args.SettingsStrings); err != nil {
			return errors.Trace(err)
		}
	}
	// Update application's constraints.
	if args.Constraints != nil {
		return svc.SetConstraints(*args.Constraints)
	}
	return nil
}

// SetCharm sets the charm for a given for the application.
func (api *API) SetCharm(args params.ApplicationSetCharm) error {
	if err := api.checkCanWrite(); err != nil {
		return err
	}
	// when forced units in error, don't block
	if !args.ForceUnits {
		if err := api.check.ChangeAllowed(); err != nil {
			return errors.Trace(err)
		}
	}
	application, err := api.state.Application(args.ApplicationName)
	if err != nil {
		return errors.Trace(err)
	}
	channel := csparams.Channel(args.Channel)
	return api.applicationSetCharm(application, args.CharmUrl, channel, args.ForceSeries, args.ForceUnits, args.ResourceIDs)
}

// applicationSetCharm sets the charm for the given for the application.
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
// charm.Settings from it that can then be sent to the application.
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

// applicationSetSettingsYAML updates the settings for the given application,
// taking the configuration from a YAML string.
func applicationSetSettingsYAML(application *state.Application, settings string) error {
	b := []byte(settings)
	var all map[string]interface{}
	if err := goyaml.Unmarshal(b, &all); err != nil {
		return errors.Annotate(err, "parsing settings data")
	}
	// The file is already in the right format.
	if _, ok := all[application.Name()]; !ok {
		changes, err := settingsFromGetYaml(all)
		if err != nil {
			return errors.Annotate(err, "processing YAML generated by get")
		}
		return errors.Annotate(application.UpdateConfigSettings(changes), "updating settings with application YAML")
	}

	ch, _, err := application.Charm()
	if err != nil {
		return errors.Annotate(err, "obtaining charm for this application")
	}

	changes, err := ch.Config().ParseSettingsYAML(b, application.Name())
	if err != nil {
		return errors.Annotate(err, "creating config from YAML")
	}
	return errors.Annotate(application.UpdateConfigSettings(changes), "updating settings")
}

// GetCharmURL returns the charm URL the given application is
// running at present.
func (api *API) GetCharmURL(args params.ApplicationGet) (params.StringResult, error) {
	if err := api.checkCanWrite(); err != nil {
		return params.StringResult{}, errors.Trace(err)
	}
	application, err := api.state.Application(args.ApplicationName)
	if err != nil {
		return params.StringResult{}, errors.Trace(err)
	}
	charmURL, _ := application.CharmURL()
	return params.StringResult{Result: charmURL.String()}, nil
}

// Set implements the server side of Application.Set.
// It does not unset values that are set to an empty string.
// Unset should be used for that.
func (api *API) Set(p params.ApplicationSet) error {
	if err := api.checkCanWrite(); err != nil {
		return err
	}
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
	if err := api.checkCanWrite(); err != nil {
		return err
	}
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

// CharmRelations implements the server side of Application.CharmRelations.
func (api *API) CharmRelations(p params.ApplicationCharmRelations) (params.ApplicationCharmRelationsResults, error) {
	var results params.ApplicationCharmRelationsResults
	if err := api.checkCanRead(); err != nil {
		return results, errors.Trace(err)
	}

	application, err := api.state.Application(p.ApplicationName)
	if err != nil {
		return results, errors.Trace(err)
	}
	endpoints, err := application.Endpoints()
	if err != nil {
		return results, errors.Trace(err)
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
	if err := api.checkCanWrite(); err != nil {
		return err
	}
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
	if err := api.checkCanWrite(); err != nil {
		return err
	}
	if err := api.check.ChangeAllowed(); err != nil {
		return errors.Trace(err)
	}
	svc, err := api.state.Application(args.ApplicationName)
	if err != nil {
		return err
	}
	return svc.ClearExposed()
}

// addApplicationUnits adds a given number of units to an application.
func addApplicationUnits(st *state.State, args params.AddApplicationUnits) ([]*state.Unit, error) {
	application, err := st.Application(args.ApplicationName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if args.NumUnits < 1 {
		return nil, errors.New("must add at least one unit")
	}
	return jjj.AddUnits(st, application, args.NumUnits, args.Placement)
}

// AddUnits adds a given number of units to an application.
func (api *API) AddUnits(args params.AddApplicationUnits) (params.AddApplicationUnitsResults, error) {
	if err := api.checkCanWrite(); err != nil {
		return params.AddApplicationUnitsResults{}, errors.Trace(err)
	}
	if err := api.check.ChangeAllowed(); err != nil {
		return params.AddApplicationUnitsResults{}, errors.Trace(err)
	}
	units, err := addApplicationUnits(api.state, args)
	if err != nil {
		return params.AddApplicationUnitsResults{}, errors.Trace(err)
	}
	unitNames := make([]string, len(units))
	for i, unit := range units {
		unitNames[i] = unit.String()
	}
	return params.AddApplicationUnitsResults{Units: unitNames}, nil
}

// DestroyUnits removes a given set of application units.
func (api *API) DestroyUnits(args params.DestroyApplicationUnits) error {
	if err := api.checkCanWrite(); err != nil {
		return err
	}
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

// Destroy destroys a given application.
func (api *API) Destroy(args params.ApplicationDestroy) error {
	if err := api.checkCanWrite(); err != nil {
		return err
	}
	if err := api.check.RemoveAllowed(); err != nil {
		return errors.Trace(err)
	}
	svc, err := api.state.Application(args.ApplicationName)
	if err != nil {
		return err
	}
	return svc.Destroy()
}

// GetConstraints returns the constraints for a given application.
func (api *API) GetConstraints(args params.GetApplicationConstraints) (params.GetConstraintsResults, error) {
	if err := api.checkCanRead(); err != nil {
		return params.GetConstraintsResults{}, errors.Trace(err)
	}
	svc, err := api.state.Application(args.ApplicationName)
	if err != nil {
		return params.GetConstraintsResults{}, errors.Trace(err)
	}
	cons, err := svc.Constraints()
	return params.GetConstraintsResults{cons}, errors.Trace(err)
}

// SetConstraints sets the constraints for a given application.
func (api *API) SetConstraints(args params.SetConstraints) error {
	if err := api.checkCanWrite(); err != nil {
		return err
	}
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
	if err := api.checkCanWrite(); err != nil {
		return params.AddRelationResults{}, errors.Trace(err)
	}
	if err := api.check.ChangeAllowed(); err != nil {
		return params.AddRelationResults{}, errors.Trace(err)
	}
	inEps, err := api.state.InferEndpoints(args.Endpoints...)
	if err != nil {
		return params.AddRelationResults{}, errors.Trace(err)
	}
	rel, err := api.state.AddRelation(inEps...)
	if err != nil {
		return params.AddRelationResults{}, errors.Trace(err)
	}
	outEps := make(map[string]params.CharmRelation)
	for _, inEp := range inEps {
		outEp, err := rel.Endpoint(inEp.ApplicationName)
		if err != nil {
			return params.AddRelationResults{}, errors.Trace(err)
		}
		outEps[inEp.ApplicationName] = params.CharmRelation{
			Name:      outEp.Relation.Name,
			Role:      string(outEp.Relation.Role),
			Interface: outEp.Relation.Interface,
			Optional:  outEp.Relation.Optional,
			Limit:     outEp.Relation.Limit,
			Scope:     string(outEp.Relation.Scope),
		}
	}
	return params.AddRelationResults{Endpoints: outEps}, nil
}

// DestroyRelation removes the relation between the specified endpoints.
func (api *API) DestroyRelation(args params.DestroyRelation) error {
	if err := api.checkCanWrite(); err != nil {
		return err
	}
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
