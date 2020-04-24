// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package application provides access to the application api facade.
// This facade contains api calls that are specific to applications.
// As a rule of thumb, if the argument for an api requires an application name
// and affects only that application then the call belongs here.
package application

import (
	"time"

	"github.com/juju/charm/v7"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/charmstore"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/devices"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/storage"
)

var logger = loggo.GetLogger("juju.api.application")

// Client allows access to the application API end point.
type Client struct {
	base.ClientFacade
	st     base.APICallCloser
	facade base.FacadeCaller
}

// NewClient creates a new client for accessing the application api.
func NewClient(st base.APICallCloser) *Client {
	frontend, backend := base.NewClientFacade(st, "Application")
	return &Client{ClientFacade: frontend, st: st, facade: backend}
}

// SetMetricCredentials sets the metric credentials for the application specified.
func (c *Client) SetMetricCredentials(application string, credentials []byte) error {
	creds := []params.ApplicationMetricCredential{
		{application, credentials},
	}
	p := params.ApplicationMetricCredentials{Creds: creds}
	results := new(params.ErrorResults)
	err := c.facade.FacadeCall("SetMetricCredentials", p, results)
	if err != nil {
		return errors.Trace(err)
	}
	return errors.Trace(results.OneError())
}

// ModelUUID returns the model UUID from the client connection.
func (c *Client) ModelUUID() string {
	tag, ok := c.st.ModelTag()
	if !ok {
		logger.Warningf("controller-only API connection has no model tag")
	}
	return tag.Id()
}

// DeployArgs holds the arguments to be sent to Client.ApplicationDeploy.
type DeployArgs struct {
	// CharmID identifies the charm to deploy.
	CharmID charmstore.CharmID

	// ApplicationName is the name to give the application.
	ApplicationName string

	// Series to be used for the machine.
	Series string

	// NumUnits is the number of units to deploy.
	NumUnits int

	// ConfigYAML is a string that overrides the default config.yml.
	ConfigYAML string

	// Config are values that override those in the default config.yaml
	// or configure the application itself.
	Config map[string]string

	// Cons contains constraints on where units of this application
	// may be placed.
	Cons constraints.Value

	// Placement directives on where the machines for the unit must be
	// created.
	Placement []*instance.Placement

	// Storage contains Constraints specifying how storage should be
	// handled.
	Storage map[string]storage.Constraints

	// Devices contains Constraints specifying how devices should be
	// handled.
	Devices map[string]devices.Constraints

	// AttachStorage contains IDs of existing storage that should be
	// attached to the application unit that will be deployed. This
	// may be non-empty only if NumUnits is 1.
	AttachStorage []string

	// EndpointBindings
	EndpointBindings map[string]string

	// Collection of resource names for the application, with the
	// value being the unique ID of a pre-uploaded resources in
	// storage.
	Resources map[string]string
}

// Deploy obtains the charm, either locally or from the charm store, and deploys
// it. Placement directives, if provided, specify the machine on which the charm
// is deployed.
func (c *Client) Deploy(args DeployArgs) error {
	if len(args.AttachStorage) > 0 {
		if args.NumUnits != 1 {
			return errors.New("cannot attach existing storage when more than one unit is requested")
		}
		if c.BestAPIVersion() < 5 {
			return errors.New("this juju controller does not support AttachStorage")
		}
	}
	attachStorage := make([]string, len(args.AttachStorage))
	for i, id := range args.AttachStorage {
		if !names.IsValidStorage(id) {
			return errors.NotValidf("storage ID %q", id)
		}
		attachStorage[i] = names.NewStorageTag(id).String()
	}
	deployArgs := params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			ApplicationName:  args.ApplicationName,
			Series:           args.Series,
			CharmURL:         args.CharmID.URL.String(),
			Channel:          string(args.CharmID.Channel),
			NumUnits:         args.NumUnits,
			ConfigYAML:       args.ConfigYAML,
			Config:           args.Config,
			Constraints:      args.Cons,
			Placement:        args.Placement,
			Storage:          args.Storage,
			Devices:          args.Devices,
			AttachStorage:    attachStorage,
			EndpointBindings: args.EndpointBindings,
			Resources:        args.Resources,
		}},
	}
	var results params.ErrorResults
	var err error
	err = c.facade.FacadeCall("Deploy", deployArgs, &results)
	if err != nil {
		return errors.Trace(err)
	}
	return errors.Trace(results.OneError())
}

