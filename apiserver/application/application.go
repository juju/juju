// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package application contains api calls for functionality
// related to deploying and managing applications and their
// related charms.
package application

import (
	"regexp"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/featureflag"
	"gopkg.in/juju/charm.v6-unstable"
	csparams "gopkg.in/juju/charmrepo.v2-unstable/csclient/params"
	"gopkg.in/juju/names.v2"
	goyaml "gopkg.in/yaml.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/crossmodel"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	jujucrossmodel "github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/instance"
	jjj "github.com/juju/juju/juju"
	"github.com/juju/juju/permission"
	"github.com/juju/juju/state"
)

var logger = loggo.GetLogger("juju.apiserver.application")

func init() {
	// TODO - version 1 is required for the legacy deployer,
	// remove when deploy is updated.
	common.RegisterStandardFacade("Application", 1, newAPI)

	common.RegisterStandardFacade("Application", 2, newAPI)

	// Version 3 adds support for cross model relations.
	common.RegisterStandardFacade("Application", 3, newAPI)
}

// API implements the application interface and is the concrete
// implementation of the api end point.
type API struct {
	backend                     Backend
	applicationOffersAPIFactory crossmodel.ApplicationOffersAPIFactory
	authorizer                  facade.Authorizer
	check                       BlockChecker
	dataDir                     string

	// TODO(axw) stateCharm only exists because I ran out
	// of time unwinding all of the tendrils of state. We
	// should pass a charm.Charm and charm.URL back into
	// state wherever we pass in a state.Charm currently.
	stateCharm func(Charm) *state.Charm
}

func newAPI(
	st *state.State,
	resources facade.Resources,
	authorizer facade.Authorizer,
) (*API, error) {
	backend := NewStateBackend(st)
	blockChecker := common.NewBlockChecker(st)
	stateCharm := CharmToStateCharm
	return NewAPI(
		backend,
		authorizer,
		resources,
		blockChecker,
		stateCharm,
	)
}

// NewAPI returns a new application API facade.
func NewAPI(
	backend Backend,
	authorizer facade.Authorizer,
	resources facade.Resources,
	blockChecker BlockChecker,
	stateCharm func(Charm) *state.Charm,
) (*API, error) {
	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}
	apiFactory := resources.Get("applicationOffersApiFactory").(crossmodel.ApplicationOffersAPIFactory)
	dataDir := resources.Get("dataDir").(common.StringResource)
	return &API{
		backend:                     backend,
		authorizer:                  authorizer,
		applicationOffersAPIFactory: apiFactory,
		check:      blockChecker,
		stateCharm: stateCharm,
		dataDir:    dataDir.String(),
	}, nil
}

func (api *API) checkCanRead() error {
	canRead, err := api.authorizer.HasPermission(permission.ReadAccess, api.backend.ModelTag())
	if err != nil {
		return errors.Trace(err)
	}
	if !canRead {
		return common.ErrPerm
	}
	return nil
}

