// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package application contains api calls for functionality
// related to deploying and managing applications and their
// related charms.
package application

import (
	"fmt"
	"net"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/set"
	"gopkg.in/juju/charm.v6-unstable"
	csparams "gopkg.in/juju/charmrepo.v2-unstable/csclient/params"
	"gopkg.in/juju/names.v2"
	"gopkg.in/macaroon.v1"
	goyaml "gopkg.in/yaml.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/storagecommon"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/permission"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
	"github.com/juju/juju/status"
)

var logger = loggo.GetLogger("juju.apiserver.application")

// APIv4 provides the Application API facade for versions 1-4.
type APIv4 struct {
	*API
}

// API implements the application interface and is the concrete
// implementation of the api end point. API provides the
// Application API facades for versions 1-4.
type API struct {
	backend    Backend
	authorizer facade.Authorizer
	check      BlockChecker

	// TODO(axw) stateCharm only exists because I ran out
	// of time unwinding all of the tendrils of state. We
	// should pass a charm.Charm and charm.URL back into
	// state wherever we pass in a state.Charm currently.
	stateCharm func(Charm) *state.Charm

	deployApplicationFunc func(ApplicationDeployer, DeployApplicationParams) (Application, error)
	getEnviron            stateenvirons.NewEnvironFunc
}

// NewFacadeV4 provides the signature required for facade registration
// for versions 1-4.
func NewFacadeV4(ctx facade.Context) (*APIv4, error) {
	api, err := NewFacade(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv4{api}, nil
}

// NewFacade provides the signature required for facade registration.
func NewFacade(ctx facade.Context) (*API, error) {
	backend, err := NewStateBackend(ctx.State())
	if err != nil {
		return nil, errors.Annotate(err, "getting state")
	}
	blockChecker := common.NewBlockChecker(ctx.State())
	stateCharm := CharmToStateCharm
	return NewAPI(
		backend,
		ctx.Auth(),
		blockChecker,
		stateCharm,
		DeployApplication,
	)
}

// NewAPI returns a new application API facade.
func NewAPI(
	backend Backend,
	authorizer facade.Authorizer,
	blockChecker BlockChecker,
	stateCharm func(Charm) *state.Charm,
	deployApplication func(ApplicationDeployer, DeployApplicationParams) (Application, error),
) (*API, error) {
	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}
	return &API{
		backend:               backend,
		authorizer:            authorizer,
		check:                 blockChecker,
		stateCharm:            stateCharm,
		deployApplicationFunc: deployApplication,
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
	deployApplicationFunc func(ApplicationDeployer, DeployApplicationParams) (Application, error),
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

	// Parse storage tags in AttachStorage.
	if len(args.AttachStorage) > 0 && args.NumUnits != 1 {
		return errors.Errorf("AttachStorage is non-empty, but NumUnits is %d", args.NumUnits)
	}
	attachStorage := make([]names.StorageTag, len(args.AttachStorage))
	for i, tagString := range args.AttachStorage {
		tag, err := names.ParseStorageTag(tagString)
		if err != nil {
			return errors.Trace(err)
		}
		attachStorage[i] = tag
	}

	_, err = deployApplicationFunc(backend, DeployApplicationParams{
		ApplicationName:  args.ApplicationName,
		Series:           args.Series,
		Charm:            stateCharm(ch),
		Channel:          csparams.Channel(args.Channel),
		NumUnits:         args.NumUnits,
		ConfigSettings:   settings,
		Constraints:      args.Constraints,
		Placement:        args.Placement,
		Storage:          args.Storage,
		AttachStorage:    attachStorage,
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

// UpdateApplicationSeries updates the application series. Series for
// subordinates updated too.
func (api *API) UpdateApplicationSeries(args params.UpdateSeriesArgs) (params.ErrorResults, error) {
	if err := api.checkCanWrite(); err != nil {
		return params.ErrorResults{}, err
	}
	if err := api.check.ChangeAllowed(); err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Args)),
	}
	for i, arg := range args.Args {
		err := api.updateOneApplicationSeries(arg)
		results.Results[i].Error = common.ServerError(err)
	}
	return results, nil
}

func (api *API) updateOneApplicationSeries(arg params.UpdateSeriesArg) error {
	if arg.Series == "" {
		return &params.Error{
			Message: "series missing from args",
			Code:    params.CodeBadRequest,
		}
	}
	applicationTag, err := names.ParseApplicationTag(arg.Entity.Tag)
	if err != nil {
		return errors.Trace(err)
	}
	app, err := api.backend.Application(applicationTag.Id())
	if err != nil {
		return errors.Trace(err)
	}
	if !app.IsPrincipal() {
		return &params.Error{
			Message: fmt.Sprintf("%q is a subordinate application, update-series not supported", applicationTag.Id()),
			Code:    params.CodeNotSupported,
		}
	}
	if arg.Series == app.Series() {
		return nil // no-op
	}
	return app.UpdateApplicationSeries(arg.Series, arg.Force)
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
		unitNames[i] = unit.UnitTag().Id()
	}
	return params.AddApplicationUnitsResults{Units: unitNames}, nil
}