// GetCharmURL returns the charm URL the given application is
// running at present.
func (c *Client) GetCharmURL(branchName, applicationName string) (*charm.URL, error) {
	result := new(params.StringResult)
	args := params.ApplicationGet{
		ApplicationName: applicationName,
		BranchName:      branchName,
	}
	err := c.facade.FacadeCall("GetCharmURL", args, result)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if result.Error != nil {
		return nil, errors.Trace(result.Error)
	}
	return charm.ParseURL(result.Result)
}

// GetConfig returns the charm configuration settings for each of the
// applications. If any of the applications are not found, an error is
// returned.
func (c *Client) GetConfig(branchName string, appNames ...string) ([]map[string]interface{}, error) {
	v := c.BestAPIVersion()

	if v < 5 {
		settings, err := c.getConfigV4(branchName, appNames)
		return settings, errors.Trace(err)
	}

	callName := "CharmConfig"
	if v < 6 {
		callName = "GetConfig"
	}

	var callArg interface{}
	if v < 9 {
		arg := params.Entities{Entities: make([]params.Entity, len(appNames))}
		for i, appName := range appNames {
			arg.Entities[i] = params.Entity{Tag: names.NewApplicationTag(appName).String()}
		}
		callArg = arg
	} else {
		// Version 9 of the API introduces generational config.
		arg := params.ApplicationGetArgs{Args: make([]params.ApplicationGet, len(appNames))}
		for i, appName := range appNames {
			arg.Args[i] = params.ApplicationGet{ApplicationName: appName, BranchName: branchName}
		}
		callArg = arg
	}

	var results params.ApplicationGetConfigResults
	err := c.facade.FacadeCall(callName, callArg, &results)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var settings []map[string]interface{}
	for i, result := range results.Results {
		if result.Error != nil {
			return nil, errors.Annotatef(err, "unable to get settings for %q", appNames[i])
		}
		settings = append(settings, result.Config)
	}
	return settings, nil
}

// getConfigV4 retrieves application config for versions of the API < 5.
func (c *Client) getConfigV4(branchName string, appNames []string) ([]map[string]interface{}, error) {
	var allSettings []map[string]interface{}
	for _, appName := range appNames {
		results, err := c.Get(branchName, appName)
		if err != nil {
			return nil, errors.Annotatef(err, "unable to get settings for %q", appName)
		}
		settings, err := describeV5(results.CharmConfig)
		if err != nil {
			return nil, errors.Annotatef(err, "unable to process settings for %q", appName)
		}
		allSettings = append(allSettings, settings)
	}
	return allSettings, nil
}

// describeV5 will take the results of describeV4 from the apiserver
// and remove the "default" boolean, and add in "source".
// Mutates and returns the config map.
func describeV5(config map[string]interface{}) (map[string]interface{}, error) {
	for _, value := range config {
		vMap, ok := value.(map[string]interface{})
		if !ok {
			return nil, errors.Errorf("expected settings map got %v (%T) ", value, value)
		}
		if _, found := vMap["default"]; found {
			v, hasValue := vMap["value"]
			if hasValue {
				vMap["default"] = v
				vMap["source"] = "default"
			} else {
				delete(vMap, "default")
				vMap["source"] = "unset"
			}
		} else {
			// If default isn't set, then the source is user.
			// And we have no idea what the charm default is or whether
			// there is one.
			vMap["source"] = "user"
		}
	}
	return config, nil
}

