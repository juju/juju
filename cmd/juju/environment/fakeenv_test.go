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
	values     map[string]interface{}
	getError   error
	setError   error
	setValues  map[string]interface{}
	unsetError error
	unsetKeys  []string
}

func (f *fakeEnvAPI) Close() error {
	return nil
}

func (f *fakeEnvAPI) EnvironmentGet() (map[string]interface{}, error) {
	if f.getError != nil {
		return nil, f.getError
	}
	return f.values, nil
}

func (f *fakeEnvAPI) EnvironmentSet(config map[string]interface{}) error {
	f.setValues = config
	return f.setError
}

func (f *fakeEnvAPI) EnvironmentUnset(keys ...string) error {
	f.unsetKeys = keys
	return f.unsetError
}
