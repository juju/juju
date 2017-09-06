// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package application provides access to the application api facade.
// This facade contains api calls that are specific to applications.
// As a rule of thumb, if the argument for an api requires an application name
// and affects only that application then the call belongs here.
package application

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/charmstore"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/relation"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/storage"
)

var logger = loggo.GetLogger("juju.api.application")

// Client allows access to the service API end point.
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
func (c *Client) SetMetricCredentials(service string, credentials []byte) error {
	creds := []params.ApplicationMetricCredential{
		{service, credentials},
	}
	p := params.ApplicationMetricCredentials{creds}
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

// DeployArgs holds the arguments to be sent to Client.ServiceDeploy.
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

	// Cons contains constraints on where units of this application
	// may be placed.
	Cons constraints.Value

	// Placement directives on where the machines for the unit must be
	// created.
	Placement []*instance.Placement

	// Storage contains Constraints specifying how storage should be
	// handled.
	Storage map[string]storage.Constraints

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

	// Development signals if the application is to be deployed in development mode.
	Development bool
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
			Constraints:      args.Cons,
			Placement:        args.Placement,
			Storage:          args.Storage,
			AttachStorage:    attachStorage,
			EndpointBindings: args.EndpointBindings,
			Resources:        args.Resources,
			Development:      args.Development,
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

// GetCharmURL returns the charm URL the given service is
// running at present.
func (c *Client) GetCharmURL(serviceName string) (*charm.URL, error) {
	result := new(params.StringResult)
	args := params.ApplicationGet{ApplicationName: serviceName}
	err := c.facade.FacadeCall("GetCharmURL", args, result)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if result.Error != nil {
		return nil, errors.Trace(result.Error)
	}
	return charm.ParseURL(result.Result)
}

// SetCharmConfig holds the configuration for setting a new revision of a charm
// on a service.
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
}

// SetCharm sets the charm for a given service.
func (c *Client) SetCharm(cfg SetCharmConfig) error {
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
		ForceSeries:        cfg.ForceSeries,
		ForceUnits:         cfg.ForceUnits,
		ResourceIDs:        cfg.ResourceIDs,
		StorageConstraints: storageConstraints,
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
	params := params.DestroyApplicationUnits{unitNames}
	return c.facade.FacadeCall("DestroyUnits", params, nil)
}