// SetCharmConfig holds the configuration for setting a new revision of a charm
// on a application.
type SetCharmConfig struct {
	// ApplicationName is the name of the application to set the charm on.
	ApplicationName string

	// CharmID identifies the charm.
	CharmID charmstore.CharmID

	// ConfigSettings is the charm settings to set during the upgrade.
	// This field is only understood by Application facade version 2
	// and greater.
	ConfigSettings map[string]string `json:"config-settings,omitempty"`

	// ConfigSettingsYAML is the charm settings in YAML format to set
	// during the upgrade. If this is non-empty, it will take precedence
	// over ConfigSettings. This field is only understood by Application
	// facade version 2
	ConfigSettingsYAML string `json:"config-settings-yaml,omitempty"`

	// Force forces the use of the charm in the following scenarios:
	// overriding a lxd profile upgrade.
	// In the future, we should deprecate ForceSeries and ForceUnits and just
	// use Force for all instances.
	// TODO (stickupkid): deprecate ForceSeries and ForceUnits in favour of
	// just using Force.
	Force bool

	// ForceSeries forces the use of the charm even if it doesn't match the
	// series of the unit.
	ForceSeries bool

	// ForceUnits forces the upgrade on units in an error state.
	ForceUnits bool

	// ResourceIDs is a map of resource names to resource IDs to activate during
	// the upgrade.
	ResourceIDs map[string]string

	// StorageConstraints is a map of storage names to storage constraints to
	// update during the upgrade. This field is only understood by Application
	// facade version 2 and greater.
	StorageConstraints map[string]storage.Constraints `json:"storage-constraints,omitempty"`

	// EndpointBindings is a map of operator-defined endpoint names to
	// space names to be merged with any existing endpoint bindings.
	EndpointBindings map[string]string
}

// SetCharm sets the charm for a given application.
func (c *Client) SetCharm(branchName string, cfg SetCharmConfig) error {
	var storageConstraints map[string]params.StorageConstraints
	if len(cfg.StorageConstraints) > 0 {
		storageConstraints = make(map[string]params.StorageConstraints)
		for name, cons := range cfg.StorageConstraints {
			size, count := cons.Size, cons.Count
			var sizePtr, countPtr *uint64
			if size > 0 {
				sizePtr = &size
			}
			if count > 0 {
				countPtr = &count
			}
			storageConstraints[name] = params.StorageConstraints{
				Pool:  cons.Pool,
				Size:  sizePtr,
				Count: countPtr,
			}
		}
	}
	args := params.ApplicationSetCharm{
		ApplicationName:    cfg.ApplicationName,
		CharmURL:           cfg.CharmID.URL.String(),
		Channel:            string(cfg.CharmID.Channel),
		ConfigSettings:     cfg.ConfigSettings,
		ConfigSettingsYAML: cfg.ConfigSettingsYAML,
		Force:              cfg.Force,
		ForceSeries:        cfg.ForceSeries,
		ForceUnits:         cfg.ForceUnits,
		ResourceIDs:        cfg.ResourceIDs,
		StorageConstraints: storageConstraints,
		EndpointBindings:   cfg.EndpointBindings,
		Generation:         branchName,
	}
	return c.facade.FacadeCall("SetCharm", args, nil)
}

// Update updates the application attributes, including charm URL,
// minimum number of units, settings and constraints.
func (c *Client) Update(args params.ApplicationUpdate) error {
	return c.facade.FacadeCall("Update", args, nil)
}

// UpdateApplicationSeries updates the application series in the db.
func (c *Client) UpdateApplicationSeries(appName, series string, force bool) error {
	args := params.UpdateSeriesArgs{
		Args: []params.UpdateSeriesArg{{
			Entity: params.Entity{Tag: names.NewApplicationTag(appName).String()},
			Force:  force,
			Series: series,
		}},
	}

	results := new(params.ErrorResults)
	err := c.facade.FacadeCall("UpdateApplicationSeries", args, results)
	if err != nil {
		return errors.Trace(err)
	}
	return results.OneError()
}

// AddUnitsParams contains parameters for the AddUnits API method.
type AddUnitsParams struct {
	// ApplicationName is the name of the application to which units
	// will be added.
	ApplicationName string

	// NumUnits is the number of units to deploy.
	NumUnits int

	// Placement directives on where the machines for the unit must be
	// created.
	Placement []*instance.Placement

	// Policy represents how a machine for the unit is determined.
	// This value is ignored on any Juju server before 2.4.
	Policy string

	// AttachStorage contains IDs of existing storage that should be
	// attached to the application unit that will be deployed. This
	// may be non-empty only if NumUnits is 1.
	AttachStorage []string
}

