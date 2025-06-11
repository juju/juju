// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"context"
	"encoding/json"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/common"
	"github.com/juju/juju/api/common/cloudspec"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/internal/proxy"
	"github.com/juju/juju/rpc/params"
)

// Option is a function that can be used to configure a Client.
type Option = base.Option

// WithTracer returns an Option that configures the Client to use the
// supplied tracer.
var WithTracer = base.WithTracer

// Client provides methods that the Juju client command uses to interact
// with the Juju controller.
type Client struct {
	base.ClientFacade
	facade base.FacadeCaller
	*common.ControllerConfigAPI
	*common.ModelStatusAPI
	*cloudspec.CloudSpecAPI
}

// NewClient creates a new `Client` based on an existing authenticated API
// connection.
func NewClient(st base.APICallCloser, options ...Option) *Client {
	frontend, backend := base.NewClientFacade(st, "Controller", options...)
	return &Client{
		ClientFacade:        frontend,
		facade:              backend,
		ControllerConfigAPI: common.NewControllerConfig(backend),
		ModelStatusAPI:      common.NewModelStatusAPI(backend),
	}
}

// AllModels allows controller administrators to get the list of all the
// models in the controller.
func (c *Client) AllModels(ctx context.Context) ([]base.UserModel, error) {
	var models params.UserModelList
	err := c.facade.FacadeCall(ctx, "AllModels", nil, &models)
	if err != nil {
		return nil, errors.Trace(err)
	}
	result := make([]base.UserModel, len(models.UserModels))
	for i, usermodel := range models.UserModels {
		result[i] = base.UserModel{
			Name:           usermodel.Name,
			UUID:           usermodel.UUID,
			Type:           model.ModelType(usermodel.Type),
			Qualifier:      model.Qualifier(usermodel.Qualifier),
			LastConnection: usermodel.LastConnection,
		}
	}
	return result, nil
}

// CloudSpec returns a CloudSpec for the specified model.
func (c *Client) CloudSpec(ctx context.Context, modelTag names.ModelTag) (environscloudspec.CloudSpec, error) {
	api := cloudspec.NewCloudSpecAPI(c.facade, modelTag)
	return api.CloudSpec(ctx)
}

// HostedConfig contains the model config and the cloud spec for that
// model such that direct access to the provider can be used.
type HostedConfig struct {
	Name      string
	Qualifier string
	Config    map[string]interface{}
	CloudSpec environscloudspec.CloudSpec
	Error     error
}

// HostedModelConfigs returns all model settings for the
// models hosted on the controller.
func (c *Client) HostedModelConfigs(ctx context.Context) ([]HostedConfig, error) {
	result := params.HostedModelConfigsResults{}
	err := c.facade.FacadeCall(ctx, "HostedModelConfigs", nil, &result)
	if err != nil {
		return nil, errors.Trace(err)
	}
	// If we get to here, we have some values. Each value may or
	// may not have an error, but it should at least have a name
	// and owner.
	hostedConfigs := make([]HostedConfig, len(result.Models))
	for i, modelConfig := range result.Models {
		hostedConfigs[i].Name = modelConfig.Name
		hostedConfigs[i].Qualifier = modelConfig.Qualifier
		if modelConfig.Error != nil {
			hostedConfigs[i].Error = errors.Trace(modelConfig.Error)
			continue
		}
		hostedConfigs[i].Config = modelConfig.Config
		spec, err := c.MakeCloudSpec(modelConfig.CloudSpec)
		if err != nil {
			hostedConfigs[i].Error = errors.Trace(err)
			continue
		}
		hostedConfigs[i].CloudSpec = spec
	}
	return hostedConfigs, err
}

// DestroyControllerParams controls the behaviour of destroying the controller.
type DestroyControllerParams struct {
	// DestroyModels controls whether or not all hosted models should be
	// destroyed. If this is false, and there are non-empty hosted models,
	// an error with the code params.CodeHasHostedModels will be returned.
	DestroyModels bool

	// DestroyStorage controls whether or not storage in the model (and
	// hosted models, if DestroyModels is true) should be destroyed.
	//
	// This is ternary: nil, false, or true. If nil and there is persistent
	// storage in the model (or hosted models), an error with the code
	// params.CodeHasPersistentStorage will be returned.
	DestroyStorage *bool

	// Force specifies whether model destruction will be forced, i.e.
	// keep going despite operational errors.
	Force *bool `json:"force,omitempty"`

	// MaxWait specifies the amount of time that each step in model destroy process
	// will wait before forcing the next step to kick-off. This parameter
	// only makes sense in combination with 'force' set to 'true'.
	MaxWait *time.Duration `json:"max-wait,omitempty"`

	// ModelTimeout specifies how long to wait for the destroy process for each model.
	ModelTimeout *time.Duration `json:"model-timeout,omitempty"`
}

