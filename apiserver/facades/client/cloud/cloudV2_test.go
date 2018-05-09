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
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/permission"
	_ "github.com/juju/juju/provider/dummy"
	_ "github.com/juju/juju/provider/ec2"
	_ "github.com/juju/juju/provider/maas"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
)

var _ = gc.Suite(&cloudSuiteV2{})

type cloudSuiteV2 struct {
	gitjujutesting.IsolationSuite

	backend    *mockBackendV2
	authorizer *apiservertesting.FakeAuthorizer

	apiv2 *cloudfacade.CloudAPIV2
}

func (s *cloudSuiteV2) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	owner := names.NewUserTag("admin")
	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag: owner,
	}

	dummyCred := statetesting.CloudCredential(cloud.UserPassAuthType, map[string]string{
		"username": "admin",
		"password": "sekret",
	})
	dummyCred.Name = "onecredential"
	dummyCred.Cloud = "dummy"
	dummyCred.Owner = owner.Id()
	dummyCredTag, err := dummyCred.CloudCredentialTag()
	c.Assert(err, jc.ErrorIsNil)

	awsCred := statetesting.CloudCredential(cloud.AccessKeyAuthType, map[string]string{
		"access-key": "BLAHB3445635BLAH",
		"secret-key": "fffajdnjsdnnjd667gvd",
	})
	awsCred.Name = "twocredential"
	awsCred.Cloud = "aws"
	awsCred.Owner = owner.Id()
	awsCredTag, err := awsCred.CloudCredentialTag()
	c.Assert(err, jc.ErrorIsNil)

	maasCred := statetesting.CloudCredential(cloud.OAuth1AuthType, map[string]string{
		"maas-oauth": "jdsnbdfvbfdbvdfuvhuodhfuhdov",
	})
	maasCred.Name = "mcredential"
	maasCred.Cloud = "maas"
	maasCred.Owner = owner.Id()
	// no model will be using maas cred in this test suite :D

	s.backend = &mockBackendV2{
		credentials: []state.Credential{
			dummyCred,
			awsCred,
			maasCred,
		},
		models: map[names.CloudCredentialTag][]state.CredentialOwnerModelAccess{
			dummyCredTag: []state.CredentialOwnerModelAccess{
				{ModelName: "abcmodel", OwnerAccess: permission.AdminAccess},
				{ModelName: "xyzmodel", OwnerAccess: permission.ReadAccess},
			},
			awsCredTag: []state.CredentialOwnerModelAccess{
				{ModelName: "whynotmodel", OwnerAccess: permission.NoAccess},
			},
		},
	}

	client, err := cloudfacade.NewCloudAPIV2(s.backend, s.backend, s.authorizer, context.NewCloudCallContext())
	c.Assert(err, jc.ErrorIsNil)
	s.apiv2 = client
}

func (s *cloudSuiteV2) TestCredentialContentsAllNoSecrets(c *gc.C) {
	// Get all credentials with no secrets.
	results, err := s.apiv2.CredentialContents(params.CloudCredentialArgs{})
	c.Assert(err, jc.ErrorIsNil)
	s.backend.CheckCallNames(c, "AllCloudCredentials",
		"Cloud", "CredentialModelsAndOwnerAccess",
		"Cloud", "CredentialModelsAndOwnerAccess",
		"Cloud", "CredentialModelsAndOwnerAccess")

	expected := []params.CredentialContentResult{
		params.CredentialContentResult{
			Result: &params.ControllerCredentialInfo{
				Content: params.CredentialContent{
					Name:     "onecredential",
					Cloud:    "dummy",
					AuthType: "userpass",
					Attributes: map[string]string{
						"username": "admin",
					},
				},
				Models: []params.ModelAccess{
					{Model: "abcmodel", Access: "admin"},
					{Model: "xyzmodel", Access: "read"},
				},
			},
		},
		params.CredentialContentResult{
			Result: &params.ControllerCredentialInfo{
				Content: params.CredentialContent{
					Name:     "twocredential",
					Cloud:    "aws",
					AuthType: "access-key",
					Attributes: map[string]string{
						"access-key": "BLAHB3445635BLAH",
					},
				},
				Models: []params.ModelAccess{
					{Model: "whynotmodel"}, // no acccess
				},
			},
		},
		params.CredentialContentResult{
			Result: &params.ControllerCredentialInfo{
				Content: params.CredentialContent{
					Name:       "mcredential",
					Cloud:      "maas",
					AuthType:   "oauth1",
					Attributes: map[string]string{},
				},
				Models: []params.ModelAccess{},
			},
		},
	}

	c.Assert(results.Results, gc.DeepEquals, expected)
}