// AddUnits adds a given number of units to an application using the specified
// placement directives to assign units to machines.
func (c *Client) AddUnits(args AddUnitsParams) ([]string, error) {
	if len(args.AttachStorage) > 0 {
		if args.NumUnits != 1 {
			return nil, errors.New("cannot attach existing storage when more than one unit is requested")
		}
		if c.BestAPIVersion() < 5 {
			return nil, errors.New("this juju controller does not support AttachStorage")
		}
	}
	attachStorage := make([]string, len(args.AttachStorage))
	for i, id := range args.AttachStorage {
		if !names.IsValidStorage(id) {
			return nil, errors.NotValidf("storage ID %q", id)
		}
		attachStorage[i] = names.NewStorageTag(id).String()
	}
	results := new(params.AddApplicationUnitsResults)
	err := c.facade.FacadeCall("AddUnits", params.AddApplicationUnits{
		ApplicationName: args.ApplicationName,
		NumUnits:        args.NumUnits,
		Placement:       args.Placement,
		Policy:          args.Policy,
		AttachStorage:   attachStorage,
	}, results)
	return results.Units, err
}

// DestroyUnitsDeprecated decreases the number of units dedicated to an
// application.
//
// NOTE(axw) this exists only for backwards compatibility, for API facade
// versions 1-3; clients should prefer its successor, DestroyUnits, below.
//
// TODO(axw) 2017-03-16 #1673323
// Drop this in Juju 3.0.
func (c *Client) DestroyUnitsDeprecated(unitNames ...string) error {
	args := params.DestroyApplicationUnits{UnitNames: unitNames}
	return c.facade.FacadeCall("DestroyUnits", args, nil)
}

// DestroyUnitsParams contains parameters for the DestroyUnits API method.
type DestroyUnitsParams struct {
	// Units holds the IDs of units to destroy.
	Units []string

	// DestroyStorage controls whether or not storage attached
	// to the units will be destroyed.
	DestroyStorage bool

	// Force controls whether or not the removal of applications
	// will be forced, i.e. ignore removal errors.
	Force bool

	// MaxWait specifies the amount of time that each step in unit removal
	// will wait before forcing the next step to kick-off. This parameter
	// only makes sense in combination with 'force' set to 'true'.
	MaxWait *time.Duration
}

// DestroyUnits decreases the number of units dedicated to one or more
// applications.
func (c *Client) DestroyUnits(in DestroyUnitsParams) ([]params.DestroyUnitResult, error) {
	argsV5 := params.DestroyUnitsParams{
		Units: make([]params.DestroyUnitParams, 0, len(in.Units)),
	}
	allResults := make([]params.DestroyUnitResult, len(in.Units))
	index := make([]int, 0, len(in.Units))
	for i, name := range in.Units {
		if !names.IsValidUnit(name) {
			allResults[i].Error = &params.Error{
				Message: errors.NotValidf("unit ID %q", name).Error(),
			}
			continue
		}
		index = append(index, i)
		argsV5.Units = append(argsV5.Units, params.DestroyUnitParams{
			UnitTag:        names.NewUnitTag(name).String(),
			DestroyStorage: in.DestroyStorage,
			Force:          in.Force,
			MaxWait:        in.MaxWait,
		})
	}
	if len(argsV5.Units) == 0 {
		return allResults, nil
	}

	args := interface{}(argsV5)
	if c.BestAPIVersion() < 5 {
		if in.DestroyStorage {
			return nil, errors.New("this controller does not support --destroy-storage")
		}
		argsV4 := params.Entities{
			Entities: make([]params.Entity, len(argsV5.Units)),
		}
		for i, arg := range argsV5.Units {
			argsV4.Entities[i].Tag = arg.UnitTag
		}
		args = argsV4
	}

	var result params.DestroyUnitResults
	if err := c.facade.FacadeCall("DestroyUnit", args, &result); err != nil {
		return nil, errors.Trace(err)
	}
	if n := len(result.Results); n != len(argsV5.Units) {
		return nil, errors.Errorf("expected %d result(s), got %d", len(argsV5.Units), n)
	}
	for i, result := range result.Results {
		allResults[index[i]] = result
	}
	return allResults, nil
}

// DestroyDeprecated destroys a given application.
//
// NOTE(axw) this exists only for backwards compatibility,
// for API facade versions 1-3; clients should prefer its
// successor, DestroyApplications, below.
//
// TODO(axw) 2017-03-16 #1673323
// Drop this in Juju 3.0.
func (c *Client) DestroyDeprecated(application string) error {
	args := params.ApplicationDestroy{
		ApplicationName: application,
	}
	return c.facade.FacadeCall("Destroy", args, nil)
}