// DestroyController puts the controller model into a "dying" state,
// and removes all non-manager machine instances.
func (c *Client) DestroyController(ctx context.Context, args DestroyControllerParams) error {
	return c.facade.FacadeCall(ctx, "DestroyController", params.DestroyControllerArgs{
		DestroyModels:  args.DestroyModels,
		DestroyStorage: args.DestroyStorage,
		Force:          args.Force,
		MaxWait:        args.MaxWait,
		ModelTimeout:   args.ModelTimeout,
	}, nil)
}

// ListBlockedModels returns a list of all models within the controller
// which have at least one block in place.
func (c *Client) ListBlockedModels(ctx context.Context) ([]params.ModelBlockInfo, error) {
	result := params.ModelBlockInfoList{}
	err := c.facade.FacadeCall(ctx, "ListBlockedModels", nil, &result)
	info := make([]params.ModelBlockInfo, len(result.Models))
	for i, m := range result.Models {
		info[i] = params.ModelBlockInfo{
			UUID:      m.UUID,
			Name:      m.Name,
			Qualifier: m.Qualifier,
			Blocks:    m.Blocks,
		}
	}
	return info, err
}

// RemoveBlocks removes all the blocks in the controller.
func (c *Client) RemoveBlocks(ctx context.Context) error {
	args := params.RemoveBlocksArgs{All: true}
	return c.facade.FacadeCall(ctx, "RemoveBlocks", args, nil)
}

// WatchModelSummaries returns a SummaryWatcher, from which you can request
// the Next set of ModelAbstracts for all models the user can see.
func (c *Client) WatchModelSummaries(ctx context.Context) (*SummaryWatcher, error) {
	var info params.SummaryWatcherID
	if err := c.facade.FacadeCall(ctx, "WatchModelSummaries", nil, &info); err != nil {
		return nil, err
	}
	return NewSummaryWatcher(c.facade.RawAPICaller(), &info.WatcherID), nil
}

// WatchAllModelSummaries returns a SummaryWatcher, from which you can request
// the Next set of ModelAbstracts. This method is only valid for controller
// superusers and returns abstracts for all models in the controller.
func (c *Client) WatchAllModelSummaries(ctx context.Context) (*SummaryWatcher, error) {
	var info params.SummaryWatcherID
	if err := c.facade.FacadeCall(ctx, "WatchAllModelSummaries", nil, &info); err != nil {
		return nil, err
	}
	return NewSummaryWatcher(c.facade.RawAPICaller(), &info.WatcherID), nil
}

// GrantController grants a user access to the controller.
func (c *Client) GrantController(ctx context.Context, user, access string) error {
	return c.modifyControllerUser(ctx, params.GrantControllerAccess, user, access)
}

// RevokeController revokes a user's access to the controller.
func (c *Client) RevokeController(ctx context.Context, user, access string) error {
	return c.modifyControllerUser(ctx, params.RevokeControllerAccess, user, access)
}

func (c *Client) modifyControllerUser(ctx context.Context, action params.ControllerAction, user, access string) error {
	var args params.ModifyControllerAccessRequest

	if !names.IsValidUser(user) {
		return errors.Errorf("invalid username: %q", user)
	}
	userTag := names.NewUserTag(user)

	args.Changes = []params.ModifyControllerAccess{{
		UserTag: userTag.String(),
		Action:  action,
		Access:  access,
	}}

	var result params.ErrorResults
	err := c.facade.FacadeCall(ctx, "ModifyControllerAccess", args, &result)
	if err != nil {
		return errors.Trace(err)
	}
	if len(result.Results) != len(args.Changes) {
		return errors.Errorf("expected %d results, got %d", len(args.Changes), len(result.Results))
	}

	return result.Combine()
}

