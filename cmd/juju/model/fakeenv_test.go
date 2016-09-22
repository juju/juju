// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	"errors"

	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/testing"
)

// ModelConfig related fake environment for testing.

type fakeEnvSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	fake *fakeEnvAPI
}

func (s *fakeEnvSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.fake = &fakeEnvAPI{
		values: map[string]interface{}{
			"name":    "test-model",
			"special": "special value",
			"running": true,
		},
		defaults: config.ConfigValues{
			"attr":  {Value: "foo", Source: "default"},
			"attr2": {Value: "bar", Source: "controller"},
			"attr3": {Value: "baz", Source: "region"},
		},
	}
}

type fakeEnvAPI struct {
	values        map[string]interface{}
	cloud, region string
	defaults      config.ConfigValues
	err           error
	keys          []string
}

func (f *fakeEnvAPI) Close() error {
	return nil
}

func (f *fakeEnvAPI) ModelGet() (map[string]interface{}, error) {
	return f.values, nil
}

func (f *fakeEnvAPI) ModelGetWithMetadata() (config.ConfigValues, error) {
	result := make(config.ConfigValues)
	for name, val := range f.values {
		result[name] = config.ConfigValue{Value: val, Source: "model"}
	}
	return result, nil
}

func (f *fakeEnvAPI) ModelSet(config map[string]interface{}) error {
	f.values = config
	return f.err
}

func (f *fakeEnvAPI) ModelUnset(keys ...string) error {
	f.keys = keys
	return f.err
}

// ModelDefaults related fake environment for testing.

type fakeModelDefaultEnvSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	fakeAPIRoot     *fakeAPIConnection
	fakeDefaultsAPI *fakeModelDefaultsAPI
	fakeCloudAPI    *fakeCloudAPI
}

func (s *fakeModelDefaultEnvSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.fakeAPIRoot = &fakeAPIConnection{}
	s.fakeDefaultsAPI = &fakeModelDefaultsAPI{
		values: map[string]interface{}{
			"name":    "test-model",
			"special": "special value",
			"running": true,
		},
		defaults: config.ModelDefaultAttributes{
			"attr": {Default: "foo"},
			"attr2": {
				Controller: "bar",
				Regions: []config.RegionDefaultValue{{
					"dummy-region",
					"dummy-value"}}},
		},
	}
	s.fakeCloudAPI = &fakeCloudAPI{
		clouds: map[string]jujucloud.Cloud{
			"cloud-dummy": {
				Type: "dummy-cloud",
				Regions: []jujucloud.Region{
					{Name: "dummy-region"},
				},
			},
		},
	}
}

type fakeAPIConnection struct {
	api.Connection
}

func (*fakeAPIConnection) Close() error {
	return nil
}

type fakeModelDefaultsAPI struct {
	values        map[string]interface{}
	cloud, region string
	defaults      config.ModelDefaultAttributes
	err           error
	keys          []string
}

func (f *fakeModelDefaultsAPI) Close() error {
	return nil
}

func (f *fakeModelDefaultsAPI) ModelGet() (map[string]interface{}, error) {
	return f.values, nil
}

func (f *fakeModelDefaultsAPI) ModelDefaults() (config.ModelDefaultAttributes, error) {
	return f.defaults, nil
}

func (f *fakeModelDefaultsAPI) SetModelDefaults(cloud, region string, cfg map[string]interface{}) error {
	if f.err != nil {
		return f.err
	}
	f.cloud = cloud
	f.region = region
	for name, val := range cfg {
		f.defaults[name] = config.AttributeDefaultValues{Controller: val}
	}
	return nil
}

func (f *fakeModelDefaultsAPI) UnsetModelDefaults(cloud, region string, keys ...string) error {
	if f.err != nil {
		return f.err
	}
	f.cloud = cloud
	f.region = region
	for _, key := range keys {
		delete(f.defaults, key)
	}
	return nil
}

func (f *fakeModelDefaultsAPI) ModelSet(config map[string]interface{}) error {
	f.values = config
	return f.err
}

func (f *fakeModelDefaultsAPI) ModelUnset(keys ...string) error {
	f.keys = keys
	return f.err
}

type fakeCloudAPI struct {
	clouds map[string]jujucloud.Cloud
}

func (f *fakeCloudAPI) Close() error { return nil }
func (f *fakeCloudAPI) DefaultCloud() (names.CloudTag, error) {
	return names.NewCloudTag("dummy"), nil
}
func (f *fakeCloudAPI) Cloud(name names.CloudTag) (jujucloud.Cloud, error) {
	var (
		c  jujucloud.Cloud
		ok bool
	)
	if c, ok = f.clouds[name.String()]; !ok {
		return jujucloud.Cloud{}, errors.New("Unknown cloud")
	}
	return c, nil
}