// DestroyApplicationsParams contains parameters for the DestroyApplications
// API method.
type DestroyApplicationsParams struct {
	// Applications holds the names of applications to destroy.
	Applications []string

	// DestroyStorage controls whether or not storage attached
	// to units of the applications will be destroyed.
	DestroyStorage bool

	// Force controls whether or not the removal of applications
	// will be forced, i.e. ignore removal errors.
	Force bool

	// MaxWait specifies the amount of time that each step in application removal
	// will wait before forcing the next step to kick-off. This parameter
	// only makes sense in combination with 'force' set to 'true'.
	MaxWait *time.Duration
}

// DestroyApplications destroys the given applications.
func (c *Client) DestroyApplications(in DestroyApplicationsParams) ([]params.DestroyApplicationResult, error) {
	argsV5 := params.DestroyApplicationsParams{
		Applications: make([]params.DestroyApplicationParams, 0, len(in.Applications)),
	}
	allResults := make([]params.DestroyApplicationResult, len(in.Applications))
	index := make([]int, 0, len(in.Applications))
	for i, name := range in.Applications {
		if !names.IsValidApplication(name) {
			allResults[i].Error = &params.Error{
				Message: errors.NotValidf("application name %q", name).Error(),
			}
			continue
		}
		index = append(index, i)
		argsV5.Applications = append(argsV5.Applications, params.DestroyApplicationParams{
			ApplicationTag: names.NewApplicationTag(name).String(),
			DestroyStorage: in.DestroyStorage,
			Force:          in.Force,
			MaxWait:        in.MaxWait,
		})
	}
	if len(argsV5.Applications) == 0 {
		return allResults, nil
	}

	args := interface{}(argsV5)
	if c.BestAPIVersion() < 5 {
		if in.DestroyStorage {
			return nil, errors.New("this controller does not support --destroy-storage")
		}
		argsV4 := params.Entities{
			Entities: make([]params.Entity, len(argsV5.Applications)),
		}
		for i, arg := range argsV5.Applications {
			argsV4.Entities[i].Tag = arg.ApplicationTag
		}
		args = argsV4
	}

	var result params.DestroyApplicationResults
	if err := c.facade.FacadeCall("DestroyApplication", args, &result); err != nil {
		return nil, errors.Trace(err)
	}
	if n := len(result.Results); n != len(argsV5.Applications) {
		return nil, errors.Errorf("expected %d result(s), got %d", len(argsV5.Applications), n)
	}
	for i, result := range result.Results {
		allResults[index[i]] = result
	}
	return allResults, nil
}

type DestroyConsumedApplicationParams struct {
	// SaasNames holds the names of the consumed applications
	// that are being destroyed
	SaasNames []string

	// Force controls whether or not the removal of applications
	// will be forced, i.e. ignore removal errors.
	Force bool

	// MaxWait specifies the amount of time that each step in application removal
	// will wait before forcing the next step to kick-off. This parameter
	// only makes sense in combination with 'force' set to 'true'.
	MaxWait *time.Duration
}

// DestroyConsumedApplication destroys the given consumed (remote) applications.
func (c *Client) DestroyConsumedApplication(in DestroyConsumedApplicationParams) ([]params.ErrorResult, error) {
	apiVersion := c.BestAPIVersion()
	args := params.DestroyConsumedApplicationsParams{
		Applications: make([]params.DestroyConsumedApplicationParams, 0, len(in.SaasNames)),
	}

	if apiVersion > 9 {
		if in.MaxWait != nil && !in.Force {
			return nil, errors.New("--force is required when --max-wait is provided")
		}
	} else {
		if in.Force {
			return nil, errors.New("this controller does not support --force")
		}
		if in.MaxWait != nil {
			return nil, errors.New("this controller does not support --no-wait")
		}
	}

	allResults := make([]params.ErrorResult, len(in.SaasNames))
	index := make([]int, 0, len(in.SaasNames))
	for i, name := range in.SaasNames {
		if !names.IsValidApplication(name) {
			allResults[i].Error = &params.Error{
				Message: errors.NotValidf("SAAS application name %q", name).Error(),
			}
			continue
		}
		index = append(index, i)
		appParams := params.DestroyConsumedApplicationParams{
			ApplicationTag: names.NewApplicationTag(name).String(),
		}
		if apiVersion > 9 {
			appParams.Force = &in.Force
			appParams.MaxWait = in.MaxWait
		}
		args.Applications = append(args.Applications, appParams)
	}

	var result params.ErrorResults
	if err := c.facade.FacadeCall("DestroyConsumedApplications", args, &result); err != nil {
		return nil, errors.Trace(err)
	}
	if n := len(result.Results); n != len(args.Applications) {
		return nil, errors.Errorf("expected %d result(s), got %d", len(args.Applications), n)
	}
	for i, result := range result.Results {
		allResults[index[i]] = result
	}
	return allResults, nil
}

