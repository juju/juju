// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	"github.com/juju/names"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
)

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
	}
}

type fakeEnvAPI struct {
	values      map[string]interface{}
	err         error
	keys        []string
	addUsers    []names.UserTag
	removeUsers []names.UserTag
}

func (f *fakeEnvAPI) Close() error {
	return nil
}

func (f *fakeEnvAPI) ModelGet() (map[string]interface{}, error) {
	return f.values, nil
}

func (f *fakeEnvAPI) ModelSet(config map[string]interface{}) error {
	f.values = config
	return f.err
}

func (f *fakeEnvAPI) ModelUnset(keys ...string) error {
	f.keys = keys
	return f.err
}

func (f *fakeEnvAPI) ShareModel(users ...names.UserTag) error {
	f.addUsers = users
	return f.err
}

func (f *fakeEnvAPI) UnshareModel(users ...names.UserTag) error {
	f.removeUsers = users
	return f.err
}
