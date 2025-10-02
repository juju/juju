// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"context"
	stderrors "errors"
	"fmt"
	"strings"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/api/base"
	apicharm "github.com/juju/juju/api/common/charm"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/devices"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/internal/charm"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/rpc/params"
)

// Option is a function that can be used to configure a Client.
type Option = base.Option

// WithTracer returns an Option that configures the Client to use the
// supplied tracer.
var WithTracer = base.WithTracer

var logger = internallogger.GetLogger("juju.api.application")

// Client allows access to the application API end point.
type Client struct {
	base.ClientFacade
	st     base.APICallCloser
	facade base.FacadeCaller
}

// NewClient creates a new client for accessing the application api.
func NewClient(st base.APICallCloser, options ...Option) *Client {
	frontend, backend := base.NewClientFacade(st, "Application", options...)
	return &Client{ClientFacade: frontend, st: st, facade: backend}
}

// ModelUUID returns the model UUID from the client connection.
func (c *Client) ModelUUID() string {
	tag, ok := c.st.ModelTag()
	if !ok {
		logger.Warningf(context.TODO(), "controller-only API connection has no model tag")
	}
	return tag.Id()
}

// DeployArgs holds the arguments to be sent to Client.ApplicationDeploy.
type DeployArgs struct {
	// CharmID identifies the charm to deploy.
	CharmID CharmID

	// CharmOrigin holds information about where the charm originally came from,
	// this includes the store.
	CharmOrigin apicharm.Origin

	// ApplicationName is the name to give the application.
	ApplicationName string

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

	// Storage contains Directives specifying how storage should be
	// handled.
	Storage map[string]storage.Directive

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

	// Force can be set to true to bypass any checks for charm-specific
	// requirements ("assumes" sections in charm metadata)
	Force bool
}

// Leader returns the unit name for the leader of the provided application.
func (c *Client) Leader(ctx context.Context, app string) (string, error) {
	var result params.StringResult
	p := params.Entity{Tag: names.NewApplicationTag(app).String()}

	if err := c.facade.FacadeCall(ctx, "Leader", p, &result); err != nil {
		return "", errors.Trace(err)
	}
	if result.Error != nil {
		return "", result.Error
	}
	return result.Result, nil
}

// Deploy obtains the charm, either locally or from the charm store, and deploys
// it. Placement directives, if provided, specify the machine on which the charm
// is deployed.
func (c *Client) Deploy(ctx context.Context, args DeployArgs) error {
	if len(args.AttachStorage) > 0 {
		if args.NumUnits != 1 {
			return errors.New("cannot attach existing storage when more than one unit is requested")
		}
	}
	attachStorage := make([]string, len(args.AttachStorage))
	for i, id := range args.AttachStorage {
		if !names.IsValidStorage(id) {
			return errors.NotValidf("storage ID %q", id)
		}
		attachStorage[i] = names.NewStorageTag(id).String()
	}
	origin := args.CharmOrigin.ParamsCharmOrigin()
	deployArgs := params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			ApplicationName:  args.ApplicationName,
			CharmURL:         args.CharmID.URL,
			CharmOrigin:      &origin,
			Channel:          origin.Risk,
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
			Force:            args.Force,
		}},
	}
	var results params.ErrorResults
	var err error
	err = c.facade.FacadeCall(ctx, "Deploy", deployArgs, &results)
	if err != nil {
		return errors.Trace(err)
	}
	err = results.OneError()
	if err == nil {
		return nil
	}
	if pErr, ok := errors.Cause(err).(*params.Error); ok {
		switch pErr.Code {
		case params.CodeAlreadyExists:
			// Remove the "already exists" in the error message to prevent
			// stuttering.
			msg := strings.Replace(err.Error(), " already exists", "", -1)
			return errors.AlreadyExistsf(msg)
		}
	}
	return errors.Trace(err)
}

// GetCharmURLOrigin returns the charm URL along with the charm Origin for the
// given application is running at present.
// The charm origin gives more information about the location of the charm and
// what revision/channel it came from.
func (c *Client) GetCharmURLOrigin(ctx context.Context, applicationName string) (*charm.URL, apicharm.Origin, error) {
	args := params.ApplicationGet{
		ApplicationName: applicationName,
	}

	var result params.CharmURLOriginResult
	err := c.facade.FacadeCall(ctx, "GetCharmURLOrigin", args, &result)
	if err != nil {
		return nil, apicharm.Origin{}, errors.Trace(err)
	}
	if result.Error != nil {
		return nil, apicharm.Origin{}, errors.Trace(result.Error)
	}
	curl, err := charm.ParseURL(result.URL)
	if err != nil {
		return nil, apicharm.Origin{}, errors.Trace(err)
	}
	origin, err := apicharm.APICharmOrigin(result.Origin)
	return curl, origin, err
}