// ScaleApplicationParams contains parameters for the ScaleApplication API method.
type ScaleApplicationParams struct {
	// ApplicationName is the application to scale.
	ApplicationName string

	// Scale is the target number of units which should should be running.
	Scale int

	// ScaleChange is the amount of change to the target number of existing units.
	ScaleChange int

	// Force controls whether or not the removal of applications
	// will be forced, i.e. ignore removal errors.
	Force bool
}

// ScaleApplication sets the desired unit count for one or more applications.
func (c *Client) ScaleApplication(in ScaleApplicationParams) (params.ScaleApplicationResult, error) {
	if !names.IsValidApplication(in.ApplicationName) {
		return params.ScaleApplicationResult{}, errors.NotValidf("application %q", in.ApplicationName)
	}

	if err := validateApplicationScale(in.Scale, in.ScaleChange); err != nil {
		return params.ScaleApplicationResult{}, errors.Trace(err)
	}

	args := params.ScaleApplicationsParams{
		Applications: []params.ScaleApplicationParams{{
			ApplicationTag: names.NewApplicationTag(in.ApplicationName).String(),
			Scale:          in.Scale,
			ScaleChange:    in.ScaleChange,
			Force:          in.Force,
		}},
	}
	var results params.ScaleApplicationResults
	if err := c.facade.FacadeCall("ScaleApplications", args, &results); err != nil {
		return params.ScaleApplicationResult{}, errors.Trace(err)
	}
	if n := len(results.Results); n != 1 {
		return params.ScaleApplicationResult{}, errors.Errorf("expected 1 result, got %d", n)
	}
	result := results.Results[0]
	if err := result.Error; err != nil {
		return params.ScaleApplicationResult{}, err
	}
	return results.Results[0], nil
}

// GetConstraints returns the constraints for the given applications.
func (c *Client) GetConstraints(applications ...string) ([]constraints.Value, error) {
	var allConstraints []constraints.Value
	if c.BestAPIVersion() < 5 {
		for _, application := range applications {
			var result params.GetConstraintsResults
			err := c.facade.FacadeCall(
				"GetConstraints", params.GetApplicationConstraints{ApplicationName: application}, &result)
			if err != nil {
				return nil, errors.Trace(err)
			}
			allConstraints = append(allConstraints, result.Constraints)
		}
		return allConstraints, nil
	}

	// Make a single call to get all the constraints.
	var results params.ApplicationGetConstraintsResults
	var args params.Entities
	for _, application := range applications {
		args.Entities = append(args.Entities,
			params.Entity{Tag: names.NewApplicationTag(application).String()})
	}
	err := c.facade.FacadeCall("GetConstraints", args, &results)
	if err != nil {
		return nil, errors.Trace(err)
	}
	for i, result := range results.Results {
		if result.Error != nil {
			return nil, errors.Annotatef(result.Error, "unable to get constraints for %q", applications[i])
		}
		allConstraints = append(allConstraints, result.Constraints)
	}
	return allConstraints, nil
}

// SetConstraints specifies the constraints for the given application.
func (c *Client) SetConstraints(application string, constraints constraints.Value) error {
	args := params.SetConstraints{
		ApplicationName: application,
		Constraints:     constraints,
	}
	return c.facade.FacadeCall("SetConstraints", args, nil)
}

// Expose changes the juju-managed firewall to expose any ports that
// were also explicitly marked by units as open.
func (c *Client) Expose(application string) error {
	args := params.ApplicationExpose{ApplicationName: application}
	return c.facade.FacadeCall("Expose", args, nil)
}

// Unexpose changes the juju-managed firewall to unexpose any ports that
// were also explicitly marked by units as open.
func (c *Client) Unexpose(application string) error {
	args := params.ApplicationUnexpose{ApplicationName: application}
	return c.facade.FacadeCall("Unexpose", args, nil)
}