// addApplicationUnits adds a given number of units to an application.
func addApplicationUnits(backend Backend, args params.AddApplicationUnits) ([]Unit, error) {
	if args.NumUnits < 1 {
		return nil, errors.New("must add at least one unit")
	}
	// Parse storage tags in AttachStorage.
	if len(args.AttachStorage) > 0 && args.NumUnits != 1 {
		return nil, errors.Errorf("AttachStorage is non-empty, but NumUnits is %d", args.NumUnits)
	}
	attachStorage := make([]names.StorageTag, len(args.AttachStorage))
	for i, tagString := range args.AttachStorage {
		tag, err := names.ParseStorageTag(tagString)
		if err != nil {
			return nil, errors.Trace(err)
		}
		attachStorage[i] = tag
	}
	application, err := backend.Application(args.ApplicationName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return addUnits(
		application,
		args.ApplicationName,
		args.NumUnits,
		args.Placement,
		attachStorage,
	)
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
		return params.DestroyUnitResults{}, errors.Trace(err)
	}
	if err := api.check.RemoveAllowed(); err != nil {
		return params.DestroyUnitResults{}, errors.Trace(err)
	}
	destroyUnit := func(entity params.Entity) (*params.DestroyUnitInfo, error) {
		unitTag, err := names.ParseUnitTag(entity.Tag)
		if err != nil {
			return nil, errors.Trace(err)
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
		storage, err := storagecommon.UnitStorage(api.backend, unit.UnitTag())
		if err != nil {
			return nil, errors.Trace(err)
		}
		info.DestroyedStorage, info.DetachedStorage, err = storagecommon.ClassifyDetachedStorage(
			api.backend, storage,
		)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if err := unit.Destroy(); err != nil {
			return nil, errors.Trace(err)
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
			storage, err := storagecommon.UnitStorage(api.backend, unit.UnitTag())
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

			destroyed, detached, err := storagecommon.ClassifyDetachedStorage(
				api.backend, storage,
			)
			if err != nil {
				return nil, err
			}
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

// AddRelation adds a relation between the specified endpoints and returns the relation info.
func (api *API) AddRelation(args params.AddRelation) (_ params.AddRelationResults, err error) {
	var rel Relation
	defer func() {
		if err != nil && rel != nil {
			if err := rel.Destroy(); err != nil {
				logger.Errorf("cannot destroy aborted relation %q: %v", rel.Tag().Id(), err)
			}
		}
	}()

	if err := api.check.ChangeAllowed(); err != nil {
		return params.AddRelationResults{}, errors.Trace(err)
	}
	if err := api.checkCanWrite(); err != nil {
		return params.AddRelationResults{}, errors.Trace(err)
	}

	// Validate any CIDRs.
	for _, cidr := range args.ViaCIDRs {
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			return params.AddRelationResults{}, errors.Trace(err)
		}
		if cidr == "0.0.0.0/0" {
			return params.AddRelationResults{}, errors.Errorf("CIDR %q not allowed", cidr)
		}
	}

	inEps, err := api.backend.InferEndpoints(args.Endpoints...)
	if err != nil {
		return params.AddRelationResults{}, errors.Trace(err)
	}
	if rel, err = api.backend.AddRelation(inEps...); err != nil {
		return params.AddRelationResults{}, errors.Trace(err)
	}
	if _, err := api.backend.SaveEgressNetworks(rel.Tag().Id(), args.ViaCIDRs); err != nil {
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

// DestroyRelation removes the relation between the
// specified endpoints or an id.
func (api *API) DestroyRelation(args params.DestroyRelation) (err error) {
	if err := api.checkCanWrite(); err != nil {
		return err
	}
	if err := api.check.RemoveAllowed(); err != nil {
		return errors.Trace(err)
	}
	var (
		rel Relation
		eps []state.Endpoint
	)
	if len(args.Endpoints) > 0 {
		eps, err = api.backend.InferEndpoints(args.Endpoints...)
		if err != nil {
			return err
		}
		rel, err = api.backend.EndpointsRelation(eps...)
	} else {
		rel, err = api.backend.Relation(args.RelationId)
	}
	if err != nil {
		return err
	}
	return rel.Destroy()
}

// SetRelationsSuspended sets the suspended status of the specified relations.
func (api *API) SetRelationsSuspended(args params.RelationSuspendedArgs) (params.ErrorResults, error) {
	var statusResults params.ErrorResults
	if err := api.checkCanWrite(); err != nil {
		return statusResults, errors.Trace(err)
	}
	if err := api.check.ChangeAllowed(); err != nil {
		return statusResults, errors.Trace(err)
	}

	changeOne := func(arg params.RelationSuspendedArg) error {
		rel, err := api.backend.Relation(arg.RelationId)
		if err != nil {
			return errors.Trace(err)
		}
		if rel.Suspended() == arg.Suspended {
			return nil
		}
		_, err = api.backend.OfferConnectionForRelation(rel.Tag().Id())
		if errors.IsNotFound(err) {
			return errors.Errorf("cannot set suspend status for %q which is not associated with an offer", rel.Tag().Id())
		}
		err = rel.SetSuspended(arg.Suspended)
		if err != nil {
			return errors.Trace(err)
		}

		statusValue := status.Joining
		if arg.Suspended {
			statusValue = status.Suspending
		}
		return rel.SetStatus(status.StatusInfo{
			Status:  status.Status(statusValue),
			Message: arg.Message,
		})
	}
	results := make([]params.ErrorResult, len(args.Args))
	for i, arg := range args.Args {
		err := changeOne(arg)
		results[i].Error = common.ServerError(err)
	}
	statusResults.Results = results
	return statusResults, nil
}

// Consume adds remote applications to the model without creating any
// relations.
func (api *API) Consume(args params.ConsumeApplicationArgs) (params.ErrorResults, error) {
	var consumeResults params.ErrorResults
	if err := api.checkCanWrite(); err != nil {
		return consumeResults, errors.Trace(err)
	}
	if err := api.check.ChangeAllowed(); err != nil {
		return consumeResults, errors.Trace(err)
	}

	results := make([]params.ErrorResult, len(args.Args))
	for i, arg := range args.Args {
		err := api.consumeOne(arg)
		results[i].Error = common.ServerError(err)
	}
	consumeResults.Results = results
	return consumeResults, nil
}

func (api *API) consumeOne(arg params.ConsumeApplicationArg) error {
	sourceModelTag, err := names.ParseModelTag(arg.SourceModelTag)
	if err != nil {
		return errors.Trace(err)
	}

	// Maybe save the details of the controller hosting the offer.
	if arg.ControllerInfo != nil {
		controllerTag, err := names.ParseControllerTag(arg.ControllerInfo.ControllerTag)
		if err != nil {
			return errors.Trace(err)
		}
		// Only save controller details if the offer comes from
		// a different controller.
		if controllerTag.Id() != api.backend.ControllerTag().Id() {
			if _, err = api.backend.SaveController(crossmodel.ControllerInfo{
				ControllerTag: controllerTag,
				Addrs:         arg.ControllerInfo.Addrs,
				CACert:        arg.ControllerInfo.CACert,
			}, sourceModelTag.Id()); err != nil {
				return errors.Trace(err)
			}
		}
	}

	appName := arg.ApplicationAlias
	if appName == "" {
		appName = arg.OfferName
	}
	_, err = api.saveRemoteApplication(sourceModelTag, appName, arg.ApplicationOffer, arg.Macaroon)
	return err
}

// saveRemoteApplication saves the details of the specified remote application and its endpoints
// to the state model so relations to the remote application can be created.
func (api *API) saveRemoteApplication(
	sourceModelTag names.ModelTag,
	applicationName string,
	offer params.ApplicationOffer,
	mac *macaroon.Macaroon,
) (RemoteApplication, error) {
	remoteEps := make([]charm.Relation, len(offer.Endpoints))
	for j, ep := range offer.Endpoints {
		remoteEps[j] = charm.Relation{
			Name:      ep.Name,
			Role:      ep.Role,
			Interface: ep.Interface,
		}
	}

	remoteSpaces := make([]*environs.ProviderSpaceInfo, len(offer.Spaces))
	for i, space := range offer.Spaces {
		remoteSpaces[i] = providerSpaceInfoFromParams(space)
	}

	// If the a remote application with the same name and endpoints from the same
	// source model already exists, we will use that one.
	remoteApp, err := api.maybeUpdateExistingApplicationEndpoints(applicationName, sourceModelTag, remoteEps)
	if err == nil {
		return remoteApp, nil
	} else if !errors.IsNotFound(err) {
		return nil, errors.Trace(err)
	}

	return api.backend.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name:        applicationName,
		OfferUUID:   offer.OfferUUID,
		URL:         offer.OfferURL,
		SourceModel: sourceModelTag,
		Endpoints:   remoteEps,
		Spaces:      remoteSpaces,
		Bindings:    offer.Bindings,
		Macaroon:    mac,
	})
}

// providerSpaceInfoFromParams converts a params.RemoteSpace to the
// equivalent ProviderSpaceInfo.
func providerSpaceInfoFromParams(space params.RemoteSpace) *environs.ProviderSpaceInfo {
	result := &environs.ProviderSpaceInfo{
		CloudType:          space.CloudType,
		ProviderAttributes: space.ProviderAttributes,
		SpaceInfo: network.SpaceInfo{
			Name:       space.Name,
			ProviderId: network.Id(space.ProviderId),
		},
	}
	for _, subnet := range space.Subnets {
		resultSubnet := network.SubnetInfo{
			CIDR:              subnet.CIDR,
			ProviderId:        network.Id(subnet.ProviderId),
			ProviderNetworkId: network.Id(subnet.ProviderNetworkId),
			SpaceProviderId:   network.Id(subnet.ProviderSpaceId),
			VLANTag:           subnet.VLANTag,
			AvailabilityZones: subnet.Zones,
		}
		result.Subnets = append(result.Subnets, resultSubnet)
	}
	return result
}

// maybeUpdateExistingApplicationEndpoints looks for a remote application with the
// specified name and source model tag and tries to update its endpoints with the
// new ones specified. If the endpoints are compatible, the newly updated remote
// application is returned.
func (api *API) maybeUpdateExistingApplicationEndpoints(
	applicationName string, sourceModelTag names.ModelTag, remoteEps []charm.Relation,
) (RemoteApplication, error) {
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
