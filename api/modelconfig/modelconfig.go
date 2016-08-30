// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelconfig

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs/config"
)

// Client provides methods that the Juju client command uses to interact
// with models stored in the Juju Server.
type Client struct {
	base.ClientFacade
	facade base.FacadeCaller
}

// NewClient creates a new `Client` based on an existing authenticated API
// connection.
func NewClient(st base.APICallCloser) *Client {
	frontend, backend := base.NewClientFacade(st, "ModelConfig")
	return &Client{ClientFacade: frontend, facade: backend}
}

// Close closes the api connection.
func (c *Client) Close() error {
	return c.ClientFacade.Close()
}

// ModelGet returns all model settings.
func (c *Client) ModelGet() (map[string]interface{}, error) {
	result := params.ModelConfigResults{}
	err := c.facade.FacadeCall("ModelGet", nil, &result)
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
func (c *Client) ModelGetWithMetadata() (config.ConfigValues, error) {
	result := params.ModelConfigResults{}
	err := c.facade.FacadeCall("ModelGet", nil, &result)
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
func (c *Client) ModelSet(config map[string]interface{}) error {
	args := params.ModelSet{Config: config}
	return c.facade.FacadeCall("ModelSet", args, nil)
}

// ModelUnset sets the given key-value pairs in the model.
func (c *Client) ModelUnset(keys ...string) error {
	args := params.ModelUnset{Keys: keys}
	return c.facade.FacadeCall("ModelUnset", args, nil)
}

// ModelDefaults returns the default values for various sources used when
// creating a new model.
func (c *Client) ModelDefaults() (config.ModelDefaultAttributes, error) {
	result := params.ModelDefaultsResult{}
	err := c.facade.FacadeCall("ModelDefaults", nil, &result)
	if err != nil {
		return nil, errors.Trace(err)
	}
	values := make(config.ModelDefaultAttributes)
	for name, val := range result.Config {
		setting := config.AttributeDefaultValues{
			Default:    val.Default,
			Controller: val.Controller,
		}
		for _, region := range val.Regions {
			setting.Regions = append(setting.Regions, config.RegionDefaultValue{
				Name:  region.RegionName,
				Value: region.Value})
		}
		values[name] = setting
	}
	return values, nil
}

// SetModelDefaults updates the specified default model config values.
func (c *Client) SetModelDefaults(cloud, region string, config map[string]interface{}) error {
	var cloudTag string
	if cloud != "" {
		cloudTag = names.NewCloudTag(cloud).String()
	}
	args := params.SetModelDefaults{
		Config: []params.ModelDefaultValues{{
			Config:      config,
			CloudTag:    cloudTag,
			CloudRegion: region,
		}},
	}
	var result params.ErrorResults
	err := c.facade.FacadeCall("SetModelDefaults", args, &result)
	if err != nil {
		return err
	}
	return result.OneError()
}

// UnsetModelDefaults removes the specified default model config values.
func (c *Client) UnsetModelDefaults(cloud, region string, keys ...string) error {
	var cloudTag string
	if cloud != "" {
		cloudTag = names.NewCloudTag(cloud).String()
	}
	args := params.UnsetModelDefaults{
		Keys: []params.ModelUnsetKeys{{
			Keys:        keys,
			CloudTag:    cloudTag,
			CloudRegion: region,
		}},
	}
	var result params.ErrorResults
	err := c.facade.FacadeCall("UnsetModelDefaults", args, &result)
	if err != nil {
		return err
	}
	return result.OneError()
}
