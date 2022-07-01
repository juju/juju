// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelcmd_test

import (
	"fmt"
	"io/ioutil"
	"path/filepath"

	"github.com/golang/mock/gomock"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/v3/cloud"
	"github.com/juju/juju/v3/cmd/modelcmd"
	"github.com/juju/juju/v3/environs"
	"github.com/juju/juju/v3/jujuclient"
	_ "github.com/juju/juju/v3/provider/dummy"
)

func init() {
	dummyProvider, err := environs.Provider("dummy")
	if err != nil {
		panic(err)
	}
	// dummy does implement CloudEnvironProvider
	asCloud := dummyProvider.(environs.CloudEnvironProvider)
	environs.RegisterProvider("fake", mockProvider{asCloud})
}

type mockProvider struct {
	environs.CloudEnvironProvider
}

func (mockProvider) CredentialSchemas() map[cloud.AuthType]cloud.CredentialSchema {
	schema := cloud.CredentialSchema{
		{
			"username", cloud.CredentialAttr{},
		}, {
			"password", cloud.CredentialAttr{},
		}, {
			"key", cloud.CredentialAttr{FileAttr: "key-file"},
		},
	}
	return map[cloud.AuthType]cloud.CredentialSchema{
		cloud.UserPassAuthType: schema,
		"interactive": {
			{"username", cloud.CredentialAttr{}},
		},
	}
}

func (mockProvider) FinalizeCredential(
	ctx environs.FinalizeCredentialContext,
	args environs.FinalizeCredentialParams,
) (*cloud.Credential, error) {
	if args.Credential.AuthType() == "interactive" {
		username := args.Credential.Attributes()["username"]
		fmt.Fprintf(ctx.GetStderr(), "generating credential for %q\n", username)
		out := cloud.NewCredential(cloud.UserPassAuthType, map[string]string{
			"username": username,
			"password": "sekret",
			"key":      "value",
		})
		return &out, nil
	}
	return &args.Credential, nil
}

type credentialsSuite struct {
	testing.IsolationSuite
	cloud cloud.Cloud
	store *jujuclient.MemStore
}

var _ = gc.Suite(&credentialsSuite{})

func (s *credentialsSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.cloud = cloud.Cloud{
		Name: "cloud",
		Type: "fake",
		Regions: []cloud.Region{
			{Name: "first-region"},
			{Name: "second-region"},
		},
	}

	dir := c.MkDir()
	keyFile := filepath.Join(dir, "keyfile")
	err := ioutil.WriteFile(keyFile, []byte("value"), 0600)
	c.Assert(err, jc.ErrorIsNil)

	s.store = jujuclient.NewMemStore()
	s.store.Credentials["cloud"] = cloud.CloudCredential{
		DefaultRegion: "second-region",
		AuthCredentials: map[string]cloud.Credential{
			"interactive": cloud.NewCredential("interactive", map[string]string{
				"username": "user",
			}),
			"secrets": cloud.NewCredential(cloud.UserPassAuthType, map[string]string{
				"username": "user",
				"password": "sekret",
				"key-file": keyFile,
			}),
		},
	}
}