func (s *cloudSuiteV2) TestCredentialContentsNoneForUser(c *gc.C) {
	s.backend.credentials = nil
	results, err := s.apiv2.CredentialContents(params.CloudCredentialArgs{})
	c.Assert(err, jc.ErrorIsNil)
	s.backend.CheckCallNames(c, "AllCloudCredentials")
	c.Assert(results.Results, gc.DeepEquals, []params.CredentialContentResult{})
}

func (s *cloudSuiteV2) TestCredentialContentsNamedWithSecrets(c *gc.C) {
	results, err := s.apiv2.CredentialContents(params.CloudCredentialArgs{
		IncludeSecrets: true,
		Credentials:    []params.CloudCredentialArg{{CloudName: "aws", CredentialName: "twocredential"}},
	})
	c.Assert(err, jc.ErrorIsNil)
	s.backend.CheckCallNames(c, "CloudCredential", "Cloud", "CredentialModelsAndOwnerAccess")

	expected := []params.CredentialContentResult{
		params.CredentialContentResult{
			Result: &params.ControllerCredentialInfo{
				Content: params.CredentialContent{
					Name:     "twocredential",
					Cloud:    "aws",
					AuthType: "access-key",
					Attributes: map[string]string{
						"access-key": "BLAHB3445635BLAH",
						"secret-key": "fffajdnjsdnnjd667gvd",
					},
				},
				Models: []params.ModelAccess{
					{Model: "whynotmodel"}, // no access
				},
			},
		},
	}
	c.Assert(results.Results, gc.DeepEquals, expected)
}

func (s *cloudSuiteV2) TestAddCloudInV2(c *gc.C) {
	s.authorizer.Tag = names.NewUserTag("admin")
	paramsCloud := params.AddCloudArgs{
		Name: "newcloudname",
		Cloud: params.Cloud{
			Type:      "fake",
			AuthTypes: []string{"empty", "userpass"},
			Endpoint:  "fake-endpoint",
			Regions:   []params.CloudRegion{{Name: "nether", Endpoint: "nether-endpoint"}},
		}}
	err := s.apiv2.AddCloud(paramsCloud)
	c.Assert(err, jc.ErrorIsNil)
	s.backend.CheckCallNames(c, "AddCloud")
	s.backend.CheckCall(c, 0, "AddCloud", cloud.Cloud{
		Name:      "newcloudname",
		Type:      "fake",
		AuthTypes: []cloud.AuthType{cloud.EmptyAuthType, cloud.UserPassAuthType},
		Endpoint:  "fake-endpoint",
		Regions:   []cloud.Region{{Name: "nether", Endpoint: "nether-endpoint"}},
	})
}

func (s *cloudSuiteV2) TestAddCredentialInV2(c *gc.C) {
	s.authorizer.Tag = names.NewUserTag("admin")
	paramsCreds := params.TaggedCredentials{Credentials: []params.TaggedCredential{{
		Tag: "cloudcred-fake_fake_fake",
		Credential: params.CloudCredential{
			AuthType:   "userpass",
			Attributes: map[string]string{},
		}},
	}}
	results, err := s.apiv2.AddCredentials(paramsCreds)
	c.Assert(err, jc.ErrorIsNil)
	s.backend.CheckCallNames(c, "ControllerTag", "UpdateCloudCredential")
	s.backend.CheckCall(c, 1, "UpdateCloudCredential",
		names.NewCloudCredentialTag("fake/fake/fake"),
		cloud.NewCredential(cloud.UserPassAuthType, map[string]string{}))
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)
}

