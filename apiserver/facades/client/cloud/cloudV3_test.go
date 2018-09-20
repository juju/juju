// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud_test

import (
	"github.com/juju/errors"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	cloudfacade "github.com/juju/juju/apiserver/facades/client/cloud"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/permission"
)

var _ = gc.Suite(&cloudSuiteV3{})

type cloudSuiteV3 struct {
	gitjujutesting.IsolationSuite

	backend    *mockBackendV3
	authorizer *apiservertesting.FakeAuthorizer

	apiV3 *cloudfacade.CloudAPI
}

func (s *cloudSuiteV3) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	owner := names.NewUserTag("admin")
	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag: owner,
	}

	s.backend = &mockBackendV3{cloudAccess: permission.NoAccess}

	client, err := cloudfacade.NewCloudAPIV3(s.backend, s.backend, s.authorizer, context.NewCloudCallContext())
	c.Assert(err, jc.ErrorIsNil)
	s.apiV3 = client
}

func (s *cloudSuiteV3) TestModifyCloudAccess(c *gc.C) {
	results, err := s.apiV3.ModifyCloudAccess(params.ModifyCloudAccessRequest{
		Changes: []params.ModifyCloudAccess{
			{
				Action:   params.GrantCloudAccess,
				CloudTag: names.NewCloudTag("fluffy").String(),
				UserTag:  names.NewUserTag("fred").String(),
				Access:   "add-model",
			}, {
				Action:   params.RevokeCloudAccess,
				CloudTag: names.NewCloudTag("fluffy").String(),
				UserTag:  names.NewUserTag("mary").String(),
				Access:   "add-model",
			},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	s.backend.CheckCallNames(c, "Cloud", "ControllerTag", "CreateCloudAccess", "Cloud", "ControllerTag", "RemoveCloudAccess")
	s.backend.CheckCall(c, 2, "CreateCloudAccess", "fluffy", names.NewUserTag("fred"), permission.AddModelAccess)
	s.backend.CheckCall(c, 5, "RemoveCloudAccess", "fluffy", names.NewUserTag("mary"))
	c.Assert(results.Results, gc.DeepEquals, []params.ErrorResult{
		{}, {},
	})
}

func (s *cloudSuiteV3) TestModifyCloudUpdateAccess(c *gc.C) {
	s.backend.cloudAccess = permission.AddModelAccess
	results, err := s.apiV3.ModifyCloudAccess(params.ModifyCloudAccessRequest{
		Changes: []params.ModifyCloudAccess{
			{
				Action:   params.GrantCloudAccess,
				CloudTag: names.NewCloudTag("fluffy").String(),
				UserTag:  names.NewUserTag("fred").String(),
				Access:   "admin",
			},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	s.backend.CheckCallNames(c, "Cloud", "ControllerTag", "CreateCloudAccess", "GetCloudAccess", "UpdateCloudAccess")
	s.backend.CheckCall(c, 2, "CreateCloudAccess", "fluffy", names.NewUserTag("fred"), permission.AdminAccess)
	s.backend.CheckCall(c, 4, "UpdateCloudAccess", "fluffy", names.NewUserTag("fred"), permission.AdminAccess)
	c.Assert(results.Results, gc.DeepEquals, []params.ErrorResult{
		{},
	})
}

func (s *cloudSuiteV3) TestModifyCloudAlreadyHasAccess(c *gc.C) {
	s.backend.cloudAccess = permission.AdminAccess
	results, err := s.apiV3.ModifyCloudAccess(params.ModifyCloudAccessRequest{
		Changes: []params.ModifyCloudAccess{
			{
				Action:   params.GrantCloudAccess,
				CloudTag: names.NewCloudTag("fluffy").String(),
				UserTag:  names.NewUserTag("fred").String(),
				Access:   "admin",
			},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	s.backend.CheckCallNames(c, "Cloud", "ControllerTag", "CreateCloudAccess", "GetCloudAccess")
	s.backend.CheckCall(c, 2, "CreateCloudAccess", "fluffy", names.NewUserTag("fred"), permission.AdminAccess)
	s.backend.CheckCall(c, 3, "GetCloudAccess", "fluffy", names.NewUserTag("fred"))
	c.Assert(results.Results, gc.DeepEquals, []params.ErrorResult{
		{Error: &params.Error{Message: `could not grant cloud access: user already has "admin" access or greater`}},
	})
}

type mockBackendV3 struct {
	gitjujutesting.Stub
	cloudfacade.Backend
	cloudAccess permission.Access
}

func (st *mockBackendV3) Cloud(name string) (cloud.Cloud, error) {
	st.MethodCall(st, "Cloud", name)
	return cloud.Cloud{Name: name}, nil
}

func (st *mockBackendV3) ControllerTag() names.ControllerTag {
	st.MethodCall(st, "ControllerTag")
	return names.NewControllerTag("deadbeef-1bad-500d-9000-4b1d0d06f00d")
}

func (st *mockBackendV3) GetCloudAccess(cloud string, user names.UserTag) (permission.Access, error) {
	st.MethodCall(st, "GetCloudAccess", cloud, user)
	return st.cloudAccess, nil
}

func (st *mockBackendV3) CreateCloudAccess(cloud string, user names.UserTag, access permission.Access) error {
	st.MethodCall(st, "CreateCloudAccess", cloud, user, access)
	if st.cloudAccess != permission.NoAccess {
		return errors.AlreadyExistsf("access %s", access)
	}
	st.cloudAccess = access
	return nil
}

func (st *mockBackendV3) UpdateCloudAccess(cloud string, user names.UserTag, access permission.Access) error {
	st.MethodCall(st, "UpdateCloudAccess", cloud, user, access)
	st.cloudAccess = access
	return nil
}

func (st *mockBackendV3) RemoveCloudAccess(cloud string, user names.UserTag) error {
	st.MethodCall(st, "RemoveCloudAccess", cloud, user)
	st.cloudAccess = permission.NoAccess
	return nil
}