func (api *API) checkCanWrite() error {
	canWrite, err := api.authorizer.HasPermission(permission.WriteAccess, api.backend.ModelTag())
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
		application, err := api.backend.Application(a.ApplicationName)
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
		err := deployApplication(api.backend, api.stateCharm, arg)
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// deployApplication fetches the charm from the charm store and deploys it.
// The logic has been factored out into a common function which is called by
// both the legacy API on the client facade, as well as the new application facade.
func deployApplication(
	backend Backend,
	stateCharm func(Charm) *state.Charm,
	args params.ApplicationDeploy,
) error {
	curl, err := charm.ParseURL(args.CharmURL)
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
		_, err = backend.Machine(p.Directive)
		if err != nil {
			return errors.Annotatef(err, `cannot deploy "%v" to machine %v`, args.ApplicationName, p.Directive)
		}
	}

	// Try to find the charm URL in state first.
	ch, err := backend.Charm(curl)
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
		settings, err = parseSettingsCompatible(ch.Config(), args.Config)
	}
	if err != nil {
		return errors.Trace(err)
	}

	channel := csparams.Channel(args.Channel)

	_, err = jjj.DeployApplication(backend,
		jjj.DeployApplicationParams{
			ApplicationName:  args.ApplicationName,
			Series:           args.Series,
			Charm:            stateCharm(ch),
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
func ApplicationSetSettingsStrings(application Application, settings map[string]string) error {
	ch, _, err := application.Charm()
	if err != nil {
		return errors.Trace(err)
	}
	// Parse config in a compatible way (see function comment).
	changes, err := parseSettingsCompatible(ch.Config(), settings)
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
func parseSettingsCompatible(charmConfig *charm.Config, settings map[string]string) (charm.Settings, error) {
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
	changes, err := charmConfig.ParseSettingsStrings(setSettings)
	if err != nil {
		return nil, errors.Trace(err)
	}
	// Validate the unsettings and merge them into the changes.
	unsetSettings, err = charmConfig.ValidateSettings(unsetSettings)
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
	if !args.ForceCharmURL {
		if err := api.check.ChangeAllowed(); err != nil {
			return errors.Trace(err)
		}
	}
	app, err := api.backend.Application(args.ApplicationName)
	if err != nil {
		return errors.Trace(err)
	}
	// Set the charm for the given application.
	if args.CharmURL != "" {
		// For now we do not support changing the channel through Update().
		// TODO(ericsnow) Support it?
		channel := app.Channel()
		if err = api.applicationSetCharm(
			args.ApplicationName,
			app,
			args.CharmURL,
			channel,
			nil, // charm settings (strings map)
			"",  // charm settings (YAML)
			args.ForceSeries,
			args.ForceCharmURL,
			nil, // resource IDs
			nil, // storage constraints
		); err != nil {
			return errors.Trace(err)
		}
	}
	// Set the minimum number of units for the given application.
	if args.MinUnits != nil {
		if err = app.SetMinUnits(*args.MinUnits); err != nil {
			return errors.Trace(err)
		}
	}
	// Set up application's settings.
	if args.SettingsYAML != "" {
		if err = applicationSetSettingsYAML(args.ApplicationName, app, args.SettingsYAML); err != nil {
			return errors.Annotate(err, "setting configuration from YAML")
		}
	} else if len(args.SettingsStrings) > 0 {
		if err = ApplicationSetSettingsStrings(app, args.SettingsStrings); err != nil {
			return errors.Trace(err)
		}
	}
	// Update application's constraints.
	if args.Constraints != nil {
		return app.SetConstraints(*args.Constraints)
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
	application, err := api.backend.Application(args.ApplicationName)
	if err != nil {
		return errors.Trace(err)
	}
	channel := csparams.Channel(args.Channel)
	return api.applicationSetCharm(
		args.ApplicationName,
		application,
		args.CharmURL,
		channel,
		args.ConfigSettings,
		args.ConfigSettingsYAML,
		args.ForceSeries,
		args.ForceUnits,
		args.ResourceIDs,
		args.StorageConstraints,
	)
}

// applicationSetCharm sets the charm for the given for the application.
func (api *API) applicationSetCharm(
	appName string,
	application Application,
	url string,
	channel csparams.Channel,
	configSettingsStrings map[string]string,
	configSettingsYAML string,
	forceSeries,
	forceUnits bool,
	resourceIDs map[string]string,
	storageConstraints map[string]params.StorageConstraints,
) error {
	curl, err := charm.ParseURL(url)
	if err != nil {
		return errors.Trace(err)
	}
	sch, err := api.backend.Charm(curl)
	if err != nil {
		return errors.Trace(err)
	}
	var settings charm.Settings
	if configSettingsYAML != "" {
		settings, err = sch.Config().ParseSettingsYAML([]byte(configSettingsYAML), appName)
	} else if len(configSettingsStrings) > 0 {
		settings, err = parseSettingsCompatible(sch.Config(), configSettingsStrings)
	}
	if err != nil {
		return errors.Annotate(err, "parsing config settings")
	}
	var stateStorageConstraints map[string]state.StorageConstraints
	if len(storageConstraints) > 0 {
		stateStorageConstraints = make(map[string]state.StorageConstraints)
		for name, cons := range storageConstraints {
			stateCons := state.StorageConstraints{Pool: cons.Pool}
			if cons.Size != nil {
				stateCons.Size = *cons.Size
			}
			if cons.Count != nil {
				stateCons.Count = *cons.Count
			}
			stateStorageConstraints[name] = stateCons
		}
	}
	cfg := state.SetCharmConfig{
		Charm:              api.stateCharm(sch),
		Channel:            channel,
		ConfigSettings:     settings,
		ForceSeries:        forceSeries,
		ForceUnits:         forceUnits,
		ResourceIDs:        resourceIDs,
		StorageConstraints: stateStorageConstraints,
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
func applicationSetSettingsYAML(appName string, application Application, settings string) error {
	b := []byte(settings)
	var all map[string]interface{}
	if err := goyaml.Unmarshal(b, &all); err != nil {
		return errors.Annotate(err, "parsing settings data")
	}
	// The file is already in the right format.
	if _, ok := all[appName]; !ok {
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

	changes, err := ch.Config().ParseSettingsYAML(b, appName)
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
	application, err := api.backend.Application(args.ApplicationName)
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
	app, err := api.backend.Application(p.ApplicationName)
	if err != nil {
		return err
	}
	ch, _, err := app.Charm()
	if err != nil {
		return err
	}
	// Validate the settings.
	changes, err := ch.Config().ParseSettingsStrings(p.Options)
	if err != nil {
		return err
	}

	return app.UpdateConfigSettings(changes)

}

// Unset implements the server side of Client.Unset.
func (api *API) Unset(p params.ApplicationUnset) error {
	if err := api.checkCanWrite(); err != nil {
		return err
	}
	if err := api.check.ChangeAllowed(); err != nil {
		return errors.Trace(err)
	}
	app, err := api.backend.Application(p.ApplicationName)
	if err != nil {
		return err
	}
	settings := make(charm.Settings)
	for _, option := range p.Options {
		settings[option] = nil
	}
	return app.UpdateConfigSettings(settings)
}

// CharmRelations implements the server side of Application.CharmRelations.
func (api *API) CharmRelations(p params.ApplicationCharmRelations) (params.ApplicationCharmRelationsResults, error) {
	var results params.ApplicationCharmRelationsResults
	if err := api.checkCanRead(); err != nil {
		return results, errors.Trace(err)
	}

	application, err := api.backend.Application(p.ApplicationName)
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
	app, err := api.backend.Application(args.ApplicationName)
	if err != nil {
		return err
	}
	return app.SetExposed()
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
	app, err := api.backend.Application(args.ApplicationName)
	if err != nil {
		return err
	}
	return app.ClearExposed()
}

// addApplicationUnits adds a given number of units to an application.
func addApplicationUnits(backend Backend, args params.AddApplicationUnits) ([]*state.Unit, error) {
	application, err := backend.Application(args.ApplicationName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if args.NumUnits < 1 {
		return nil, errors.New("must add at least one unit")
	}
	return jjj.AddUnits(backend, application, args.ApplicationName, args.NumUnits, args.Placement)
}

// AddUnits adds a given number of units to an application.
func (api *API) AddUnits(args params.AddApplicationUnits) (params.AddApplicationUnitsResults, error) {
	if err := api.checkCanWrite(); err != nil {
		return params.AddApplicationUnitsResults{}, errors.Trace(err)
	}
	if err := api.check.ChangeAllowed(); err != nil {
		return params.AddApplicationUnitsResults{}, errors.Trace(err)
	}
	units, err := addApplicationUnits(api.backend, args)
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
		unit, err := api.backend.Unit(name)
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

type appDestroy interface {
	Destroy() (err error)
}

// Destroy destroys a given application, local or remote.
func (api *API) Destroy(args params.ApplicationDestroy) error {
	if err := api.checkCanWrite(); err != nil {
		return err
	}
	if err := api.check.RemoveAllowed(); err != nil {
		return errors.Trace(err)
	}
	var (
		app appDestroy
		err error
	)
	app, err = api.backend.RemoteApplication(args.ApplicationName)
	if errors.IsNotFound(err) {
		app, err = api.backend.Application(args.ApplicationName)
	}
	if err != nil {
		return err
	}
	return app.Destroy()
}

// GetConstraints returns the constraints for a given application.
func (api *API) GetConstraints(args params.GetApplicationConstraints) (params.GetConstraintsResults, error) {
	if err := api.checkCanRead(); err != nil {
		return params.GetConstraintsResults{}, errors.Trace(err)
	}
	app, err := api.backend.Application(args.ApplicationName)
	if err != nil {
		return params.GetConstraintsResults{}, errors.Trace(err)
	}
	cons, err := app.Constraints()
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
	app, err := api.backend.Application(args.ApplicationName)
	if err != nil {
		return err
	}
	return app.SetConstraints(args.Constraints)
}

// applicationUrlEndpointParse is used to split an application url and optional
// relation name into url and relation name.
var applicationUrlEndpointParse = regexp.MustCompile("(?P<url>.*[/.][^:]*)(:(?P<relname>.*)$)?")

// AddRelation adds a relation between the specified endpoints and returns the relation info.
func (api *API) AddRelation(args params.AddRelation) (params.AddRelationResults, error) {
	if err := api.checkCanWrite(); err != nil {
		return params.AddRelationResults{}, errors.Trace(err)
	}
	if err := api.check.ChangeAllowed(); err != nil {
		return params.AddRelationResults{}, errors.Trace(err)
	}

	endpoints := make([]string, len(args.Endpoints))
	// We may have a remote application passed in as the endpoint spec.
	// We'll iterate the endpoints to check.
	for i, ep := range args.Endpoints {
		endpoints[i] = ep

		// If cross model relations not enabled, ignore remote endpoints.
		if !featureflag.Enabled(feature.CrossModelRelations) {
			continue
		}

		// If the endpoint is not remote, skip it.
		// We first need to strip off any relation name
		// which may have been appended to the URL, then
		// we try parsing the URL.
		possibleURL := applicationUrlEndpointParse.ReplaceAllString(ep, "$url")
		relName := applicationUrlEndpointParse.ReplaceAllString(ep, "$relname")

		// If the URL parses, we need to look up the remote application
		// details and save to state.
		url, err := jujucrossmodel.ParseApplicationURL(possibleURL)
		if err != nil {
			// Not a URL.
			continue
		}
		// Save the remote application details into state.
		// TODO(wallyworld) - allow app name to be aliased
		remoteApp, err := api.processRemoteApplication(*url, url.ApplicationName)
		if err != nil {
			return params.AddRelationResults{}, errors.Trace(err)
		}
		// The endpoint is named after the remote application name,
		// not the application name from the URL.
		endpoints[i] = remoteApp.Name()
		if relName != "" {
			endpoints[i] = remoteApp.Name() + ":" + relName
		}
	}

	inEps, err := api.backend.InferEndpoints(endpoints...)
	if err != nil {
		return params.AddRelationResults{}, errors.Trace(err)
	}
	rel, err := api.backend.AddRelation(inEps...)
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

// processRemoteApplication takes a remote application URL and retrieves or confirms the the details
// of the application and endpoint. These details are saved to the state model so relations to
// the remote application can be created.
func (api *API) processRemoteApplication(url jujucrossmodel.ApplicationURL, alias string) (*state.RemoteApplication, error) {
	// The application URL is either for an application in another model on this controller,
	// or is for an application offer contained in a directory.
	if url.Directory == "" {
		if url.ModelName == "" {
			return nil, errors.Errorf("missing model name in URL %q", url.String())
		}
		return api.processSameControllerRemoteApplication(url, alias)
	}

	offersAPI, err := api.applicationOffersAPIFactory.ApplicationOffers(url.Directory)
	if err != nil {
		return nil, errors.Trace(err)
	}
	offers, err := offersAPI.ListOffers(params.OfferFilters{
		Directory: url.Directory,
		Filters: []params.OfferFilter{
			{
				ApplicationURL: url.String(),
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
		return nil, errors.NotFoundf("application offer %q", url.String())
	}

	// Create a remote application entry in the model for the consumed service.
	offer := offers.Offers[0]
	sourceModelTag, err := names.ParseModelTag(offer.SourceModelTag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return api.saveRemoteApplication(sourceModelTag, url.ApplicationName, url.ApplicationName, url.String(), offer.Endpoints)
}

func (api *API) sameControllerSourceModel(userName string, url jujucrossmodel.ApplicationURL) (names.ModelTag, error) {
	// Look up the model by qualified name, ie user/model.
	var sourceModelTag names.ModelTag
	allModels, err := api.backend.AllModels()
	if err != nil {
		return sourceModelTag, errors.Trace(err)
	}
	for _, m := range allModels {
		if m.Name() != url.ModelName {
			continue
		}
		if m.Owner().Name() != userName {
			continue
		}
		sourceModelTag = m.Tag().(names.ModelTag)
	}
	if sourceModelTag.Id() == "" {
		return sourceModelTag, errors.NotFoundf(`model "%s/%s"`, userName, url.ModelName)
	}
	return sourceModelTag, nil
}

// processSameControllerRemoteApplication handles the case where we have an application
// from another model on the same controller.
func (api *API) processSameControllerRemoteApplication(url jujucrossmodel.ApplicationURL, alias string) (*state.RemoteApplication, error) {
	// The user name is either specified in URL, or else we default to
	// the logged in user.
	userName := url.User
	if userName == "" {
		userName = api.authorizer.GetAuthTag().Id()
	}
	// To relate to an application in another model, the user needs at least write permission.
	sourceModelTag, err := api.sameControllerSourceModel(userName, url)
	if err != nil {
		return nil, errors.Trace(err)
	}
	ok, err := api.authorizer.HasPermission(permission.WriteAccess, sourceModelTag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if !ok {
		return nil, common.ErrPerm
	}
	// Get the backend state for the source model so we can lookup the application
	// and its endpoints.
	st, err := api.backend.ForModel(sourceModelTag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer st.Close()
	application, err := st.Application(url.ApplicationName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	eps, err := application.Endpoints()
	if err != nil {
		return nil, errors.Trace(err)
	}
	endpoints := make([]params.RemoteEndpoint, len(eps))
	for i, ep := range eps {
		endpoints[i] = params.RemoteEndpoint{
			Name:      ep.Name,
			Scope:     ep.Scope,
			Interface: ep.Interface,
			Role:      ep.Role,
			Limit:     ep.Limit,
		}
	}
	appName := alias
	if appName == "" {
		appName = url.ApplicationName
	}
	return api.saveRemoteApplication(sourceModelTag, appName, url.ApplicationName, url.String(), endpoints)
}

// saveRemoteApplication saves the details of the specified remote application and its endpoints
// to the state model so relations to the remote application can be created.
func (api *API) saveRemoteApplication(
	sourceModelTag names.ModelTag, applicationName, offerName, url string, endpoints []params.RemoteEndpoint,
) (*state.RemoteApplication, error) {
	remoteEps := make([]charm.Relation, len(endpoints))
	for j, ep := range endpoints {
		remoteEps[j] = charm.Relation{
			Name:      ep.Name,
			Role:      ep.Role,
			Interface: ep.Interface,
			Limit:     ep.Limit,
			Scope:     ep.Scope,
		}
	}
	remoteApp, err := api.backend.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name:        applicationName,
		OfferName:   offerName,
		URL:         url,
		SourceModel: sourceModelTag,
		Endpoints:   remoteEps,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return remoteApp, nil
}

// RemoteApplicationInfo returns information about the requested remote application.
func (api *API) RemoteApplicationInfo(args params.ApplicationURLs) (params.RemoteApplicationInfoResults, error) {
	if err := api.checkCanRead(); err != nil {
		return params.RemoteApplicationInfoResults{}, errors.Trace(err)
	}
	results := make([]params.RemoteApplicationInfoResult, len(args.ApplicationURLs))
	for i, url := range args.ApplicationURLs {
		info, err := api.oneRemoteApplicationInfo(url)
		results[i].Result = info
		results[i].Error = common.ServerError(err)
	}
	return params.RemoteApplicationInfoResults{results}, nil
}

func (api *API) oneRemoteApplicationInfo(urlStr string) (*params.RemoteApplicationInfo, error) {
	url, err := jujucrossmodel.ParseApplicationURL(urlStr)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if url.Directory == "" {
		if url.ModelName == "" {
			return nil, errors.Errorf("missing model name in URL %q", url.String())
		}
		return api.sameControllerRemoteApplicationInfo(*url)
	}

	offersAPI, err := api.applicationOffersAPIFactory.ApplicationOffers(url.Directory)
	if err != nil {
		return nil, errors.Trace(err)
	}
	offers, err := offersAPI.ListOffers(params.OfferFilters{
		Directory: url.Directory,
		Filters: []params.OfferFilter{
			{
				ApplicationURL: url.String(),
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
		return nil, errors.NotFoundf("application offer %q", url.String())
	}
	offer := offers.Offers[0]
	return &params.RemoteApplicationInfo{
		ModelTag:         offer.SourceModelTag,
		Name:             offer.ApplicationName,
		Description:      offer.ApplicationDescription,
		ApplicationURL:   urlStr,
		SourceModelLabel: offer.SourceLabel,
		Endpoints:        offer.Endpoints,
		// TODO(wallyworld)
		Icon: []byte(common.DefaultCharmIcon),
	}, nil
}

func (api *API) sameControllerRemoteApplicationInfo(url jujucrossmodel.ApplicationURL) (*params.RemoteApplicationInfo, error) {
	// The user name is either specified in URL, or else we default to
	// the logged in user.
	userName := url.User
	if userName == "" {
		userName = api.authorizer.GetAuthTag().Id()
	}
	sourceModelTag, err := api.sameControllerSourceModel(userName, url)
	if err != nil {
		return nil, errors.Trace(err)
	}
	// We need at least read access to the model to see the application details.
	ok, err := api.authorizer.HasPermission(permission.ReadAccess, sourceModelTag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if !ok {
		return nil, common.ErrPerm
	}
	// Get the backend state for the source model so we can lookup the application
	// and its endpoints.
	st, err := api.backend.ForModel(sourceModelTag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer st.Close()
	application, err := st.Application(url.ApplicationName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	eps, err := application.Endpoints()
	if err != nil {
		return nil, errors.Trace(err)
	}
	endpoints := make([]params.RemoteEndpoint, len(eps))
	for i, ep := range eps {
		endpoints[i] = params.RemoteEndpoint{
			Name:      ep.Name,
			Scope:     ep.Scope,
			Interface: ep.Interface,
			Role:      ep.Role,
			Limit:     ep.Limit,
		}
	}
	ch, _, err := application.Charm()
	if err != nil {
		return nil, errors.Trace(err)
	}
	model, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	icon, err := api.charmIcon(NewStateBackend(st), ch.URL())
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &params.RemoteApplicationInfo{
		ModelTag:         sourceModelTag.String(),
		Name:             application.Name(),
		Description:      ch.Meta().Description,
		ApplicationURL:   url.String(),
		SourceModelLabel: model.Name(),
		Endpoints:        endpoints,
		Icon:             icon,
	}, nil
}

func (api *API) charmIcon(backend Backend, curl *charm.URL) ([]byte, error) {
	ch, err := backend.Charm(curl)
	if err != nil {
		return nil, errors.Trace(err)
	}
	charmPath, err := common.ReadCharmFromStorage(backend.NewStorage(), api.dataDir, ch.StoragePath())
	if err != nil {
		return nil, errors.Trace(err)
	}
	return common.CharmArchiveEntry(charmPath, "icon.svg", true)
}

// Consume adds remote applications to the model without creating any
// relations.
func (api *API) Consume(args params.ConsumeApplicationArgs) (params.ConsumeApplicationResults, error) {
	var consumeResults params.ConsumeApplicationResults
	if err := api.checkCanWrite(); err != nil {
		return consumeResults, err
	}
	if err := api.check.ChangeAllowed(); err != nil {
		return consumeResults, errors.Trace(err)
	}
	if !featureflag.Enabled(feature.CrossModelRelations) {
		err := errors.Errorf(
			"set %q feature flag to enable consuming remote applications",
			feature.CrossModelRelations,
		)
		return consumeResults, err
	}
	results := make([]params.ConsumeApplicationResult, len(args.Args))
	for i, arg := range args.Args {
		localName, err := api.consumeOne(arg.ApplicationURL, arg.ApplicationAlias)
		results[i].LocalName = localName
		results[i].Error = common.ServerError(err)
	}
	consumeResults.Results = results
	return consumeResults, nil
}

func (api *API) consumeOne(possibleURL, alias string) (string, error) {
	url, err := jujucrossmodel.ParseApplicationURL(possibleURL)
	if err != nil {
		return "", errors.Trace(err)
	}
	if url.HasEndpoint() {
		return "", errors.Errorf("remote application %q shouldn't include endpoint", url)
	}
	remoteApp, err := api.processRemoteApplication(*url, alias)
	if err != nil {
		return "", errors.Trace(err)
	}
	return remoteApp.Name(), nil
}

// DestroyRelation removes the relation between the specified endpoints.
func (api *API) DestroyRelation(args params.DestroyRelation) error {
	if err := api.checkCanWrite(); err != nil {
		return err
	}
	if err := api.check.RemoveAllowed(); err != nil {
		return errors.Trace(err)
	}
	eps, err := api.backend.InferEndpoints(args.Endpoints...)
	if err != nil {
		return err
	}
	rel, err := api.backend.EndpointsRelation(eps...)
	if err != nil {
		return err
	}
	return rel.Destroy()
}