// DestroyUnits decreases the number of units dedicated to one or more
// applications.
func (c *Client) DestroyUnits(unitNames ...string) ([]params.DestroyUnitResult, error) {
	args := params.Entities{
		Entities: make([]params.Entity, 0, len(unitNames)),
	}
	allResults := make([]params.DestroyUnitResult, len(unitNames))
	index := make([]int, 0, len(unitNames))
	for i, name := range unitNames {
		if !names.IsValidUnit(name) {
			allResults[i].Error = &params.Error{
				Message: errors.NotValidf("unit ID %q", name).Error(),
			}
			continue
		}
		index = append(index, i)
		args.Entities = append(args.Entities, params.Entity{
			Tag: names.NewUnitTag(name).String(),
		})
	}
	if len(args.Entities) > 0 {
		var result params.DestroyUnitResults
		if err := c.facade.FacadeCall("DestroyUnit", args, &result); err != nil {
			return nil, errors.Trace(err)
		}
		if n := len(result.Results); n != len(args.Entities) {
			return nil, errors.Errorf("expected %d result(s), got %d", len(args.Entities), n)
		}
		for i, result := range result.Results {
			allResults[index[i]] = result
		}
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
	params := params.ApplicationDestroy{
		ApplicationName: application,
	}
	return c.facade.FacadeCall("Destroy", params, nil)
}

// DestroyApplications destroys the given applications.
func (c *Client) DestroyApplications(appNames ...string) ([]params.DestroyApplicationResult, error) {
	args := params.Entities{
		Entities: make([]params.Entity, 0, len(appNames)),
	}
	allResults := make([]params.DestroyApplicationResult, len(appNames))
	index := make([]int, 0, len(appNames))
	for i, name := range appNames {
		if !names.IsValidApplication(name) {
			allResults[i].Error = &params.Error{
				Message: errors.NotValidf("application name %q", name).Error(),
			}
			continue
		}
		index = append(index, i)
		args.Entities = append(args.Entities, params.Entity{
			Tag: names.NewApplicationTag(name).String(),
		})
	}
	if len(args.Entities) > 0 {
		var result params.DestroyApplicationResults
		if err := c.facade.FacadeCall("DestroyApplication", args, &result); err != nil {
			return nil, errors.Trace(err)
		}
		if n := len(result.Results); n != len(args.Entities) {
			return nil, errors.Errorf("expected %d result(s), got %d", len(args.Entities), n)
		}
		for i, result := range result.Results {
			allResults[index[i]] = result
		}
	}
	return allResults, nil
}

// GetConstraints returns the constraints for the given application.
func (c *Client) GetConstraints(service string) (constraints.Value, error) {
	results := new(params.GetConstraintsResults)
	err := c.facade.FacadeCall("GetConstraints", params.GetApplicationConstraints{service}, results)
	return results.Constraints, err
}

// SetConstraints specifies the constraints for the given application.
func (c *Client) SetConstraints(application string, constraints constraints.Value) error {
	params := params.SetConstraints{
		ApplicationName: application,
		Constraints:     constraints,
	}
	return c.facade.FacadeCall("SetConstraints", params, nil)
}

// Expose changes the juju-managed firewall to expose any ports that
// were also explicitly marked by units as open.
func (c *Client) Expose(application string) error {
	params := params.ApplicationExpose{ApplicationName: application}
	return c.facade.FacadeCall("Expose", params, nil)
}

// Unexpose changes the juju-managed firewall to unexpose any ports that
// were also explicitly marked by units as open.
func (c *Client) Unexpose(application string) error {
	params := params.ApplicationUnexpose{ApplicationName: application}
	return c.facade.FacadeCall("Unexpose", params, nil)
}

// Get returns the configuration for the named application.
func (c *Client) Get(application string) (*params.ApplicationGetResults, error) {
	var results params.ApplicationGetResults
	params := params.ApplicationGet{ApplicationName: application}
	err := c.facade.FacadeCall("Get", params, &results)
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
	params := params.ApplicationCharmRelations{ApplicationName: application}
	err := c.facade.FacadeCall("CharmRelations", params, &results)
	return results.CharmRelations, err
}

// AddRelation adds a relation between the specified endpoints and returns the relation info.
func (c *Client) AddRelation(endpoints, viaCIDRs []string) (*params.AddRelationResults, error) {
	var addRelRes params.AddRelationResults
	params := params.AddRelation{Endpoints: endpoints, ViaCIDRs: viaCIDRs}
	err := c.facade.FacadeCall("AddRelation", params, &addRelRes)
	return &addRelRes, err
}

// DestroyRelation removes the relation between the specified endpoints.
func (c *Client) DestroyRelation(endpoints ...string) error {
	params := params.DestroyRelation{Endpoints: endpoints}
	return c.facade.FacadeCall("DestroyRelation", params, nil)
}

// DestroyRelationId removes the relation with the specified id.
func (c *Client) DestroyRelationId(relationId int) error {
	params := params.DestroyRelation{RelationId: relationId}
	return c.facade.FacadeCall("DestroyRelation", params, nil)
}

// SetRelationStatus updates the status of the relation with the specified id.
func (c *Client) SetRelationStatus(relationId int, status relation.Status) error {
	args := params.RelationStatusArgs{
		Args: []params.RelationStatusArg{{RelationId: relationId, Status: params.RelationStatusValue(status)}},
	}
	var results params.ErrorResults
	if err := c.facade.FacadeCall("SetRelationStatus", args, &results); err != nil {
		return errors.Trace(err)
	}
	return results.OneError()
}

// Consume adds a remote application to the model.
func (c *Client) Consume(arg crossmodel.ConsumeApplicationArgs) (string, error) {
	var consumeRes params.ErrorResults
	args := params.ConsumeApplicationArgs{
		Args: []params.ConsumeApplicationArg{{
			ApplicationOffer: arg.ApplicationOffer,
			ApplicationAlias: arg.ApplicationAlias,
			Macaroon:         arg.Macaroon,
		}},
	}
	if arg.ControllerInfo != nil {
		args.Args[0].ControllerInfo = &params.ExternalControllerInfo{
			ControllerTag: arg.ControllerInfo.ControllerTag.String(),
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
	localName := arg.ApplicationOffer.OfferName
	if arg.ApplicationAlias != "" {
		localName = arg.ApplicationAlias
	}
	return localName, nil
}
