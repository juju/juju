// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud_test

import (
	"strings"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/testing"
)

type detectCredentialsSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	store       jujuclient.CredentialStore
	aCredential jujucloud.CloudCredential
}

var _ = gc.Suite(&detectCredentialsSuite{})

type mockProvider struct {
	environs.EnvironProvider
	detectedCreds jujucloud.CloudCredential
}

func (p *mockProvider) DetectCredentials() (*jujucloud.CloudCredential, error) {
	return &p.detectedCreds, nil
}

func (p *mockProvider) CredentialSchemas() map[jujucloud.AuthType]jujucloud.CredentialSchema {
	return map[jujucloud.AuthType]jujucloud.CredentialSchema{
		jujucloud.AccessKeyAuthType: {
			"access-key": {},
			"secret-key": {
				Hidden: true,
			},
		},
		jujucloud.UserPassAuthType: {
			"username": {},
			"password": {
				Hidden: true,
			},
			"application-password": {
				Hidden: true,
			},
		},
		jujucloud.OAuth2AuthType: {
			"client-id":    {},
			"client-email": {},
			"private-key": {
				Hidden: true,
			},
			"project-id": {},
		},
	}
}

func (s *detectCredentialsSuite) SetUpSuite(c *gc.C) {
	s.aCredential = jujucloud.CloudCredential{
		DefaultRegion: "detected region",
		AuthCredentials: map[string]jujucloud.Credential{
			"test": jujucloud.NewCredential(jujucloud.AccessKeyAuthType, nil)},
	}
	environs.RegisterProvider("test", &mockProvider{detectedCreds: s.aCredential})
}

func (s *detectCredentialsSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.store = jujuclienttesting.NewMemStore()
	err := jujucloud.WritePersonalCloudMetadata(map[string]jujucloud.Cloud{
		"test": jujucloud.Cloud{Type: "test"},
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *detectCredentialsSuite) TestDetectCredentials(c *gc.C) {
	s.detectCredentials(c, "test")
	creds, err := s.store.CredentialForCloud("test")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(creds, jc.DeepEquals, &s.aCredential)
}

func (s *detectCredentialsSuite) TestDetectPromptsForOverwrite(c *gc.C) {
	inital := jujucloud.CloudCredential{
		AuthCredentials: map[string]jujucloud.Credential{
			"test": jujucloud.NewCredential(jujucloud.UserPassAuthType, nil)},
	}
	s.store.UpdateCredential("test", inital)
	out := s.detectCredentials(c, "test")
	out = strings.Replace(out, "\n", "", -1)
	c.Assert(out, gc.Equals, `Detected credentials [test] would overwrite existing credentials for cloud test.Use the --replace option.`)

	// Has not been replaced.
	creds, err := s.store.CredentialForCloud("test")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(creds, jc.DeepEquals, &inital)
}

func (s *detectCredentialsSuite) TestDetectForceOverwrite(c *gc.C) {
	inital := jujucloud.CloudCredential{
		DefaultRegion: "region",
		AuthCredentials: map[string]jujucloud.Credential{
			"test":    jujucloud.NewCredential(jujucloud.UserPassAuthType, nil),
			"another": jujucloud.NewCredential(jujucloud.AccessKeyAuthType, nil)},
	}
	s.store.UpdateCredential("test", inital)
	out := s.detectCredentials(c, "test", "--replace")
	out = strings.Replace(out, "\n", "", -1)
	c.Assert(out, gc.Equals, `test cloud credential "test" found`)

	// Has been replaced.
	creds, err := s.store.CredentialForCloud("test")
	c.Assert(err, jc.ErrorIsNil)
	replaced := inital
	replaced.DefaultRegion = "detected region"
	replaced.AuthCredentials["test"] = s.aCredential.AuthCredentials["test"]
	c.Assert(creds, jc.DeepEquals, &replaced)
}

// TODO(wallyworld) - add more test coverage

func (s *detectCredentialsSuite) detectCredentials(c *gc.C, args ...string) string {
	ctx, err := testing.RunCommand(c, cloud.NewDetectCredentialsCommandForTest(s.store), args...)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stderr(ctx), gc.Equals, "")
	return testing.Stdout(ctx)
}
