// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environment_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
)

type fakeEnvSuite struct {
	testing.FakeJujuHomeSuite
	fake *fakeEnvAPI
}

func (s *fakeEnvSuite) SetUpTest(c *gc.C) {
	s.FakeJujuHomeSuite.SetUpTest(c)
	s.fake = &fakeEnvAPI{
		values: map[string]interface{}{
			"name":    "test-env",
			"special": "special value",
			"running": true,
		},
	}
}

type fakeEnvAPI struct {
	values map[string]interface{}
	err    error
	keys   []string
}

func (f *fakeEnvAPI) Close() error {
	return nil
}

func (f *fakeEnvAPI) EnvironmentGet() (map[string]interface{}, error) {
	return f.values, nil
}

func (f *fakeEnvAPI) EnvironmentSet(config map[string]interface{}) error {
	f.values = config
	return f.err
}

func (f *fakeEnvAPI) EnvironmentUnset(keys ...string) error {
	f.keys = keys
	return f.err
}