// Get returns the configuration for the named application.
func (c *Client) Get(branchName, application string) (*params.ApplicationGetResults, error) {
	var results params.ApplicationGetResults
	args := params.ApplicationGet{
		ApplicationName: application,
		BranchName:      branchName,
	}
	err := c.facade.FacadeCall("Get", args, &results)
	return &results, err
}

// Set sets configuration options on an application.
func (c *Client) Set(application string, options map[string]string) error {
	p := params.ApplicationSet{
		ApplicationName: application,
		Options:         options,
	}
	return c.facade.FacadeCall("Set", p, nil)
}

// Unset resets configuration options on an application.
func (c *Client) Unset(application string, options []string) error {
	p := params.ApplicationUnset{
		ApplicationName: application,
		Options:         options,
	}
	return c.facade.FacadeCall("Unset", p, nil)
}

// CharmRelations returns the application's charms relation names.
func (c *Client) CharmRelations(application string) ([]string, error) {
	var results params.ApplicationCharmRelationsResults
	args := params.ApplicationCharmRelations{ApplicationName: application}
	err := c.facade.FacadeCall("CharmRelations", args, &results)
	return results.CharmRelations, err
}

// AddRelation adds a relation between the specified endpoints and returns the relation info.
func (c *Client) AddRelation(endpoints, viaCIDRs []string) (*params.AddRelationResults, error) {
	var addRelRes params.AddRelationResults
	args := params.AddRelation{Endpoints: endpoints, ViaCIDRs: viaCIDRs}
	err := c.facade.FacadeCall("AddRelation", args, &addRelRes)
	return &addRelRes, err
}

// DestroyRelation removes the relation between the specified endpoints.
func (c *Client) DestroyRelation(force *bool, maxWait *time.Duration, endpoints ...string) error {
	args := params.DestroyRelation{
		Endpoints: endpoints,
		Force:     force,
		MaxWait:   maxWait,
	}
	return c.facade.FacadeCall("DestroyRelation", args, nil)
}

// DestroyRelationId removes the relation with the specified id.
func (c *Client) DestroyRelationId(relationId int, force *bool, maxWait *time.Duration) error {
	args := params.DestroyRelation{
		RelationId: relationId,
		Force:      force,
		MaxWait:    maxWait,
	}
	return c.facade.FacadeCall("DestroyRelation", args, nil)
}

// SetRelationSuspended updates the suspended status of the relation with the specified id.
func (c *Client) SetRelationSuspended(relationIds []int, suspended bool, message string) error {
	var args params.RelationSuspendedArgs
	for _, relId := range relationIds {
		args.Args = append(args.Args, params.RelationSuspendedArg{
			RelationId: relId,
			Suspended:  suspended,
			Message:    message,
		})
	}
	var results params.ErrorResults
	if err := c.facade.FacadeCall("SetRelationsSuspended", args, &results); err != nil {
		return errors.Trace(err)
	}
	if len(results.Results) != len(args.Args) {
		return errors.Errorf("expected %d results, got %d", len(args.Args), len(results.Results))
	}
	return results.Combine()
}

// Consume adds a remote application to the model.
func (c *Client) Consume(arg crossmodel.ConsumeApplicationArgs) (string, error) {
	var consumeRes params.ErrorResults
	args := params.ConsumeApplicationArgs{
		Args: []params.ConsumeApplicationArg{{
			ApplicationOfferDetails: arg.Offer,
			ApplicationAlias:        arg.ApplicationAlias,
			Macaroon:                arg.Macaroon,
		}},
	}
	if arg.ControllerInfo != nil {
		args.Args[0].ControllerInfo = &params.ExternalControllerInfo{
			ControllerTag: arg.ControllerInfo.ControllerTag.String(),
			Alias:         arg.ControllerInfo.Alias,
			Addrs:         arg.ControllerInfo.Addrs,
			CACert:        arg.ControllerInfo.CACert,
		}
	}
	err := c.facade.FacadeCall("Consume", args, &consumeRes)
	if err != nil {
		return "", errors.Trace(err)
	}
	if resultLen := len(consumeRes.Results); resultLen != 1 {
		return "", errors.Errorf("expected 1 result, got %d", resultLen)
	}
	if err := consumeRes.Results[0].Error; err != nil {
		return "", errors.Trace(err)
	}
	localName := arg.Offer.OfferName
	if arg.ApplicationAlias != "" {
		localName = arg.ApplicationAlias
	}
	return localName, nil
}

