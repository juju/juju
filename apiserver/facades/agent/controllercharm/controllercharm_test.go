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

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/facades/agent/controllercharm"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type suite struct{}

var _ = gc.Suite(&suite{})

func Test(t *testing.T) {
	gc.TestingT(t)
}

func (*suite) TestAuth(c *gc.C) {
	tests := []struct {
		description string
		authorizer  facade.Authorizer
		modelType   state.ModelType
		expected    string
	}{{
		description: "unit containeragent on k8s",
		authorizer: apiservertesting.FakeAuthorizer{
			Tag: names.NewUnitTag("controller/0"),
		},
		modelType: state.ModelTypeCAAS,
		expected:  "",
	}, {
		description: "machine agent on lxd",
		authorizer: apiservertesting.FakeAuthorizer{
			Tag:        names.NewMachineTag("0"),
			Controller: true,
		},
		modelType: state.ModelTypeIAAS,
		expected:  "",
	}, {
		description: "non-controller application",
		authorizer: apiservertesting.FakeAuthorizer{
			Tag: names.NewUnitTag("mysql/0"),
		},
		modelType: state.ModelTypeCAAS,
		expected:  `application name should be "controller", received "mysql"`,
	}, {
		description: "client can't access facade",
		authorizer: apiservertesting.FakeAuthorizer{
			Tag: names.NewUserTag("bob"),
		},
		modelType: state.ModelTypeCAAS,
		expected:  "permission denied",
	}}

	for _, test := range tests {
		err := controllercharm.CheckAuth(test.authorizer, test.modelType)
		if test.expected == "" {
			c.Check(err, jc.ErrorIsNil, gc.Commentf(test.description))
		} else {
			c.Check(err, gc.ErrorMatches, test.expected, gc.Commentf(test.description))
		}
	}
}

func (*suite) TestAddMetricsUser(c *gc.C) {
	tests := []struct {
		description string
		username    string
		initUsers   []string
		expectedRes *params.Error
	}{{
		description: "Add non-existent user",
		username:    "juju-metrics-r0",
		initUsers:   []string{},
		expectedRes: nil,
	}, {
		description: "Missing metrics user prefix",
		username:    "foo",
		expectedRes: &params.Error{
			Message: `username "foo" missing prefix "juju-metrics-" not valid`,
			Code:    params.CodeNotValid,
		},
	}, {
		description: "User already exists",
		username:    "juju-metrics-r0",
		initUsers:   []string{"juju-metrics-r0"},
		expectedRes: &params.Error{
			Message: `failed to create user "juju-metrics-r0": user "juju-metrics-r0" already exists`,
			Code:    params.CodeAlreadyExists,
		},
	}}

	for _, test := range tests {
		api := controllercharm.NewAPI(newFakeState(test.initUsers...))
		res, err := api.AddMetricsUser(params.AddUsers{[]params.AddUser{{
			Username:    test.username,
			DisplayName: test.username,
			Password:    "supersecret",
		}}})
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(res, gc.DeepEquals, params.AddUserResults{[]params.AddUserResult{{
			Tag:   test.username,
			Error: test.expectedRes,
		}}})
	}

}

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

func (s *fakeState) Model() (controllercharm.Model, error) {
	return fakeModel{}, nil
}

type fakeModel struct{}

func (m fakeModel) AddUser(state.UserAccessSpec) (permission.UserAccess, error) {
	return permission.UserAccess{}, nil
}
