// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelcmd_test

import (
	"fmt"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"io/ioutil"
	"path/filepath"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	_ "github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/testing"
)

func init() {
	dummyProvider, err := environs.Provider("dummy")
	if err != nil {
		panic(err)
	}
	environs.RegisterProvider("fake", mockProvider{dummyProvider})
}

type mockProvider struct {
	environs.EnvironProvider
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
		"interactive": cloud.CredentialSchema{
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
	testing.FakeJujuXDGDataHomeSuite
	cloud cloud.Cloud
	store *jujuclienttesting.MemStore
}

var _ = gc.Suite(&credentialsSuite{})

func (s *credentialsSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.cloud = cloud.Cloud{
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

	s.store = jujuclienttesting.NewMemStore()
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
		testing.Context(c), s.store, modelcmd.GetCredentialsParams{
			Cloud:          s.cloud,
			CloudName:      "cloud",
			CloudRegion:    region,
			CredentialName: cred,
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	expectedRegion := region
	if expectedRegion == "" {
		expectedRegion = s.store.Credentials["cloud"].DefaultRegion
		if expectedRegion == "" && len(s.cloud.Regions) > 0 {
			expectedRegion = "first-region"
		}
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