// GetConfig returns the charm configuration settings for each of the
// applications. If any of the applications are not found, an error is
// returned.
func (c *Client) GetConfig(ctx context.Context, appNames ...string) ([]map[string]interface{}, error) {
	arg := params.ApplicationGetArgs{Args: make([]params.ApplicationGet, len(appNames))}
	for i, appName := range appNames {
		arg.Args[i] = params.ApplicationGet{ApplicationName: appName}
	}

	var results params.ApplicationGetConfigResults
	err := c.facade.FacadeCall(ctx, "CharmConfig", arg, &results)
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

// CharmID represents the underlying charm for a given application. This
// includes both the URL and the origin.
type CharmID struct {

	// URL of the given charm, includes the reference name and a revision.
	URL string

	// Origin holds the origin of a charm. This includes the source of the
	// charm, along with the revision and channel to identify where the charm
	// originated from.
	Origin apicharm.Origin
}

// SetCharmConfig holds the configuration for setting a new revision of a charm
// on a application.
type SetCharmConfig struct {
	// ApplicationName is the name of the application to set the charm on.
	ApplicationName string

	// CharmID identifies the charm.
	CharmID CharmID

	// ConfigSettings is the charm settings to set during the upgrade.
	// This field is only understood by Application facade version 2
	// and greater.
	ConfigSettings map[string]string

	// ConfigSettingsYAML is the charm settings in YAML format to set
	// during the upgrade. If this is non-empty, it will take precedence
	// over ConfigSettings. This field is only understood by Application
	// facade version 2
	ConfigSettingsYAML string

	// Force forces the use of the charm in the following scenarios:
	// overriding a lxd profile upgrade.
	// In the future, we should deprecate ForceBase and ForceUnits and just
	// use Force for all instances.
	// TODO (stickupkid): deprecate ForceBase and ForceUnits in favour of
	// just using Force.
	Force bool

	// ForceBase forces the use of the charm even if it doesn't match the
	// series of the unit.
	ForceBase bool

	// ForceUnits forces the upgrade on units in an error state.
	ForceUnits bool

	// ResourceIDs is a map of resource names to resource IDs to activate during
	// the upgrade.
	ResourceIDs map[string]string

	// StorageDirectives is a map of storage names to storage directives to
	// update during the upgrade. This field is only understood by Application
	// facade version 2 and greater.
	StorageDirectives map[string]storage.Directive

	// EndpointBindings is a map of operator-defined endpoint names to
	// space names to be merged with any existing endpoint bindings.
	EndpointBindings map[string]string
}

// SetCharm sets the charm for a given application.
func (c *Client) SetCharm(ctx context.Context, cfg SetCharmConfig) error {
	var storageDirectives map[string]params.StorageDirectives
	if len(cfg.StorageDirectives) > 0 {
		storageDirectives = make(map[string]params.StorageDirectives)
		for name, directive := range cfg.StorageDirectives {
			size, count := directive.Size, directive.Count
			var sizePtr, countPtr *uint64
			if size > 0 {
				sizePtr = &size
			}
			if count > 0 {
				countPtr = &count
			}
			storageDirectives[name] = params.StorageDirectives{
				Pool:    directive.Pool,
				SizeMiB: sizePtr,
				Count:   countPtr,
			}
		}
	}

	origin := cfg.CharmID.Origin
	paramsCharmOrigin := origin.ParamsCharmOrigin()

	args := params.ApplicationSetCharmV2{
		ApplicationName:    cfg.ApplicationName,
		CharmURL:           cfg.CharmID.URL,
		CharmOrigin:        &paramsCharmOrigin,
		Channel:            origin.Risk,
		ConfigSettings:     cfg.ConfigSettings,
		ConfigSettingsYAML: cfg.ConfigSettingsYAML,
		Force:              cfg.Force,
		ForceBase:          cfg.ForceBase,
		ForceUnits:         cfg.ForceUnits,
		ResourceIDs:        cfg.ResourceIDs,
		StorageDirectives:  storageDirectives,
		EndpointBindings:   cfg.EndpointBindings,
	}
	return c.facade.FacadeCall(ctx, "SetCharm", args, nil)
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
func (c *Client) AddUnits(ctx context.Context, args AddUnitsParams) ([]string, error) {
	if len(args.AttachStorage) > 0 {
		if args.NumUnits != 1 {
			return nil, errors.New("cannot attach existing storage when more than one unit is requested")
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
	err := c.facade.FacadeCall(ctx, "AddUnits", params.AddApplicationUnits{
		ApplicationName: args.ApplicationName,
		NumUnits:        args.NumUnits,
		Placement:       args.Placement,
		Policy:          args.Policy,
		AttachStorage:   attachStorage,
	}, results)
	return results.Units, err
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

	// DryRun specifies whether to perform the destroy action or
	// just return what this action will destroy
	DryRun bool
}

// DestroyUnits decreases the number of units dedicated to one or more
// applications.
func (c *Client) DestroyUnits(ctx context.Context, in DestroyUnitsParams) ([]params.DestroyUnitResult, error) {
	args := params.DestroyUnitsParams{
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
		args.Units = append(args.Units, params.DestroyUnitParams{
			UnitTag:        names.NewUnitTag(name).String(),
			DestroyStorage: in.DestroyStorage,
			Force:          in.Force,
			MaxWait:        in.MaxWait,
			DryRun:         in.DryRun,
		})
	}
	if len(args.Units) == 0 {
		return allResults, nil
	}

	var result params.DestroyUnitResults
	if err := c.facade.FacadeCall(ctx, "DestroyUnit", args, &result); err != nil {
		return nil, errors.Trace(err)
	}
	if n := len(result.Results); n != len(args.Units) {
		return nil, errors.Errorf("expected %d result(s), got %d", len(args.Units), n)
	}
	for i, result := range result.Results {
		allResults[index[i]] = result
	}
	return allResults, nil
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

	// DryRun specifies whether this should perform this destroy
	// action or just return what this action will destroy
	DryRun bool
}

// DestroyApplications destroys the given applications.
func (c *Client) DestroyApplications(ctx context.Context, in DestroyApplicationsParams) ([]params.DestroyApplicationResult, error) {
	args := params.DestroyApplicationsParams{
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
		args.Applications = append(args.Applications, params.DestroyApplicationParams{
			ApplicationTag: names.NewApplicationTag(name).String(),
			DestroyStorage: in.DestroyStorage,
			Force:          in.Force,
			MaxWait:        in.MaxWait,
			DryRun:         in.DryRun,
		})
	}
	if len(args.Applications) == 0 {
		return allResults, nil
	}

	var result params.DestroyApplicationResults
	if err := c.facade.FacadeCall(ctx, "DestroyApplication", args, &result); err != nil {
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
func (c *Client) DestroyConsumedApplication(ctx context.Context, in DestroyConsumedApplicationParams) ([]params.ErrorResult, error) {
	args := params.DestroyConsumedApplicationsParams{
		Applications: make([]params.DestroyConsumedApplicationParams, 0, len(in.SaasNames)),
	}

	if in.MaxWait != nil && !in.Force {
		return nil, errors.New("--force is required when --max-wait is provided")
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
		appParams.Force = &in.Force
		appParams.MaxWait = in.MaxWait
		args.Applications = append(args.Applications, appParams)
	}

	var result params.ErrorResults
	if err := c.facade.FacadeCall(ctx, "DestroyConsumedApplications", args, &result); err != nil {
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

	// AttachStorage contains IDs of existing storage that should be
	// attached to the application unit that will be deployed. This
	// may be non-empty only if NumUnits is 1.
	AttachStorage []string
}

// ScaleApplication sets the desired unit count for one or more applications.
func (c *Client) ScaleApplication(ctx context.Context, in ScaleApplicationParams) (params.ScaleApplicationResult, error) {
	if len(in.AttachStorage) > 0 && c.BestAPIVersion() < 21 {
		// ScaleApplication with attach storage was introduced in ApplicationAPIV21.
		return params.ScaleApplicationResult{}, errors.NotImplementedf("scale application with attach storage on this version of Juju")
	}
	if len(in.AttachStorage) > 0 && in.ScaleChange != 1 {
		return params.ScaleApplicationResult{}, errors.New("cannot attach existing storage when more than one unit is requested")
	}

	if !names.IsValidApplication(in.ApplicationName) {
		return params.ScaleApplicationResult{}, errors.NotValidf("application %q", in.ApplicationName)
	}

	if err := validateApplicationScale(in.Scale, in.ScaleChange); err != nil {
		return params.ScaleApplicationResult{}, errors.Trace(err)
	}

	var results params.ScaleApplicationResults
	if c.BestAPIVersion() < 21 {
		args := params.ScaleApplicationsParams{
			Applications: []params.ScaleApplicationParams{{
				ApplicationTag: names.NewApplicationTag(in.ApplicationName).String(),
				Scale:          in.Scale,
				ScaleChange:    in.ScaleChange,
				Force:          in.Force,
			}},
		}
		if err := c.facade.FacadeCall(ctx, "ScaleApplications", args, &results); err != nil {
			return params.ScaleApplicationResult{}, errors.Trace(err)
		}
	} else {
		args := params.ScaleApplicationsParamsV2{
			Applications: []params.ScaleApplicationParamsV2{{
				ApplicationTag: names.NewApplicationTag(in.ApplicationName).String(),
				Scale:          in.Scale,
				ScaleChange:    in.ScaleChange,
				Force:          in.Force,
				AttachStorage:  in.AttachStorage,
			}},
		}
		if err := c.facade.FacadeCall(ctx, "ScaleApplications", args, &results); err != nil {
			return params.ScaleApplicationResult{}, errors.Trace(err)
		}
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
func (c *Client) GetConstraints(ctx context.Context, applications ...string) ([]constraints.Value, error) {
	var allConstraints []constraints.Value

	// Make a single call to get all the constraints.
	var results params.ApplicationGetConstraintsResults
	var args params.Entities
	for _, application := range applications {
		args.Entities = append(args.Entities,
			params.Entity{Tag: names.NewApplicationTag(application).String()})
	}
	err := c.facade.FacadeCall(ctx, "GetConstraints", args, &results)
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
func (c *Client) SetConstraints(ctx context.Context, application string, constraints constraints.Value) error {
	args := params.SetConstraints{
		ApplicationName: application,
		Constraints:     constraints,
	}
	return c.facade.FacadeCall(ctx, "SetConstraints", args, nil)
}

// Expose changes the juju-managed firewall to expose any ports that
// were also explicitly marked by units as open. The exposedEndpoints argument
// can be used to restrict the set of ports that get exposed and at the same
// time specify which spaces and/or CIDRs should be able to access these ports
// on a per endpoint basis.
//
// If the exposedEndpoints parameter is empty, the controller will expose *all*
// open ports of the application to 0.0.0.0/0. This matches the behavior of
// pre-2.9 juju controllers.
func (c *Client) Expose(ctx context.Context, application string, exposedEndpoints map[string]params.ExposedEndpoint) error {
	args := params.ApplicationExpose{
		ApplicationName:  application,
		ExposedEndpoints: exposedEndpoints,
	}
	return c.facade.FacadeCall(ctx, "Expose", args, nil)
}

// Unexpose changes the juju-managed firewall to unexpose any ports that
// were also explicitly marked by units as open.
func (c *Client) Unexpose(ctx context.Context, application string, endpoints []string) error {
	args := params.ApplicationUnexpose{
		ApplicationName:  application,
		ExposedEndpoints: endpoints,
	}
	return c.facade.FacadeCall(ctx, "Unexpose", args, nil)
}

// Get returns the configuration for the named application.
func (c *Client) Get(ctx context.Context, application string) (*params.ApplicationGetResults, error) {
	var results params.ApplicationGetResults
	args := params.ApplicationGet{
		ApplicationName: application,
	}
	err := c.facade.FacadeCall(ctx, "Get", args, &results)
	return &results, err
}

// CharmRelations returns the application's charms relation names.
func (c *Client) CharmRelations(ctx context.Context, application string) ([]string, error) {
	var results params.ApplicationCharmRelationsResults
	args := params.ApplicationCharmRelations{ApplicationName: application}
	err := c.facade.FacadeCall(ctx, "CharmRelations", args, &results)
	return results.CharmRelations, err
}

// AddRelation adds a relation between the specified endpoints and returns the relation info.
func (c *Client) AddRelation(ctx context.Context, endpoints, viaCIDRs []string) (*params.AddRelationResults, error) {
	var addRelRes params.AddRelationResults
	args := params.AddRelation{Endpoints: endpoints, ViaCIDRs: viaCIDRs}
	err := c.facade.FacadeCall(ctx, "AddRelation", args, &addRelRes)
	return &addRelRes, err
}

// DestroyRelation removes the relation between the specified endpoints.
func (c *Client) DestroyRelation(ctx context.Context, force *bool, maxWait *time.Duration, endpoints ...string) error {
	args := params.DestroyRelation{
		Endpoints: endpoints,
		Force:     force,
		MaxWait:   maxWait,
	}
	return c.facade.FacadeCall(ctx, "DestroyRelation", args, nil)
}

// DestroyRelationId removes the relation with the specified id.
func (c *Client) DestroyRelationId(ctx context.Context, relationId int, force *bool, maxWait *time.Duration) error {
	args := params.DestroyRelation{
		RelationId: relationId,
		Force:      force,
		MaxWait:    maxWait,
	}
	return c.facade.FacadeCall(ctx, "DestroyRelation", args, nil)
}

// SetRelationSuspended updates the suspended status of the relation with the specified id.
func (c *Client) SetRelationSuspended(ctx context.Context, relationIds []int, suspended bool, message string) error {
	var args params.RelationSuspendedArgs
	for _, relId := range relationIds {
		args.Args = append(args.Args, params.RelationSuspendedArg{
			RelationId: relId,
			Suspended:  suspended,
			Message:    message,
		})
	}
	var results params.ErrorResults
	if err := c.facade.FacadeCall(ctx, "SetRelationsSuspended", args, &results); err != nil {
		return errors.Trace(err)
	}
	if len(results.Results) != len(args.Args) {
		return errors.Errorf("expected %d results, got %d", len(args.Args), len(results.Results))
	}
	var errorStrings []string
	for i, r := range results.Results {
		if r.Error != nil {
			if r.Error.Code == params.CodeUnauthorized {
				errorStrings = append(errorStrings, fmt.Sprintf("cannot resume relation %d without consume permission", relationIds[i]))
			} else {
				errorStrings = append(errorStrings, r.Error.Error())
			}
		}
	}
	if errorStrings != nil {
		return errors.New(strings.Join(errorStrings, "\n"))
	}
	return nil
}

// Consume adds a remote application to the model.
func (c *Client) Consume(ctx context.Context, arg crossmodel.ConsumeApplicationArgs) (string, error) {
	var consumeRes params.ErrorResults
	args := params.ConsumeApplicationArgsV5{
		Args: []params.ConsumeApplicationArgV5{{
			ApplicationOfferDetailsV5: arg.Offer,
			ApplicationAlias:          arg.ApplicationAlias,
			Macaroon:                  arg.Macaroon,
		}},
	}
	if arg.ControllerInfo != nil {
		args.Args[0].ControllerInfo = &params.ExternalControllerInfo{
			ControllerTag: names.NewControllerTag(arg.ControllerInfo.ControllerUUID).String(),
			Alias:         arg.ControllerInfo.Alias,
			Addrs:         arg.ControllerInfo.Addrs,
			CACert:        arg.ControllerInfo.CACert,
		}
	}
	err := c.facade.FacadeCall(ctx, "Consume", args, &consumeRes)
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

// SetConfig sets configuration options on an application and the charm.
func (c *Client) SetConfig(ctx context.Context, application, configYAML string, config map[string]string) error {
	args := params.ConfigSetArgs{
		Args: []params.ConfigSet{{
			ApplicationName: application,
			Config:          config,
			ConfigYAML:      configYAML,
		}},
	}
	var results params.ErrorResults
	err := c.facade.FacadeCall(ctx, "SetConfigs", args, &results)
	if err != nil {
		return errors.Trace(err)
	}
	return results.OneError()
}

// UnsetApplicationConfig resets configuration options on an application.
func (c *Client) UnsetApplicationConfig(ctx context.Context, application string, options []string) error {
	args := params.ApplicationConfigUnsetArgs{
		Args: []params.ApplicationUnset{{
			ApplicationName: application,
			Options:         options,
		}},
	}
	var results params.ErrorResults
	err := c.facade.FacadeCall(ctx, "UnsetApplicationsConfig", args, &results)
	if err != nil {
		return errors.Trace(err)
	}
	return results.OneError()
}

// ResolveUnitErrors clears errors on one or more units.
// Either specify one or more units, or all.
func (c *Client) ResolveUnitErrors(ctx context.Context, units []string, all, retry bool) error {
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
	err := c.facade.FacadeCall(ctx, "ResolveUnitErrors", args, results)
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
func (c *Client) ApplicationsInfo(ctx context.Context, applications []names.ApplicationTag) ([]params.ApplicationInfoResult, error) {
	all := make([]params.Entity, len(applications))
	for i, one := range applications {
		all[i] = params.Entity{Tag: one.String()}
	}
	in := params.Entities{Entities: all}
	var out params.ApplicationInfoResults
	err := c.facade.FacadeCall(ctx, "ApplicationsInfo", in, &out)
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
func (c *Client) MergeBindings(ctx context.Context, req params.ApplicationMergeBindingsArgs) error {
	var results params.ErrorResults
	err := c.facade.FacadeCall(ctx, "MergeBindings", req, &results)
	if err != nil {
		return errors.Trace(err)
	}
	return results.OneError()
}

// UnitInfo holds information about a unit.
type UnitInfo struct {
	Error error

	Tag             string
	WorkloadVersion string
	Machine         string
	OpenedPorts     []string
	PublicAddress   string
	Charm           string
	Leader          bool
	Life            string
	RelationData    []EndpointRelationData

	// The following are for CAAS models.
	ProviderId string
	Address    string
}

// RelationData holds information about a unit's relation.
type RelationData struct {
	InScope  bool
	UnitData map[string]interface{}
}

// EndpointRelationData holds information about a relation to a given endpoint.
type EndpointRelationData struct {
	RelationId       int
	Endpoint         string
	CrossModel       bool
	RelatedEndpoint  string
	ApplicationData  map[string]interface{}
	UnitRelationData map[string]RelationData
}

// UnitsInfo retrieves units information.
func (c *Client) UnitsInfo(ctx context.Context, units []names.UnitTag) ([]UnitInfo, error) {
	all := make([]params.Entity, len(units))
	for i, one := range units {
		all[i] = params.Entity{Tag: one.String()}
	}
	in := params.Entities{Entities: all}
	var out params.UnitInfoResults
	err := c.facade.FacadeCall(ctx, "UnitsInfo", in, &out)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if resultsLen := len(out.Results); resultsLen != len(units) {
		return nil, errors.Errorf("expected %d results, got %d", len(units), resultsLen)
	}
	infos := make([]UnitInfo, len(out.Results))
	for i, r := range out.Results {
		infos[i] = unitInfoFromParams(r)
	}
	return infos, nil
}

func unitInfoFromParams(in params.UnitInfoResult) UnitInfo {
	if in.Error != nil {
		return UnitInfo{Error: stderrors.New(in.Error.Error())}
	}
	info := UnitInfo{
		Tag:             in.Result.Tag,
		WorkloadVersion: in.Result.WorkloadVersion,
		Machine:         in.Result.Machine,
		PublicAddress:   in.Result.PublicAddress,
		Charm:           in.Result.Charm,
		Leader:          in.Result.Leader,
		Life:            in.Result.Life,
		ProviderId:      in.Result.ProviderId,
		Address:         in.Result.Address,
	}
	for _, p := range in.Result.OpenedPorts {
		info.OpenedPorts = append(info.OpenedPorts, p)
	}
	for _, inRd := range in.Result.RelationData {
		erd := EndpointRelationData{
			RelationId:      inRd.RelationId,
			Endpoint:        inRd.Endpoint,
			CrossModel:      inRd.CrossModel,
			RelatedEndpoint: inRd.RelatedEndpoint,
		}
		if len(inRd.ApplicationData) > 0 {
			erd.ApplicationData = make(map[string]interface{})
			for k, v := range inRd.ApplicationData {
				erd.ApplicationData[k] = v
			}
		}
		if len(inRd.UnitRelationData) > 0 {
			erd.UnitRelationData = make(map[string]RelationData)
			for unit, inUrd := range inRd.UnitRelationData {
				urd := RelationData{
					InScope: inUrd.InScope,
				}
				if len(inUrd.UnitData) > 0 {
					urd.UnitData = make(map[string]interface{})
					for k, v := range inUrd.UnitData {
						urd.UnitData[k] = v
					}
				}
				erd.UnitRelationData[unit] = urd
			}
		}
		info.RelationData = append(info.RelationData, erd)
	}
	return info
}

type DeployInfo struct {
	// Architecture is the architecture used to deploy the charm.
	Architecture string
	// Base is the base used to deploy the charm.
	Base corebase.Base
	// Channel is a string representation of the channel used to
	// deploy the charm.
	Channel string
	// EffectiveChannel is the channel actually deployed from as determined
	// by the charmhub response.
	EffectiveChannel *string
	// Is the name of the application deployed. This may vary from
	// the charm name provided if differs in the metadata.yaml and
	// no provided on the cli.
	Name string
	// Revision is the revision of the charm deployed.
	Revision int
}

type PendingResourceUpload struct {
	// Name is the name of the resource.
	Name string
	// Filename is the name of the file as it exists on disk.
	// Sometimes referred to as the path.
	Filename string
	// Type of the resource, a string matching one of the resource.Type
	Type string
}

type DeployFromRepositoryArg struct {
	// CharmName is a string identifying the name of the thing to deploy.
	// Required.
	CharmName string
	// ApplicationName is the name to give the application. Optional. By
	// default, the charm name and the application name will be the same.
	ApplicationName string
	// AttachStorage contains IDs of existing storage that should be
	// attached to the application unit that will be deployed. This
	// may be non-empty only if NumUnits is 1.
	AttachStorage []string
	// Base describes the OS base intended to be used by the charm.
	Base *corebase.Base
	// Channel is the channel in the repository to deploy from.
	// This is an optional value. Required if revision is provided.
	// Defaults to “stable” if not defined nor required.
	Channel *string
	// ConfigYAML is a string that overrides the default config.yml.
	ConfigYAML string
	// Cons contains constraints on where units of this application
	// may be placed.
	Cons constraints.Value
	// Devices contains Constraints specifying how devices should be
	// handled.
	Devices map[string]devices.Constraints
	// DryRun just shows what the deploy would do, including finding the
	// charm; determining version, channel and base to use; validation
	// of the config. Does not actually download or deploy the charm.
	DryRun bool
	// EndpointBindings
	EndpointBindings map[string]string
	// Force can be set to true to bypass any checks for charm-specific
	// requirements ("assumes" sections in charm metadata, supported series,
	// LXD profile allow list)
	Force bool
	// NumUnits is the number of units to deploy. Defaults to 1 if no
	// value provided. Synonymous with scale for kubernetes charms.
	NumUnits *int
	// Placement directives define on which machines the unit(s) must be
	// created.
	Placement []*instance.Placement
	// Revision is the charm revision number. Requires the channel
	// be explicitly set.
	Revision *int
	// Resources is a collection of resource names for the
	// application, with the value being the revision of the
	// resource to use if default revision is not desired.
	Resources map[string]string
	// Storage contains Directives specifying how storage should be
	// handled.
	Storage map[string]storage.Directive
	//  Trust allows charm to run hooks that require access credentials
	Trust bool
}

// DeployFromRepository deploys a charm from a repository based on the
// provided arguments. Returned in info on application was deployed,
// data required to upload any local resources and errors returned.
// Where possible, more than all errors regarding argument validation
// are returned.
func (c *Client) DeployFromRepository(ctx context.Context, arg DeployFromRepositoryArg) (DeployInfo, []PendingResourceUpload, []error) {
	var result params.DeployFromRepositoryResults
	args := params.DeployFromRepositoryArgs{
		Args: []params.DeployFromRepositoryArg{paramsFromDeployFromRepositoryArg(arg)},
	}
	err := c.facade.FacadeCall(ctx, "DeployFromRepository", args, &result)
	if err != nil {
		return DeployInfo{}, nil, []error{err}
	}
	if len(result.Results) != 1 {
		return DeployInfo{}, nil, []error{fmt.Errorf("expected 1 result, got %d", len(result.Results))}
	}
	deployInfo, err := deployInfoFromParams(result.Results[0].Info)
	pendingResourceUploads := pendingResourceUploadsFromParams(result.Results[0].PendingResourceUploads)
	var errs []error
	if err != nil {
		errs = append(errs, err)
	}
	if len(result.Results[0].Errors) > 0 {
		errs = make([]error, len(result.Results[0].Errors))
		for i, er := range result.Results[0].Errors {
			errs[i] = errors.New(er.Error())
		}
	}
	return deployInfo, pendingResourceUploads, errs
}

func deployInfoFromParams(di params.DeployFromRepositoryInfo) (DeployInfo, error) {
	base, err := corebase.ParseBase(di.Base.Name, di.Base.Channel)
	return DeployInfo{
		Architecture:     di.Architecture,
		Base:             base,
		Channel:          di.Channel,
		EffectiveChannel: di.EffectiveChannel,
		Name:             di.Name,
		Revision:         di.Revision,
	}, err
}

func pendingResourceUploadsFromParams(uploads []*params.PendingResourceUpload) []PendingResourceUpload {
	if len(uploads) == 0 {
		return []PendingResourceUpload{}
	}
	prus := make([]PendingResourceUpload, len(uploads))
	for i, upload := range uploads {
		prus[i] = PendingResourceUpload{
			Name:     upload.Name,
			Filename: upload.Filename,
			Type:     upload.Type,
		}
	}
	return prus
}

func paramsFromDeployFromRepositoryArg(arg DeployFromRepositoryArg) params.DeployFromRepositoryArg {
	var b *params.Base
	if arg.Base != nil {
		b = &params.Base{
			Name:    arg.Base.OS,
			Channel: arg.Base.Channel.String(),
		}
	}
	return params.DeployFromRepositoryArg{
		CharmName:        arg.CharmName,
		ApplicationName:  arg.ApplicationName,
		AttachStorage:    arg.AttachStorage,
		Base:             b,
		Channel:          arg.Channel,
		ConfigYAML:       arg.ConfigYAML,
		Cons:             arg.Cons,
		Devices:          arg.Devices,
		DryRun:           arg.DryRun,
		EndpointBindings: arg.EndpointBindings,
		Force:            arg.Force,
		NumUnits:         arg.NumUnits,
		Placement:        arg.Placement,
		Revision:         arg.Revision,
		Resources:        arg.Resources,
		Storage:          arg.Storage,
		Trust:            arg.Trust,
	}
}

// ApplicationStorageInfo contains the storage information for an application.
type ApplicationStorageInfo struct {
	Error error
	// StorageConstraints is a map of storage names to storage constraints to
	// update during the upgrade. This field is only understood by Application
	// facade version 2 and greater.
	StorageConstraints map[string]storage.Directive `json:"storage-constraints,omitempty"`
}

// GetApplicationStorage retrieves storage information for the specified applications.
func (c *Client) GetApplicationStorage(applicationName string) (ApplicationStorageInfo, error) {
	application := names.NewApplicationTag(applicationName)
	in := params.Entities{Entities: []params.Entity{{Tag: application.String()}}}
	var out params.ApplicationStorageGetResults
	err := c.facade.FacadeCall(context.Background(), "GetApplicationStorage", in, &out)
	if err != nil {
		return ApplicationStorageInfo{}, errors.Trace(err)
	}
	if resultsLen := len(out.Results); resultsLen != 1 {
		return ApplicationStorageInfo{}, errors.Errorf("expected 1 result, got %d", resultsLen)
	}
	return applicationInfoFromParams(out.Results[0]), nil
}

func applicationInfoFromParams(param params.ApplicationStorageGetResult) ApplicationStorageInfo {
	if param.Error != nil {
		return ApplicationStorageInfo{
			Error: apiservererrors.RestoreError(param.Error),
		}
	}
	sc := make(map[string]storage.Directive)
	for k, v := range param.StorageConstraints {
		con := storage.Directive{
			Pool: v.Pool,
		}
		if v.SizeMiB != nil {
			con.Size = *v.SizeMiB
		}
		if v.Count != nil {
			con.Count = *v.Count
		}
		sc[k] = con
	}

	return ApplicationStorageInfo{
		StorageConstraints: sc,
	}
}

// ApplicationStorageUpdate holds the desired storage constraint updates for a single application.
type ApplicationStorageUpdate struct {
	ApplicationTag names.ApplicationTag

	// StorageConstraints is a map of storage names to storage constraints to
	// update during the upgrade. This field is only understood by Application
	// facade version 2 and greater.
	StorageConstraints map[string]storage.Directive `json:"storage-constraints,omitempty"`
}

// UpdateApplicationStorage updates the storage constraints for multiple existing applications in bulk.
func (c *Client) UpdateApplicationStorage(applicationStorageUpdate ApplicationStorageUpdate) error {
	sc := make(map[string]params.StorageDirectives)
	for k, v := range applicationStorageUpdate.StorageConstraints {
		sc[k] = params.StorageDirectives{
			Pool:    v.Pool,
			SizeMiB: &v.Size,
			Count:   &v.Count,
		}
	}
	in := params.ApplicationStorageUpdateRequest{
		ApplicationStorageUpdates: []params.ApplicationStorageUpdate{
			{
				ApplicationTag:     applicationStorageUpdate.ApplicationTag.String(),
				StorageConstraints: sc,
			},
		},
	}

	var out params.ErrorResults
	err := c.facade.FacadeCall(context.Background(), "UpdateApplicationStorage", in, &out)
	if err != nil {
		return errors.Trace(err)
	}
	if resultsLen := len(out.Results); resultsLen != 1 {
		return errors.Errorf("expected 1 result, got %d", resultsLen)
	}
	if err := out.Results[0].Error; err != nil {
		return err
	}
	return nil
}