// GetControllerAccess returns the access level the user has on the controller.
func (c *Client) GetControllerAccess(ctx context.Context, user string) (permission.Access, error) {
	if !names.IsValidUser(user) {
		return "", errors.Errorf("invalid username: %q", user)
	}
	entities := params.Entities{Entities: []params.Entity{{Tag: names.NewUserTag(user).String()}}}
	var results params.UserAccessResults
	err := c.facade.FacadeCall(ctx, "GetControllerAccess", entities, &results)
	if err != nil {
		return "", errors.Trace(err)
	}
	if len(results.Results) != 1 {
		return "", errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	if err := results.Results[0].Error; err != nil {
		return "", errors.Trace(err)
	}
	return permission.Access(results.Results[0].Result.Access), nil
}

// ConfigSet updates the passed controller configuration values. Any
// settings that aren't passed will be left with their previous
// values.
func (c *Client) ConfigSet(ctx context.Context, values map[string]interface{}) error {
	return errors.Trace(
		c.facade.FacadeCall(ctx, "ConfigSet", params.ControllerConfigSet{Config: values}, nil),
	)
}

// MigrationSpec holds the details required to start the migration of
// a single model.
type MigrationSpec struct {
	ModelUUID             string
	TargetControllerUUID  string
	TargetControllerAlias string
	TargetAddrs           []string
	TargetCACert          string
	TargetUser            string
	TargetPassword        string
	TargetMacaroons       []macaroon.Slice
}

// Validate performs sanity checks on the migration configuration it
// holds.
func (s *MigrationSpec) Validate() error {
	if !names.IsValidModel(s.ModelUUID) {
		return errors.NotValidf("model UUID")
	}
	if !names.IsValidModel(s.TargetControllerUUID) {
		return errors.NotValidf("controller UUID")
	}
	if len(s.TargetAddrs) < 1 {
		return errors.NotValidf("empty target API addresses")
	}
	if !names.IsValidUser(s.TargetUser) {
		return errors.NotValidf("target user")
	}
	if s.TargetPassword == "" && len(s.TargetMacaroons) == 0 {
		return errors.NotValidf("missing authentication secrets")
	}
	return nil
}

// InitiateMigration attempts to start a migration for the specified
// model, returning the migration's ID.
//
// The API server supports starting multiple migrations in one request
// but we don't need that at the client side yet (and may never) so
// this call just supports starting one migration at a time.
func (c *Client) InitiateMigration(ctx context.Context, spec MigrationSpec) (string, error) {
	if err := spec.Validate(); err != nil {
		return "", errors.Annotatef(err, "client-side validation failed")
	}

	macsJSON, err := macaroonsToJSON(spec.TargetMacaroons)
	if err != nil {
		return "", errors.Annotatef(err, "client-side validation failed")
	}

	args := params.InitiateMigrationArgs{
		Specs: []params.MigrationSpec{{
			ModelTag: names.NewModelTag(spec.ModelUUID).String(),
			TargetInfo: params.MigrationTargetInfo{
				ControllerTag:   names.NewControllerTag(spec.TargetControllerUUID).String(),
				ControllerAlias: spec.TargetControllerAlias,
				Addrs:           spec.TargetAddrs,
				CACert:          spec.TargetCACert,
				AuthTag:         names.NewUserTag(spec.TargetUser).String(),
				Password:        spec.TargetPassword,
				Macaroons:       macsJSON,
			},
		}},
	}
	response := params.InitiateMigrationResults{}
	if err := c.facade.FacadeCall(ctx, "InitiateMigration", args, &response); err != nil {
		return "", errors.Trace(err)
	}
	if len(response.Results) != 1 {
		return "", errors.New("unexpected number of results returned")
	}
	result := response.Results[0]
	if result.Error != nil {
		return "", errors.Trace(result.Error)
	}
	return result.MigrationId, nil
}

func macaroonsToJSON(macs []macaroon.Slice) (string, error) {
	if len(macs) == 0 {
		return "", nil
	}
	out, err := json.Marshal(macs)
	if err != nil {
		return "", errors.Annotate(err, "marshalling macaroons")
	}
	return string(out), nil
}

type ControllerVersion struct {
	Version   string
	GitCommit string
}

// ControllerVersion fetches the controller version information.
func (c *Client) ControllerVersion(ctx context.Context) (ControllerVersion, error) {
	result := params.ControllerVersionResults{}
	err := c.facade.FacadeCall(ctx, "ControllerVersion", nil, &result)
	out := ControllerVersion{
		Version:   result.Version,
		GitCommit: result.GitCommit,
	}
	return out, err
}

// DashboardConnectionInfo
type DashboardConnectionInfo struct {
	Proxier   proxy.Proxier
	SSHTunnel *DashboardConnectionSSHTunnel
}

type DashboardConnectionSSHTunnel struct {
	Model  string
	Entity string
	Host   string
	Port   string
}

// ProxierFactory is an interface type representing a factory that can make a
// new juju proxier from the supplied raw config.
type ProxierFactory interface {
	ProxierFromConfig(string, map[string]interface{}) (proxy.Proxier, error)
}

// DashboardConnectionInfo fetches the connection information needed for
// connecting to the Juju Dashboard.
func (c *Client) DashboardConnectionInfo(ctx context.Context, factory ProxierFactory) (DashboardConnectionInfo, error) {
	rval := DashboardConnectionInfo{}
	result := params.DashboardConnectionInfo{}
	err := c.facade.FacadeCall(ctx, "DashboardConnectionInfo", nil, &result)
	if err != nil {
		return rval, errors.Trace(err)
	}

	if result.Error != nil {
		return rval, params.TranslateWellKnownError(result.Error)
	}

	if result.SSHConnection != nil {
		rval.SSHTunnel = &DashboardConnectionSSHTunnel{
			Model:  result.SSHConnection.Model,
			Entity: result.SSHConnection.Entity,
			Host:   result.SSHConnection.Host,
			Port:   result.SSHConnection.Port,
		}
		return rval, nil
	}

	proxier, err := factory.ProxierFromConfig(
		result.ProxyConnection.Type,
		result.ProxyConnection.Config)
	if err != nil {
		return rval, errors.Annotate(err, "creating proxier from config")
	}

	rval.Proxier = proxier
	return rval, nil
}