func (s *credentialsSuite) assertGetCredentials(c *gc.C, cred, region string) {
	credential, credentialName, regionName, err := modelcmd.GetCredentials(
		cmdtesting.Context(c), s.store, modelcmd.GetCredentialsParams{
			Cloud:          s.cloud,
			CloudRegion:    region,
			CredentialName: cred,
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	expectedRegion := region
	if expectedRegion == "" {
		expectedRegion = s.store.Credentials["cloud"].DefaultRegion
	}
	c.Assert(regionName, gc.Equals, expectedRegion)
	c.Assert(credentialName, gc.Equals, cred)
	c.Assert(credential.Attributes(), jc.DeepEquals, map[string]string{
		"key":      "value",
		"username": "user",
		"password": "sekret",
	})
}

func (s *credentialsSuite) TestGetCredentialsUserDefaultRegion(c *gc.C) {
	s.assertGetCredentials(c, "secrets", "")
}

func (s *credentialsSuite) TestGetCredentialsCloudDefaultRegion(c *gc.C) {
	creds := s.store.Credentials["cloud"]
	creds.DefaultRegion = ""
	s.store.Credentials["cloud"] = creds
	s.assertGetCredentials(c, "secrets", "")
}

func (s *credentialsSuite) TestGetCredentialsNoRegion(c *gc.C) {
	creds := s.store.Credentials["cloud"]
	creds.DefaultRegion = ""
	s.store.Credentials["cloud"] = creds
	s.cloud.Regions = nil
	s.assertGetCredentials(c, "secrets", "")
}

func (s *credentialsSuite) TestGetCredentials(c *gc.C) {
	s.cloud.Regions = append(s.cloud.Regions, cloud.Region{Name: "third-region"})
	s.assertGetCredentials(c, "secrets", "third-region")
}

func (s *credentialsSuite) TestGetCredentialsProviderFinalizeCredential(c *gc.C) {
	s.assertGetCredentials(c, "interactive", "")
}

func (s *credentialsSuite) TestRegisterCredentials(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockProvider := modelcmd.NewMockTestCloudProvider(ctrl)

	credential := map[string]*cloud.CloudCredential{
		"fake": {
			AuthCredentials: map[string]cloud.Credential{
				"admin": cloud.NewCredential("certificate", map[string]string{
					"cert": "certificate",
				}),
			},
		},
	}

	exp := mockProvider.EXPECT()
	exp.RegisterCredentials(cloud.Cloud{
		Name: "fake",
	}).Return(credential, nil)

	credentials, err := modelcmd.RegisterCredentials(mockProvider, modelcmd.RegisterCredentialsParams{
		Cloud: cloud.Cloud{
			Name: "fake",
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(credentials, gc.DeepEquals, credential)
}

func (s *credentialsSuite) TestRegisterCredentialsWithNoCredentials(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockProvider := modelcmd.NewMockTestCloudProvider(ctrl)

	credential := map[string]*cloud.CloudCredential{}

	exp := mockProvider.EXPECT()
	exp.RegisterCredentials(cloud.Cloud{
		Name: "fake",
	}).Return(credential, nil)

	credentials, err := modelcmd.RegisterCredentials(mockProvider, modelcmd.RegisterCredentialsParams{
		Cloud: cloud.Cloud{
			Name: "fake",
		},
	})
	c.Assert(errors.Cause(err).Error(), gc.Matches, `credentials for provider not found`)
	c.Assert(credentials, gc.IsNil)
}

func (s *credentialsSuite) TestRegisterCredentialsWithCallFailure(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockProvider := modelcmd.NewMockTestCloudProvider(ctrl)

	exp := mockProvider.EXPECT()
	exp.RegisterCredentials(cloud.Cloud{
		Name: "fake",
	}).Return(nil, errors.New("bad"))

	credentials, err := modelcmd.RegisterCredentials(mockProvider, modelcmd.RegisterCredentialsParams{
		Cloud: cloud.Cloud{
			Name: "fake",
		},
	})
	c.Assert(err.Error(), gc.Matches, `registering credentials for provider: bad`)
	c.Assert(credentials, gc.IsNil)
}

func (s *credentialsSuite) TestDetectCredential(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	credential := &cloud.CloudCredential{
		AuthCredentials: map[string]cloud.Credential{
			"admin": cloud.NewCredential("certificate", map[string]string{
				"cert": "certificate",
			}),
		},
	}

	mockProvider := modelcmd.NewMockTestCloudProvider(ctrl)
	mockProvider.EXPECT().DetectCredentials("fake").Return(credential, nil)

	credentials, err := modelcmd.DetectCredential("fake", mockProvider)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(credentials, gc.DeepEquals, credential)
}
