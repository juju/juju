// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelconfig

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/rpc/params"
)

// Option is a function that can be used to configure a Client.
type Option = base.Option

// WithTracer returns an Option that configures the Client to use the
// supplied tracer.
var WithTracer = base.WithTracer

// Client provides methods that the Juju client command uses to interact
// with models stored in the Juju Server.
type Client struct {
	base.ClientFacade
	facade base.FacadeCaller
}

// NewClient creates a new `Client` based on an existing authenticated API
// connection.
func NewClient(st base.APICallCloser, options ...Option) *Client {
	frontend, backend := base.NewClientFacade(st, "ModelConfig", options...)
	return &Client{ClientFacade: frontend, facade: backend}
}

// ModelGet returns all model settings.
func (c *Client) ModelGet(ctx context.Context) (map[string]interface{}, error) {
	result := params.ModelConfigResults{}
	err := c.facade.FacadeCall(ctx, "ModelGet", nil, &result)
	if err != nil {
		return nil, errors.Trace(err)
	}
	values := make(map[string]interface{})
	for name, val := range result.Config {
		values[name] = val.Value
	}
	return values, nil
}

// ModelGetWithMetadata returns all model settings along with extra
// metadata like the source of the setting value.
func (c *Client) ModelGetWithMetadata(ctx context.Context) (config.ConfigValues, error) {
	result := params.ModelConfigResults{}
	err := c.facade.FacadeCall(ctx, "ModelGet", nil, &result)
	if err != nil {
		return nil, errors.Trace(err)
	}
	values := make(config.ConfigValues)
	for name, val := range result.Config {
		values[name] = config.ConfigValue{
			Value:  val.Value,
			Source: val.Source,
		}
	}
	return values, nil
}

// ModelSet sets the given key-value pairs in the model.
func (c *Client) ModelSet(ctx context.Context, config map[string]interface{}) error {
	args := params.ModelSet{Config: config}
	return c.facade.FacadeCall(ctx, "ModelSet", args, nil)
}

// ModelUnset sets the given key-value pairs in the model.
func (c *Client) ModelUnset(ctx context.Context, keys ...string) error {
	args := params.ModelUnset{Keys: keys}
	return c.facade.FacadeCall(ctx, "ModelUnset", args, nil)
}

// GetModelConstraints returns the constraints for the model.
func (c *Client) GetModelConstraints(ctx context.Context) (constraints.Value, error) {
	results := new(params.GetConstraintsResults)
	err := c.facade.FacadeCall(ctx, "GetModelConstraints", nil, results)
	return results.Constraints, err
}

// SetModelConstraints specifies the constraints for the model.
func (c *Client) SetModelConstraints(ctx context.Context, constraints constraints.Value) error {
	params := params.SetConstraints{
		Constraints: constraints,
	}
	return c.facade.FacadeCall(ctx, "SetModelConstraints", params, nil)
}

// Sequences returns all sequence names and next values.
func (c *Client) Sequences(ctx context.Context) (map[string]int, error) {
	var result params.ModelSequencesResult
	err := c.facade.FacadeCall(ctx, "Sequences", nil, &result)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return result.Sequences, nil
}

// GetModelSecretBackend returns the secret backend name for the specified model,
// returning an error satisfying [modelerrors.NotFound] if the model provided does not exist.
func (c *Client) GetModelSecretBackend(ctx context.Context) (string, error) {
	if c.facade.BestAPIVersion() < 4 {
		return "", errors.NotSupportedf("getting model secret backend")
	}

	var result params.StringResult
	err := c.facade.FacadeCall(ctx, "GetModelSecretBackend", nil, &result)
	if err != nil {
		return "", errors.Trace(err)
	}
	if result.Error != nil {
		return "", params.TranslateWellKnownError(result.Error)
	}
	return result.Result, nil
}

// SetModelSecretBackend sets the secret backend config for the specified model,
// returning an error satisfying [secretbackenderrors.NotFound] if the backend provided does not exist,
// returning an error satisfying [modelerrors.NotFound] if the model provided does not exist,
// returning an error satisfying [secretbackenderrors.NotValid] if the backend name provided is not valid.
func (c *Client) SetModelSecretBackend(ctx context.Context, secretBackendName string) error {
	if c.facade.BestAPIVersion() < 4 {
		return errors.NotSupportedf("setting model secret backend")
	}

	var result params.ErrorResult
	err := c.facade.FacadeCall(ctx, "SetModelSecretBackend", params.SetModelSecretBackendArg{
		SecretBackendName: secretBackendName,
	}, &result)
	if err != nil {
		return errors.Trace(err)
	}
	if result.Error != nil {
		return params.TranslateWellKnownError(result.Error)
	}
	return nil
}

// BestAPIVersion returns the best API version supported by the client.
func (c *Client) BestAPIVersion() int {
	return c.facade.BestAPIVersion()
}