var cloudTypes = map[string]string{
	"aws":   "ec2",
	"dummy": "dummy",
	"maas":  "maas",
}

type mockBackendV2 struct {
	gitjujutesting.Stub
	credentials []state.Credential
	models      map[names.CloudCredentialTag][]state.CredentialOwnerModelAccess
}

func (st *mockBackendV2) AllCloudCredentials(user names.UserTag) ([]state.Credential, error) {
	st.MethodCall(st, "AllCloudCredentials", user)
	return st.credentials, st.NextErr()
}

func (st *mockBackendV2) CredentialModelsAndOwnerAccess(tag names.CloudCredentialTag) ([]state.CredentialOwnerModelAccess, error) {
	st.MethodCall(st, "CredentialModelsAndOwnerAccess", tag)
	err := st.NextErr()
	if err != nil {
		return nil, st.NextErr()
	}

	models, found := st.models[tag]
	if !found {
		return nil, errors.NotFoundf("models using credential %v", tag)
	}

	return models, st.NextErr()
}

func (st *mockBackendV2) CloudCredential(tag names.CloudCredentialTag) (state.Credential, error) {
	st.MethodCall(st, "CloudCredential", tag)
	err := st.NextErr()
	if err != nil {
		return state.Credential{}, st.NextErr()
	}
	for _, cred := range st.credentials {
		if cred.Name == tag.Name() && cred.Cloud == tag.Cloud().Id() {
			return cred, nil
		}
	}
	return state.Credential{}, errors.NotFoundf("test credential %v", tag)

}

func (st *mockBackendV2) Cloud(name string) (cloud.Cloud, error) {
	st.MethodCall(st, "Cloud", name)
	err := st.NextErr()
	if err != nil {
		return cloud.Cloud{}, st.NextErr()
	}
	// clouds returned here should match some test credential
	for _, cred := range st.credentials {
		if cred.Cloud == name {
			return cloud.Cloud{
				Name:      name,
				Type:      cloudTypes[name],
				AuthTypes: []cloud.AuthType{cloud.AuthType(cred.AuthType)},
			}, nil
		}
	}

	return cloud.Cloud{}, errors.NotFoundf("test cloud %v", name)
}

func (st *mockBackendV2) ControllerTag() names.ControllerTag {
	st.MethodCall(st, "ControllerTag")
	return names.NewControllerTag("deadbeef-1bad-500d-9000-4b1d0d06f00d")
}

func (st *mockBackendV2) UpdateCloudCredential(tag names.CloudCredentialTag, cred cloud.Credential) error {
	st.MethodCall(st, "UpdateCloudCredential", tag, cred)
	return st.NextErr()
}

func (st *mockBackendV2) AddCloud(cloud cloud.Cloud) error {
	st.MethodCall(st, "AddCloud", cloud)
	return st.NextErr()
}

func (st *mockBackendV2) Model() (cloudfacade.Model, error) {
	st.MethodCall(st, "Model")
	return nil, errors.NewNotImplemented(nil, "Model")
}

func (st *mockBackendV2) Clouds() (map[names.CloudTag]cloud.Cloud, error) {
	st.MethodCall(st, "Clouds")
	return nil, errors.NewNotImplemented(nil, "Clouds")
}

func (st *mockBackendV2) CloudCredentials(user names.UserTag, cloudName string) (map[string]state.Credential, error) {
	st.MethodCall(st, "CloudCredentials", user, cloudName)
	return nil, errors.NewNotImplemented(nil, "CloudCredential")
}

func (st *mockBackendV2) RemoveCloudCredential(tag names.CloudCredentialTag) error {
	st.MethodCall(st, "RemoveCloudCredential", tag)
	return errors.NewNotImplemented(nil, "RemoveCloudCredential")
}

func (st *mockBackendV2) ModelConfig() (*config.Config, error) {
	st.MethodCall(st, "ModelConfig")
	return nil, errors.NewNotImplemented(nil, "ModelConfig")
}
