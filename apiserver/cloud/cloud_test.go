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
	results, err := s.api.Cloud(params.Entities{
		[]params.Entity{{"cloud-my-cloud"}, {"machine-0"}},
	})
	c.Assert(err, jc.ErrorIsNil)
	s.backend.CheckCalls(c, []gitjujutesting.StubCall{
		{"Cloud", []interface{}{"my-cloud"}},
	})
	c.Assert(results.Results, gc.HasLen, 2)
	c.Assert(results.Results[0].Error, gc.IsNil)
	c.Assert(results.Results[0].Cloud, jc.DeepEquals, &params.Cloud{
		Type:      "dummy",
		AuthTypes: []string{"empty", "userpass"},
		Regions:   []params.CloudRegion{{Name: "nether", Endpoint: "endpoint"}},
	})
	c.Assert(results.Results[1].Error, jc.DeepEquals, &params.Error{
		Message: `"machine-0" is not a valid cloud tag`,
	})
}

func (s *cloudSuite) TestCloudDefaults(c *gc.C) {
	results, err := s.api.CloudDefaults(params.Entities{[]params.Entity{
		{"machine-0"},
		{"user-admin"},
		{"user-bruce"},
	}})
	c.Assert(err, jc.ErrorIsNil)
	s.backend.CheckCallNames(c,
		"IsControllerAdmin", // for auth-checking
		"ControllerModel",
		"IsControllerAdmin", // to get default credential
	)
	c.Assert(results.Results, gc.HasLen, 3)
	c.Assert(results.Results[0].Error, jc.DeepEquals, &params.Error{
		Message: `"machine-0" is not a valid user tag`,
	})
	c.Assert(results.Results[1].Error, jc.DeepEquals, &params.Error{
		Message: "permission denied", Code: params.CodeUnauthorized,
	})
	c.Assert(results.Results[2].Error, gc.IsNil)
	c.Assert(results.Results[2].Result, jc.DeepEquals, &params.CloudDefaults{
		CloudTag:        "cloud-some-cloud",
		CloudRegion:     "some-region",
		CloudCredential: "",
	})
}

func (s *cloudSuite) TestCloudDefaultsAdminAccess(c *gc.C) {
	s.authorizer.Tag = names.NewUserTag("admin@local")
	results, err := s.api.CloudDefaults(params.Entities{[]params.Entity{
		{"user-admin"},
	}})
	c.Assert(err, jc.ErrorIsNil)
	s.backend.CheckCallNames(c,
		"IsControllerAdmin", // for auth-checking
		"ControllerModel",
		"IsControllerAdmin", // to get default credential
	)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)
	c.Assert(results.Results[0].Result, jc.DeepEquals, &params.CloudDefaults{
		CloudTag:        "cloud-some-cloud",
		CloudRegion:     "some-region",
		CloudCredential: "some-credential",
	})
}

func (s *cloudSuite) TestCredentials(c *gc.C) {
	results, err := s.api.Credentials(params.UserClouds{[]params.UserCloud{{
		UserTag:  "machine-0",
		CloudTag: "cloud-meep",
	}, {
		UserTag:  "user-admin",
		CloudTag: "cloud-meep",
	}, {
		UserTag:  "user-bruce",
		CloudTag: "cloud-meep",
	}}})
	c.Assert(err, jc.ErrorIsNil)
	s.backend.CheckCallNames(c, "IsControllerAdmin", "CloudCredentials")
	s.backend.CheckCall(c, 1, "CloudCredentials", names.NewUserTag("bruce"), "meep")

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
	results, err := s.api.Credentials(params.UserClouds{[]params.UserCloud{{
		UserTag:  "user-julia",
		CloudTag: "cloud-meep",
	}}})
	c.Assert(err, jc.ErrorIsNil)
	s.backend.CheckCallNames(c, "IsControllerAdmin", "CloudCredentials")
	c.Assert(results.Results, gc.HasLen, 1)
	// admin can access others' credentials
	c.Assert(results.Results[0].Error, gc.IsNil)
}

func (s *cloudSuite) TestUpdateCredentials(c *gc.C) {
	results, err := s.api.UpdateCredentials(params.UsersCloudCredentials{[]params.UserCloudCredentials{{
		UserTag:  "machine-0",
		CloudTag: "cloud-meep",
	}, {
		UserTag:  "user-admin",
		CloudTag: "cloud-meep",
	}, {
		UserTag:  "user-bruce",
		CloudTag: "cloud-meep",
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
	s.backend.CheckCallNames(c, "IsControllerAdmin", "UpdateCloudCredentials")
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
		"meep",
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
		UserTag:  "user-julia",
		CloudTag: "cloud-meep",
		Credentials: map[string]params.CloudCredential{
			"three": {
				AuthType:   "oauth1",
				Attributes: map[string]string{"token": "foo:bar:baz"},
			},
		},
	}}})
	c.Assert(err, jc.ErrorIsNil)
	s.backend.CheckCallNames(c, "IsControllerAdmin", "UpdateCloudCredentials")
	c.Assert(results.Results, gc.HasLen, 1)
	// admin can update others' credentials
	c.Assert(results.Results[0].Error, gc.IsNil)
}

type mockBackend struct {
	gitjujutesting.Stub
	cloud cloud.Cloud
	creds map[string]cloud.Credential
}

func (st *mockBackend) IsControllerAdmin(user names.UserTag) (bool, error) {
	st.MethodCall(st, "IsControllerAdmin", user)
	return user.Canonical() == "admin@local", st.NextErr()
}

func (st *mockBackend) ControllerModel() (cloudfacade.Model, error) {
	st.MethodCall(st, "ControllerModel")
	return &mockModel{"some-cloud", "some-region", "some-credential"}, st.NextErr()
}

func (st *mockBackend) Cloud(name string) (cloud.Cloud, error) {
	st.MethodCall(st, "Cloud", name)
	return st.cloud, st.NextErr()
}

func (st *mockBackend) CloudCredentials(user names.UserTag, cloudName string) (map[string]cloud.Credential, error) {
	st.MethodCall(st, "CloudCredentials", user, cloudName)
	return st.creds, st.NextErr()
}

func (st *mockBackend) UpdateCloudCredentials(user names.UserTag, cloudName string, creds map[string]cloud.Credential) error {
	st.MethodCall(st, "UpdateCloudCredentials", user, cloudName, creds)
	return st.NextErr()
}

func (st *mockBackend) Close() error {
	st.MethodCall(st, "Close")
	return st.NextErr()
}

type mockModel struct {
	cloud           string
	cloudRegion     string
	cloudCredential string
}

func (m *mockModel) Cloud() string {
	return m.cloud
}

func (m *mockModel) CloudRegion() string {
	return m.cloudRegion
}

func (m *mockModel) CloudCredential() string {
	return m.cloudCredential
}
