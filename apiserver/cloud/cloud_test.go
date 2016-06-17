// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud_test

import (
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	cloudfacade "github.com/juju/juju/apiserver/cloud"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/cloud"
)

type cloudSuite struct {
	gitjujutesting.IsolationSuite
	backend    mockBackend
	authorizer apiservertesting.FakeAuthorizer
	api        *cloudfacade.CloudAPI
}

var _ = gc.Suite(&cloudSuite{})

func (s *cloudSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: names.NewUserTag("bruce@local"),
	}
	s.backend = mockBackend{
		cloud: cloud.Cloud{
			Type:      "dummy",
			AuthTypes: []cloud.AuthType{cloud.EmptyAuthType, cloud.UserPassAuthType},
			Regions:   []cloud.Region{{Name: "nether", Endpoint: "endpoint"}},
		},
		creds: map[string]cloud.Credential{
			"one": cloud.NewEmptyCredential(),
			"two": cloud.NewCredential(cloud.UserPassAuthType, map[string]string{
				"username": "admin",
				"password": "adm1n",
			}),
		},
	}
	var err error
	s.api, err = cloudfacade.NewCloudAPI(&s.backend, &s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *cloudSuite) TestCloud(c *gc.C) {
	cloud, err := s.api.Cloud()
	c.Assert(err, jc.ErrorIsNil)
	s.backend.CheckCallNames(c, "Cloud")
	c.Assert(cloud, jc.DeepEquals, params.Cloud{
		Type:      "dummy",
		AuthTypes: []string{"empty", "userpass"},
		Regions:   []params.CloudRegion{{Name: "nether", Endpoint: "endpoint"}},
	})
}

func (s *cloudSuite) TestCredentials(c *gc.C) {
	results, err := s.api.Credentials(params.Entities{[]params.Entity{{
		Tag: "machine-0",
	}, {
		Tag: "user-admin",
	}, {
		Tag: "user-bruce",
	}}})
	c.Assert(err, jc.ErrorIsNil)
	s.backend.CheckCallNames(c, "IsControllerAdministrator", "CloudCredentials")
	c.Assert(results.Results, gc.HasLen, 3)
	c.Assert(results.Results[0].Error, jc.DeepEquals, &params.Error{
		Message: `"machine-0" is not a valid user tag`,
	})
	c.Assert(results.Results[1].Error, jc.DeepEquals, &params.Error{
		Message: "permission denied", Code: params.CodeUnauthorized,
	})
	c.Assert(results.Results[2].Error, gc.IsNil)
	c.Assert(results.Results[2].Credentials, jc.DeepEquals, map[string]params.CloudCredential{
		"one": {
			AuthType: "empty",
		},
		"two": {
			AuthType: "userpass",
			Attributes: map[string]string{
				"username": "admin",
				"password": "adm1n",
			},
		},
	})
}

func (s *cloudSuite) TestCredentialsAdminAccess(c *gc.C) {
	s.authorizer.Tag = names.NewUserTag("admin@local")
	results, err := s.api.Credentials(params.Entities{[]params.Entity{{
		Tag: "user-julia",
	}}})
	c.Assert(err, jc.ErrorIsNil)
	s.backend.CheckCallNames(c, "IsControllerAdministrator", "CloudCredentials")
	c.Assert(results.Results, gc.HasLen, 1)
	// admin can access others' credentials
	c.Assert(results.Results[0].Error, gc.IsNil)
}

func (s *cloudSuite) TestUpdateCredentials(c *gc.C) {
	results, err := s.api.UpdateCredentials(params.UsersCloudCredentials{[]params.UserCloudCredentials{{
		UserTag: "machine-0",
	}, {
		UserTag: "user-admin",
	}, {
		UserTag: "user-bruce",
		Credentials: map[string]params.CloudCredential{
			"three": {
				AuthType:   "oauth1",
				Attributes: map[string]string{"token": "foo:bar:baz"},
			},
			"four": {
				AuthType: "access-key",
				Attributes: map[string]string{
					"access-key": "foo",
					"secret-key": "bar",
				},
			},
		},
	}}})
	c.Assert(err, jc.ErrorIsNil)
	s.backend.CheckCallNames(c, "IsControllerAdministrator", "UpdateCloudCredentials")
	c.Assert(results.Results, gc.HasLen, 3)
	c.Assert(results.Results[0].Error, jc.DeepEquals, &params.Error{
		Message: `"machine-0" is not a valid user tag`,
	})
	c.Assert(results.Results[1].Error, jc.DeepEquals, &params.Error{
		Message: "permission denied", Code: params.CodeUnauthorized,
	})
	c.Assert(results.Results[2].Error, gc.IsNil)

	s.backend.CheckCall(
		c, 1, "UpdateCloudCredentials",
		names.NewUserTag("bruce"),
		map[string]cloud.Credential{
			"three": cloud.NewCredential(
				cloud.OAuth1AuthType,
				map[string]string{"token": "foo:bar:baz"},
			),
			"four": cloud.NewCredential(
				cloud.AccessKeyAuthType,
				map[string]string{"access-key": "foo", "secret-key": "bar"},
			),
		},
	)
}

func (s *cloudSuite) TestUpdateCredentialsAdminAccess(c *gc.C) {
	s.authorizer.Tag = names.NewUserTag("admin@local")
	results, err := s.api.UpdateCredentials(params.UsersCloudCredentials{[]params.UserCloudCredentials{{
		UserTag: "user-julia",
		Credentials: map[string]params.CloudCredential{
			"three": {
				AuthType:   "oauth1",
				Attributes: map[string]string{"token": "foo:bar:baz"},
			},
		},
	}}})
	c.Assert(err, jc.ErrorIsNil)
	s.backend.CheckCallNames(c, "IsControllerAdministrator", "UpdateCloudCredentials")
	c.Assert(results.Results, gc.HasLen, 1)
	// admin can update others' credentials
	c.Assert(results.Results[0].Error, gc.IsNil)
}

type mockBackend struct {
	gitjujutesting.Stub
	cloud cloud.Cloud
	creds map[string]cloud.Credential
}

func (st *mockBackend) IsControllerAdministrator(user names.UserTag) (bool, error) {
	st.MethodCall(st, "IsControllerAdministrator", user)
	return user.Canonical() == "admin@local", st.NextErr()
}

func (st *mockBackend) Cloud() (cloud.Cloud, error) {
	st.MethodCall(st, "Cloud")
	return st.cloud, st.NextErr()
}

func (st *mockBackend) CloudCredentials(user names.UserTag) (map[string]cloud.Credential, error) {
	st.MethodCall(st, "CloudCredentials", user)
	return st.creds, st.NextErr()
}

func (st *mockBackend) UpdateCloudCredentials(user names.UserTag, creds map[string]cloud.Credential) error {
	st.MethodCall(st, "UpdateCloudCredentials", user, creds)
	return st.NextErr()
}

func (st *mockBackend) Close() error {
	st.MethodCall(st, "Close")
	return st.NextErr()
}
