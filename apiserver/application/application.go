// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package application contains api calls for functionality
// related to deploying and managing applications and their
// related charms.
package application

import (
	"fmt"
	"regexp"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/featureflag"
	"github.com/juju/utils/set"
	"gopkg.in/juju/charm.v6-unstable"
	csparams "gopkg.in/juju/charmrepo.v2-unstable/csclient/params"
	"gopkg.in/juju/names.v2"
	goyaml "gopkg.in/yaml.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	jujucrossmodel "github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/instance"
	jjj "github.com/juju/juju/juju"
	"github.com/juju/juju/permission"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
)

var logger = loggo.GetLogger("juju.apiserver.application")

// API implements the application interface and is the concrete
// implementation of the api end point.
type API struct {
	backend    Backend
	authorizer facade.Authorizer
	check      BlockChecker
	dataDir    string

	statePool *state.StatePool

	// TODO(axw) stateCharm only exists because I ran out
	// of time unwinding all of the tendrils of state. We
	// should pass a charm.Charm and charm.URL back into
	// state wherever we pass in a state.Charm currently.
	stateCharm func(Charm) *state.Charm

	deployApplicationFunc func(backend Backend, args jjj.DeployApplicationParams) error
	getEnviron            stateenvirons.NewEnvironFunc
}

// DeployApplication is a wrapper around juju.DeployApplication, to
// match the function signature expected by NewAPI.
func DeployApplication(backend Backend, args jjj.DeployApplicationParams) error {
	_, err := jjj.DeployApplication(backend, args)
	return err
}

// NewFacade provides the signature required for facade registration.
func NewFacade(ctx facade.Context) (*API, error) {
	backend := NewStateBackend(ctx.State())
	blockChecker := common.NewBlockChecker(ctx.State())
	stateCharm := CharmToStateCharm
	return NewAPI(
		backend,
		ctx.Auth(),
		ctx.Resources(),
		ctx.StatePool(),
		blockChecker,
		stateCharm,
		DeployApplication,
		stateenvirons.GetNewEnvironFunc(environs.New),
	)
}

// NewAPI returns a new application API facade.
func NewAPI(
	backend Backend,
	authorizer facade.Authorizer,
	resources facade.Resources,
	statePool *state.StatePool,
	blockChecker BlockChecker,
	stateCharm func(Charm) *state.Charm,
	deployApplication func(Backend, jjj.DeployApplicationParams) error,
	getEnviron stateenvirons.NewEnvironFunc,
) (*API, error) {
	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}
	dataDir := resources.Get("dataDir").(common.StringResource)
	return &API{
		backend:               backend,
		authorizer:            authorizer,
		check:                 blockChecker,
		stateCharm:            stateCharm,
		statePool:             statePool,
		dataDir:               dataDir.String(),
		deployApplicationFunc: deployApplication,
		getEnviron:            getEnviron,
	}, nil
}

func (api *API) checkPermission(tag names.Tag, perm permission.Access) error {
	allowed, err := api.authorizer.HasPermission(perm, tag)
	if err != nil {
		return errors.Trace(err)
	}
	if !allowed {
		return common.ErrPerm
	}
	return nil
}

func (api *API) checkCanRead() error {
	return api.checkPermission(api.backend.ModelTag(), permission.ReadAccess)
}