// SetApplicationConfig sets configuration options on an application.
func (c *Client) SetApplicationConfig(branchName, application string, config map[string]string) error {
	if c.BestAPIVersion() < 6 {
		return errors.NotSupportedf("SetApplicationsConfig not supported by this version of Juju")
	}
	args := params.ApplicationConfigSetArgs{
		Args: []params.ApplicationConfigSet{{
			ApplicationName: application,
			Generation:      branchName,
			Config:          config,
		}},
	}
	var results params.ErrorResults
	err := c.facade.FacadeCall("SetApplicationsConfig", args, &results)
	if err != nil {
		return errors.Trace(err)
	}
	return results.OneError()
}

// UnsetApplicationConfig resets configuration options on an application.
func (c *Client) UnsetApplicationConfig(branchName, application string, options []string) error {
	if c.BestAPIVersion() < 6 {
		return errors.NotSupportedf("UnsetApplicationConfig not supported by this version of Juju")
	}
	args := params.ApplicationConfigUnsetArgs{
		Args: []params.ApplicationUnset{{
			ApplicationName: application,
			BranchName:      branchName,
			Options:         options,
		}},
	}
	var results params.ErrorResults
	err := c.facade.FacadeCall("UnsetApplicationsConfig", args, &results)
	if err != nil {
		return errors.Trace(err)
	}
	return results.OneError()
}

// ResolveUnitErrors clears errors on one or more units.
// Either specify one or more units, or all.
func (c *Client) ResolveUnitErrors(units []string, all, retry bool) error {
	if len(units) > 0 && all {
		return errors.NotSupportedf("specifying units with all=true")
	}
	if len(units) != set.NewStrings(units...).Size() {
		return errors.New("duplicate unit specified")
	}
	args := params.UnitsResolved{
		All:   all,
		Retry: retry,
	}
	if !all {
		entities := make([]params.Entity, len(units))
		for i, unit := range units {
			if !names.IsValidUnit(unit) {
				return errors.NotValidf("unit name %q", unit)
			}
			entities[i].Tag = names.NewUnitTag(unit).String()
		}
		args.Tags = params.Entities{Entities: entities}
	}

	results := new(params.ErrorResults)
	err := c.facade.FacadeCall("ResolveUnitErrors", args, results)
	if err != nil {
		return errors.Trace(err)
	}
	return errors.Trace(results.Combine())
}

func validateApplicationScale(scale, scaleChange int) error {
	if scale < 0 && scaleChange == 0 {
		return errors.NotValidf("scale < 0")
	} else if scale != 0 && scaleChange != 0 {
		return errors.NotValidf("requesting both scale and scale-change")
	}
	return nil
}

// ApplicationsInfo retrieves applications information.
func (c *Client) ApplicationsInfo(applications []names.ApplicationTag) ([]params.ApplicationInfoResult, error) {
	if apiVersion := c.BestAPIVersion(); apiVersion < 9 {
		return nil, errors.NotSupportedf("ApplicationsInfo for Application facade v%v", apiVersion)
	}
	all := make([]params.Entity, len(applications))
	for i, one := range applications {
		all[i] = params.Entity{Tag: one.String()}
	}
	in := params.Entities{Entities: all}
	var out params.ApplicationInfoResults
	err := c.facade.FacadeCall("ApplicationsInfo", in, &out)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if resultsLen := len(out.Results); resultsLen != len(applications) {
		return nil, errors.Errorf("expected %d results, got %d", len(applications), resultsLen)
	}
	return out.Results, nil
}

// MergeBindings merges an operator-defined bindings list with the existing
// application bindings.
func (c *Client) MergeBindings(req params.ApplicationMergeBindingsArgs) error {
	if apiVersion := c.BestAPIVersion(); apiVersion < 11 {
		return errors.NotSupportedf("MergeBindings for Application facade v%v", apiVersion)
	}

	var results params.ErrorResults
	err := c.facade.FacadeCall("MergeBindings", req, &results)
	if err != nil {
		return errors.Trace(err)
	}
	return results.OneError()
}
