// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controllercharm_test

import (
	"testing"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/facades/agent/controllercharm"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type suite struct{}

var _ = gc.Suite(&suite{})

func Test(t *testing.T) {
	gc.TestingT(t)
}

func (*suite) TestAddMetricsUser(c *gc.C) {
	c.Skip("panicking")
	username := "juju-metrics-r0"

	api := controllercharm.NewAPI(newFakeState())
	res, err := api.AddMetricsUser(params.AddUsers{[]params.AddUser{{
		Username:    username,
		DisplayName: username,
		Password:    "supersecret",
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, gc.DeepEquals, params.AddUserResults{[]params.AddUserResult{{
		Tag:   username,
		Error: nil,
	}}})
}

func (*suite) TestAddMetricsUserAlreadyExists(c *gc.C) {}
func (*suite) TestAddMetricsUserMissingPrefix(c *gc.C) {}

func (*suite) TestRemoveMetricsUser(c *gc.C) {
	tests := []struct {
		description string
		username    string
		initUsers   []string
		expectedRes *params.Error
	}{{
		description: "Remove existing user",
		username:    "juju-metrics-r0",
		initUsers:   []string{"juju-metrics-r0"},
		expectedRes: nil,
	}, {
		description: "Missing metrics user prefix",
		username:    "foo",
		expectedRes: &params.Error{Message: `username "foo" should have prefix "juju-metrics-"`},
	}, {
		description: "User doesn't exist",
		username:    "juju-metrics-r0",
		initUsers:   []string{},
		// TODO: should this succeed as a no-op ??
		expectedRes: &params.Error{
			Message: `failed to delete user "juju-metrics-r0": user "juju-metrics-r0" not found`,
			Code:    params.CodeNotFound,
		},
	}}

	for _, test := range tests {
		api := controllercharm.NewAPI(newFakeState(test.initUsers...))
		res, err := api.RemoveMetricsUser(params.Entities{[]params.Entity{{
			Tag: names.NewUserTag(test.username).String(),
		}}})
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(res, gc.DeepEquals, params.ErrorResults{[]params.ErrorResult{{test.expectedRes}}})
	}
}

func (*suite) TestRemoveMetricsUserMissingPrefix(c *gc.C) {}

func (*suite) TestRemoveMetricsUserNotFound(c *gc.C) {}

type fakeState struct {
	users set.Strings
}

func newFakeState(initUsers ...string) *fakeState {
	return &fakeState{
		users: set.NewStrings(initUsers...),
	}
}

func (s *fakeState) AddUser(name string, _, _, _ string) (*state.User, error) {
	if s.users.Contains(name) {
		return nil, errors.AlreadyExistsf("user %q", name)
	}
	s.users.Add(name)
	return nil, nil
}

func (s *fakeState) RemoveUser(tag names.UserTag) error {
	name := tag.Name()
	if s.users.Contains(name) {
		s.users.Remove(name)
		return nil
	}
	return errors.NotFoundf("user %q", name)
}

func (s *fakeState) Model() (*state.Model, error) {
	//TODO implement me
	panic("implement me")
	// how do we mock a *state.Model?
}