func (api *API) checkCanWrite() error {
	return api.checkPermission(api.backend.ModelTag(), permission.WriteAccess)
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
		err := deployApplication(api.backend, api.stateCharm, arg, api.deployApplicationFunc)
		result.Results[i].Error = common.ServerError(err)

		if err != nil && len(arg.Resources) != 0 {
			// Remove any pending resources - these would have been
			// converted into real resources if the application had
			// been created successfully, but will otherwise be
			// leaked. lp:1705730
			// TODO(babbageclunk): rework the deploy API so the
			// resources are created transactionally to avoid needing
			// to do this.
			resources, err := api.backend.Resources()
			if err != nil {
				logger.Errorf("couldn't get backend.Resources")
				continue
			}
			err = resources.RemovePendingAppResources(arg.ApplicationName, arg.Resources)
			if err != nil {
				logger.Errorf("couldn't remove pending resources for %q", arg.ApplicationName)
			}
		}
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
	deployApplicationFunc func(Backend, jjj.DeployApplicationParams) error,
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

	return errors.Trace(deployApplicationFunc(backend, jjj.DeployApplicationParams{
		ApplicationName:  args.ApplicationName,
		Series:           args.Series,
		Charm:            stateCharm(ch),
		Channel:          csparams.Channel(args.Channel),
		NumUnits:         args.NumUnits,
		ConfigSettings:   settings,
		Constraints:      args.Constraints,
		Placement:        args.Placement,
		Storage:          args.Storage,
		EndpointBindings: args.EndpointBindings,
		Resources:        args.Resources,
	}))
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
//
// NOTE(axw) this exists only for backwards compatibility,
// for API facade versions 1-3; clients should prefer its
// successor, DestroyUnit, below.
//
// TODO(axw) 2017-03-16 #1673323
// Drop this in Juju 3.0.
func (api *API) DestroyUnits(args params.DestroyApplicationUnits) error {
	var errs []error
	entities := params.Entities{
		Entities: make([]params.Entity, 0, len(args.UnitNames)),
	}
	for _, unitName := range args.UnitNames {
		if !names.IsValidUnit(unitName) {
			errs = append(errs, errors.NotValidf("unit name %q", unitName))
			continue
		}
		entities.Entities = append(entities.Entities, params.Entity{
			Tag: names.NewUnitTag(unitName).String(),
		})
	}
	results, err := api.DestroyUnit(entities)
	if err != nil {
		return errors.Trace(err)
	}
	for _, result := range results.Results {
		if result.Error != nil {
			errs = append(errs, result.Error)
		}
	}
	return common.DestroyErr("units", args.UnitNames, errs)
}

// DestroyUnit removes a given set of application units.
func (api *API) DestroyUnit(args params.Entities) (params.DestroyUnitResults, error) {
	if err := api.checkCanWrite(); err != nil {
		return params.DestroyUnitResults{}, err
	}
	if err := api.check.RemoveAllowed(); err != nil {
		return params.DestroyUnitResults{}, errors.Trace(err)
	}
	destroyUnit := func(entity params.Entity) (*params.DestroyUnitInfo, error) {
		unitTag, err := names.ParseUnitTag(entity.Tag)
		if err != nil {
			return nil, err
		}
		name := unitTag.Id()
		unit, err := api.backend.Unit(name)
		if errors.IsNotFound(err) {
			return nil, errors.Errorf("unit %q does not exist", name)
		} else if err != nil {
			return nil, errors.Trace(err)
		}
		if !unit.IsPrincipal() {
			return nil, errors.Errorf("unit %q is a subordinate", name)
		}
		var info params.DestroyUnitInfo
		storage, err := common.UnitStorage(api.backend, unit.UnitTag())
		if err != nil {
			return nil, err
		}
		info.DestroyedStorage, info.DetachedStorage = common.ClassifyDetachedStorage(storage)
		if err := unit.Destroy(); err != nil {
			return nil, err
		}
		return &info, nil
	}
	results := make([]params.DestroyUnitResult, len(args.Entities))
	for i, entity := range args.Entities {
		info, err := destroyUnit(entity)
		if err != nil {
			results[i].Error = common.ServerError(err)
			continue
		}
		results[i].Info = info
	}
	return params.DestroyUnitResults{results}, nil
}

// Destroy destroys a given application, local or remote.
//
// NOTE(axw) this exists only for backwards compatibility,
// for API facade versions 1-3; clients should prefer its
// successor, DestroyApplication, below.
//
// TODO(axw) 2017-03-16 #1673323
// Drop this in Juju 3.0.
func (api *API) Destroy(args params.ApplicationDestroy) error {
	if !names.IsValidApplication(args.ApplicationName) {
		return errors.NotValidf("application name %q", args.ApplicationName)
	}
	entities := params.Entities{
		Entities: []params.Entity{{
			Tag: names.NewApplicationTag(args.ApplicationName).String(),
		}},
	}
	results, err := api.DestroyApplication(entities)
	if err != nil {
		return errors.Trace(err)
	}
	if err := results.Results[0].Error; err != nil {
		return common.ServerError(err)
	}
	return nil
}

// DestroyApplication removes a given set of applications.
func (api *API) DestroyApplication(args params.Entities) (params.DestroyApplicationResults, error) {
	if err := api.checkCanWrite(); err != nil {
		return params.DestroyApplicationResults{}, err
	}
	if err := api.check.RemoveAllowed(); err != nil {
		return params.DestroyApplicationResults{}, errors.Trace(err)
	}
	destroyRemoteApp := func(name string) error {
		app, err := api.backend.RemoteApplication(name)
		if err != nil {
			return err
		}
		return app.Destroy()
	}
	destroyApp := func(entity params.Entity) (*params.DestroyApplicationInfo, error) {
		tag, err := names.ParseApplicationTag(entity.Tag)
		if err != nil {
			return nil, err
		}
		var info params.DestroyApplicationInfo
		if err := destroyRemoteApp(tag.Id()); !errors.IsNotFound(err) {
			return &info, err
		}
		app, err := api.backend.Application(tag.Id())
		if err != nil {
			return nil, err
		}
		units, err := app.AllUnits()
		if err != nil {
			return nil, err
		}
		storageSeen := make(set.Tags)
		for _, unit := range units {
			info.DestroyedUnits = append(
				info.DestroyedUnits,
				params.Entity{unit.UnitTag().String()},
			)
			storage, err := common.UnitStorage(api.backend, unit.UnitTag())
			if err != nil {
				return nil, err
			}

			// Filter out storage we've already seen. Shared
			// storage may be attached to multiple units.
			var unseen []state.StorageInstance
			for _, stor := range storage {
				storageTag := stor.StorageTag()
				if storageSeen.Contains(storageTag) {
					continue
				}
				storageSeen.Add(storageTag)
				unseen = append(unseen, stor)
			}
			storage = unseen

			destroyed, detached := common.ClassifyDetachedStorage(storage)
			info.DestroyedStorage = append(info.DestroyedStorage, destroyed...)
			info.DetachedStorage = append(info.DetachedStorage, detached...)
		}
		if err := app.Destroy(); err != nil {
			return nil, err
		}
		return &info, nil
	}
	results := make([]params.DestroyApplicationResult, len(args.Entities))
	for i, entity := range args.Entities {
		info, err := destroyApp(entity)
		if err != nil {
			results[i].Error = common.ServerError(err)
			continue
		}
		results[i].Info = info
	}
	return params.DestroyApplicationResults{results}, nil
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
	if err := api.check.ChangeAllowed(); err != nil {
		return params.AddRelationResults{}, errors.Trace(err)
	}

	endpoints := make([]string, len(args.Endpoints))
	// We may have a remote application passed in as the endpoint spec.
	// We'll iterate the endpoints to check.
	isRemote := false
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
		alias := url.ApplicationName
		remoteApp, err := api.processRemoteApplication(url, alias)
		if err != nil {
			return params.AddRelationResults{}, errors.Trace(err)
		}
		// The endpoint is named after the remote application name,
		// not the application name from the URL.
		endpoints[i] = remoteApp.Name()
		if relName != "" {
			endpoints[i] = remoteApp.Name() + ":" + relName
		}
		isRemote = true
	}
	// If it's not a remote relation to another model then
	// the user needs write access to the model.
	if !isRemote {
		if err := api.checkCanWrite(); err != nil {
			return params.AddRelationResults{}, errors.Trace(err)
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

func (api *API) sameControllerSourceModel(userName, modelName string) (names.ModelTag, error) {
	// Look up the model by qualified name, ie user/model.
	var sourceModelTag names.ModelTag
	allModels, err := api.backend.AllModels()
	if err != nil {
		return sourceModelTag, errors.Trace(err)
	}
	for _, m := range allModels {
		if m.Name() != modelName {
			continue
		}
		if m.Owner().Name() != userName {
			continue
		}
		sourceModelTag = m.Tag().(names.ModelTag)
	}
	if sourceModelTag.Id() == "" {
		return sourceModelTag, errors.NotFoundf(`model "%s/%s"`, userName, modelName)
	}
	return sourceModelTag, nil
}

// processRemoteApplication takes a remote application URL and retrieves or confirms the the details
// of the application and endpoint. These details are saved to the state model so relations to
// the remote application can be created.
func (api *API) processRemoteApplication(url *jujucrossmodel.ApplicationURL, alias string) (*state.RemoteApplication, error) {
	offer, err := api.offeredApplicationDetails(url, permission.ConsumeAccess)
	if err != nil {
		return nil, errors.Trace(err)
	}

	appName := alias
	if appName == "" {
		appName = url.ApplicationName
	}
	remoteApp, err := api.saveRemoteApplication(url.String(), appName, offer)
	return remoteApp, err
}

// offeredApplicationDetails returns details of the application offered at the specified URL.
// The user is required to have the specified permission on the offer.
func (api *API) offeredApplicationDetails(url *jujucrossmodel.ApplicationURL, perm permission.Access) (
	*params.ApplicationOffer, error,
) {
	// We require the hosting model to be specified.
	if url.ModelName == "" {
		return nil, errors.Errorf("missing model name in URL %q", url.String())
	}

	// The user name is either specified in URL, or else we default to
	// the logged in user.
	userName := url.User
	if userName == "" {
		userName = api.authorizer.GetAuthTag().Id()
	}

	// Get the hosting model from the name.
	sourceModelTag, err := api.sameControllerSourceModel(userName, url.ModelName)
	if err == nil {
		offerParams, err := api.sameControllerOfferedApplication(sourceModelTag, url.ApplicationName, perm)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return offerParams, err
	}
	if errors.IsNotFound(err) {
		return api.differentControllerOfferedApplication(userName, url.ModelName, url.ApplicationName)
	}
	return nil, errors.Trace(err)
}

func (api *API) differentControllerOfferedApplication(userName, modelName, offerName string) (
	*params.ApplicationOffer,
	error,
) {
	// TODO(wallyworld) - we need a way to pass in the JEM api Info
	// For now act as if the offer is not found.
	return nil, errors.NotFoundf("application offer at %s/%s.%s", userName, modelName, offerName)

	//dialOpts := jujuapi.DefaultDialOpts()
	//conn, err := jujuapi.Open(nil, dialOpts)
	//if err != nil {
	//	return fail(errors.Trace(err))
	//}
	//client := remoteendpoints.NewClient(conn)
	//filter := jujucrossmodel.ApplicationOfferFilter{
	//	OwnerName: userName,
	//	ModelName: modelName,
	//	OfferName: offerName,
	//}
	//offers, err := client.FindApplicationOffers(filter)
	//if err != nil {
	//	return fail(errors.Trace(err))
	//}
	//offerPath := fmt.Sprintf("%s/%s.%s", userName, modelName, offerName)
	//if len(offers) == 0 {
	//	return fail(errors.NotFoundf("application offer at %s", offerPath))
	//}
	//if len(offers) != 1 {
	//	return fail(errors.Errorf("unexpected: %d matching offers at %s", len(offers), offerPath))
	//}
	//sourceModelTag, err := names.ParseModelTag(offers[0].SourceModelTag)
	//if err != nil {
	//	return fail(errors.Trace(err))
	//}
	//return &offers[0], sourceModelTag, nil
}

func (api *API) makeOfferParams(st *state.State, offer *jujucrossmodel.ApplicationOffer) (
	*params.ApplicationOffer, error,
) {
	app, err := st.Application(offer.ApplicationName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	appBindings, err := app.EndpointBindings()
	if err != nil {
		return nil, errors.Trace(err)
	}
	result := params.ApplicationOffer{
		OfferName:              offer.OfferName,
		ApplicationDescription: offer.ApplicationDescription,
	}

	spaceNames := set.NewStrings()
	for _, ep := range offer.Endpoints {
		result.Endpoints = append(result.Endpoints, params.RemoteEndpoint{
			Name:      ep.Name,
			Interface: ep.Interface,
			Role:      ep.Role,
			Scope:     ep.Scope,
			Limit:     ep.Limit,
		})
		spaceName, ok := appBindings[ep.Name]
		if !ok {
			// There should always be some binding (even if it's to
			// the default space).
			return nil, errors.Errorf("no binding for %q endpoint", ep.Name)
		}
		spaceNames.Add(spaceName)
	}

	spaces, err := api.collectRemoteSpaces(st, spaceNames.SortedValues())
	if errors.IsNotSupported(err) {
		// Provider doesn't support ProviderSpaceInfo; continue
		// without any space information, we shouldn't short-circuit
		// cross-model connections.
		return &result, nil
	}
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Ensure bindings only contains entries for which we have spaces.
	result.Bindings = make(map[string]string)
	for epName, spaceName := range appBindings {
		space, ok := spaces[spaceName]
		if !ok {
			continue
		}
		result.Bindings[epName] = spaceName
		result.Spaces = append(result.Spaces, space)
	}
	return &result, nil
}

// collectRemoteSpaces gets provider information about the spaces from
// the state passed in. (This state will be for a different model than
// this API instance, which is why the results are *remote* spaces.)
// These can be used by the provider later on to decide whether a
// connection can be made via cloud-local addresses. If the provider
// doesn't support getting ProviderSpaceInfo the NotSupported error
// will be returned.
func (api *API) collectRemoteSpaces(st *state.State, spaceNames []string) (map[string]params.RemoteSpace, error) {
	env, err := api.getEnviron(st)
	if err != nil {
		return nil, errors.Trace(err)
	}
	netEnv, ok := environs.SupportsNetworking(env)
	if !ok {
		logger.Debugf("cloud provider doesn't support networking, not getting space info")
		return nil, nil
	}

	results := make(map[string]params.RemoteSpace)
	for _, name := range spaceNames {
		space := environs.DefaultSpaceInfo
		if name != environs.DefaultSpaceName {
			dbSpace, err := st.Space(name)
			if err != nil {
				return nil, errors.Trace(err)
			}
			space, err = spaceInfoFromState(dbSpace)
			if err != nil {
				return nil, errors.Trace(err)
			}
		}
		providerSpace, err := netEnv.ProviderSpaceInfo(space)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if providerSpace == nil {
			logger.Errorf("nil provider space info for %q", name)
			continue
		}
		remoteSpace := paramsFromProviderSpaceInfo(providerSpace)
		// Use the name from state in case provider and state disagree.
		remoteSpace.Name = name
		results[name] = remoteSpace
	}
	return results, nil
}

func (api *API) sameControllerOfferedApplication(sourceModelTag names.ModelTag, offerName string, perm permission.Access) (
	*params.ApplicationOffer, error,
) {
	// Get the backend state for the source model so we can lookup the application.
	st, releaser, err := api.statePool.Get(sourceModelTag.Id())
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer releaser()

	// For now, offer URL is matched against the specified application
	// name as seen from the consuming model.
	applicationOffers := state.NewApplicationOffers(st)
	offers, err := applicationOffers.ListOffers(
		jujucrossmodel.ApplicationOfferFilter{
			OfferName: offerName,
		},
	)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// The offers query succeeded but there were no offers matching the required offer name.
	if len(offers) == 0 {
		return nil, errors.NotFoundf("application offer %q", offerName)
	}
	// Sanity check - this should never happen.
	if len(offers) > 1 {
		return nil, errors.Errorf("unexpected: %d matching offers for %q", len(offers), offerName)
	}

	// Check the permissions - a user can access the offer if they are an admin
	// or they have consume access to the offer.
	isAdmin := false
	err = api.checkPermission(st.ControllerTag(), permission.SuperuserAccess)
	if err == common.ErrPerm {
		err = api.checkPermission(sourceModelTag, permission.AdminAccess)
	}
	if err != nil && err != common.ErrPerm {
		return nil, errors.Trace(err)
	}
	isAdmin = err == nil

	offer := offers[0]
	if !isAdmin {
		// Check for consume access on tne offer - we can't use api.checkPermission as
		// we need to operate on the state containing the offer.
		apiUser := api.authorizer.GetAuthTag().(names.UserTag)
		access, err := st.GetOfferAccess(names.NewApplicationOfferTag(offer.OfferName), apiUser)
		if err != nil && !errors.IsNotFound(err) {
			return nil, errors.Trace(err)
		}
		if !access.EqualOrGreaterOfferAccessThan(perm) {
			return nil, common.ErrPerm
		}
	}
	offerParams, err := api.makeOfferParams(st, &offer)
	if err != nil {
		return nil, errors.Trace(err)
	}
	offerParams.SourceModelTag = sourceModelTag.String()
	return offerParams, nil
}

// saveRemoteApplication saves the details of the specified remote application and its endpoints
// to the state model so relations to the remote application can be created.
func (api *API) saveRemoteApplication(url, applicationName string, offer *params.ApplicationOffer) (
	*state.RemoteApplication, error,
) {
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

	remoteSpaces := make([]*environs.ProviderSpaceInfo, len(offer.Spaces))
	for i, space := range offer.Spaces {
		remoteSpaces[i] = providerSpaceInfoFromParams(space)
	}

	// If the a remote application with the same name and endpoints from the same
	// source model already exists, we will use that one.
	sourceModelTag, err := names.ParseModelTag(offer.SourceModelTag)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// TODO(babbageclunk): how should we handle changed spaces and/or
	// bindings as well here?
	remoteApp, err := api.maybeUpdateExistingApplicationEndpoints(applicationName, sourceModelTag, remoteEps)
	if err == nil {
		return remoteApp, nil
	} else if !errors.IsNotFound(err) {
		return nil, errors.Trace(err)
	}

	return api.backend.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name:        applicationName,
		OfferName:   offer.OfferName,
		URL:         url,
		SourceModel: sourceModelTag,
		Endpoints:   remoteEps,
		Bindings:    offer.Bindings,
		Spaces:      remoteSpaces,
	})
}

// maybeUpdateExistingApplicationEndpoints looks for a remote application with the
// specified name and source model tag and tries to update its endpoints with the
// new ones specified. If the endpoints are compatible, the newly updated remote
// application is returned.
func (api *API) maybeUpdateExistingApplicationEndpoints(
	applicationName string, sourceModelTag names.ModelTag, remoteEps []charm.Relation,
) (*state.RemoteApplication, error) {
	existingRemoteApp, err := api.backend.RemoteApplication(applicationName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if err != nil && !errors.IsNotFound(err) {
		return nil, errors.Trace(err)
	}
	if existingRemoteApp.SourceModel().Id() != sourceModelTag.Id() {
		return nil, errors.AlreadyExistsf("remote application called %q from a different model", applicationName)
	}
	newEpsMap := make(map[charm.Relation]bool)
	for _, ep := range remoteEps {
		newEpsMap[ep] = true
	}
	existingEps, err := existingRemoteApp.Endpoints()
	if err != nil {
		return nil, errors.Trace(err)
	}
	maybeSameEndpoints := len(newEpsMap) == len(existingEps)
	existingEpsByName := make(map[string]charm.Relation)
	for _, ep := range existingEps {
		existingEpsByName[ep.Name] = ep.Relation
		delete(newEpsMap, ep.Relation)
	}
	sameEndpoints := maybeSameEndpoints && len(newEpsMap) == 0
	if sameEndpoints {
		return existingRemoteApp, nil
	}

	// Gather the new endpoints. All new endpoints passed to AddEndpoints()
	// below must not have the same name as an existing endpoint.
	var newEps []charm.Relation
	for ep := range newEpsMap {
		// See if we are attempting to update endpoints with the same name but
		// different relation data.
		if existing, ok := existingEpsByName[ep.Name]; ok && existing != ep {
			return nil, errors.Errorf("conflicting endpoint %v", ep.Name)
		}
		newEps = append(newEps, ep)
	}

	if len(newEps) > 0 {
		// Update the existing remote app to have the new, additional endpoints.
		if err := existingRemoteApp.AddEndpoints(newEps); err != nil {
			return nil, errors.Trace(err)
		}
	}
	return existingRemoteApp, nil
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

	// We need at least read access to the model to see the application details.
	offer, err := api.offeredApplicationDetails(url, permission.ReadAccess)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return &params.RemoteApplicationInfo{
		ModelTag:         offer.SourceModelTag,
		Name:             url.ApplicationName,
		Description:      offer.ApplicationDescription,
		ApplicationURL:   url.String(),
		SourceModelLabel: url.ModelName,
		Endpoints:        offer.Endpoints,
		IconURLPath:      fmt.Sprintf("rest/1.0/remote-application/%s/icon", url.ApplicationName),
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
	remoteApp, err := api.processRemoteApplication(url, alias)
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
