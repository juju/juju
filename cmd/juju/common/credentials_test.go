// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	_ "github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/testing"
	"io/ioutil"
	"path/filepath"
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
		"username": {},
		"password": {},
		"key": {
			FileAttr: "key-file",
		},
	}
	return map[cloud.AuthType]cloud.CredentialSchema{
		cloud.UserPassAuthType: schema,
	}
}

type credentialsSuite struct {
	testing.FakeJujuXDGDataHomeSuite
}

var _ = gc.Suite(&credentialsSuite{})

func (s *credentialsSuite) assertGetCredentials(c *gc.C, region string) {
	dir := c.MkDir()
	keyFile := filepath.Join(dir, "keyfile")
	err := ioutil.WriteFile(keyFile, []byte("value"), 0600)
	c.Assert(err, jc.ErrorIsNil)

	store := jujuclienttesting.NewMemStore()
	store.Credentials["cloud"] = cloud.CloudCredential{
		DefaultRegion: "default-region",
		AuthCredentials: map[string]cloud.Credential{
			"secrets": cloud.NewCredential(cloud.UserPassAuthType, map[string]string{
				"username": "user",
				"password": "sekret",
				"key-file": keyFile,
			}),
		},
	}

	credential, regionName, err := common.GetCredentials(testing.Context(c), store, region, "secrets", "cloud", "fake")
	c.Assert(err, jc.ErrorIsNil)
	expectedRegion := region
	if expectedRegion == "" {
		expectedRegion = "default-region"
	}
	c.Assert(regionName, gc.Equals, expectedRegion)
	c.Assert(credential.Attributes(), jc.DeepEquals, map[string]string{
		"key":      "value",
		"username": "user",
		"password": "sekret",
	})
}

func (s *credentialsSuite) TestGetCredentialsDefaultRegion(c *gc.C) {
	s.assertGetCredentials(c, "")
}

func (s *credentialsSuite) TestGetCredentials(c *gc.C) {
	s.assertGetCredentials(c, "region")
}
