// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud_test

import (
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/testing"
)

type detectCredentialsSuite struct {
	store       *jujuclienttesting.MemStore
	aCredential jujucloud.CloudCredential
}

var _ = gc.Suite(&detectCredentialsSuite{})

type mockProvider struct {
	environs.EnvironProvider
	detectedCreds *jujucloud.CloudCredential
}

func (p *mockProvider) DetectCredentials() (*jujucloud.CloudCredential, error) {
	if len(p.detectedCreds.AuthCredentials) == 0 {
		return nil, errors.NotFoundf("credentials")
	}
	return p.detectedCreds, nil
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
	environs.RegisterProvider("mock-provider", &mockProvider{detectedCreds: &s.aCredential})
}

func (s *detectCredentialsSuite) SetUpTest(c *gc.C) {
	s.store = jujuclienttesting.NewMemStore()
	s.aCredential = jujucloud.CloudCredential{}
}

func (s *detectCredentialsSuite) run(c *gc.C, stdin io.Reader, clouds map[string]jujucloud.Cloud) (*cmd.Context, error) {
	registeredProvidersFunc := func() []string {
		return []string{"mock-provider"}
	}
	allCloudsFunc := func() (map[string]jujucloud.Cloud, error) {
		return clouds, nil
	}
	cloudByNameFunc := func(cloudName string) (*jujucloud.Cloud, error) {
		if cloud, ok := clouds[cloudName]; ok {
			return &cloud, nil
		}
		return nil, errors.NotFoundf("cloud %s", cloudName)
	}
	command := cloud.NewDetectCredentialsCommandForTest(s.store, registeredProvidersFunc, allCloudsFunc, cloudByNameFunc)
	err := testing.InitCommand(command, nil)
	c.Assert(err, jc.ErrorIsNil)
	ctx := testing.Context(c)
	ctx.Stdin = stdin
	return ctx, command.Run(ctx)
}

func (s *detectCredentialsSuite) credentialWithLabel(authType jujucloud.AuthType, label string) jujucloud.Credential {
	cred := jujucloud.NewCredential(authType, nil)
	cred.Label = label
	return cred
}

func (s *detectCredentialsSuite) assertDetectCredential(c *gc.C, cloudName, expectedRegion, errText string) {
	s.aCredential = jujucloud.CloudCredential{
		DefaultRegion: "default region",
		AuthCredentials: map[string]jujucloud.Credential{
			"test": s.credentialWithLabel(jujucloud.AccessKeyAuthType, "credential")},
	}
	clouds := map[string]jujucloud.Cloud{
		"test-cloud": {
			Type: "mock-provider",
		},
		"another-cloud": {
			Type: "another-provider",
		},
	}

	stdin := strings.NewReader(fmt.Sprintf("1\n%s\nQ\n", cloudName))
	ctx, err := s.run(c, stdin, clouds)
	c.Assert(err, jc.ErrorIsNil)
	if errText == "" {
		if expectedRegion != "" {
			s.aCredential.DefaultRegion = expectedRegion
		}
		c.Assert(s.store.Credentials["test-cloud"], jc.DeepEquals, s.aCredential)
	} else {
		output := strings.Replace(testing.Stderr(ctx), "\n", "", -1)
		c.Assert(output, gc.Matches, ".*"+regexp.QuoteMeta(errText)+".*")
	}
}

func (s *detectCredentialsSuite) TestDetectNewCredential(c *gc.C) {
	s.assertDetectCredential(c, "test-cloud", "", "")
}

func (s *detectCredentialsSuite) TestDetectCredentialOverwrites(c *gc.C) {
	s.store.Credentials = map[string]jujucloud.CloudCredential{
		"test-cloud": {
			AuthCredentials: map[string]jujucloud.Credential{
				"test": jujucloud.NewCredential(jujucloud.AccessKeyAuthType, nil),
			},
		},
	}
	s.assertDetectCredential(c, "test-cloud", "", "")
}

func (s *detectCredentialsSuite) TestDetectCredentialKeepsExistingRegion(c *gc.C) {
	s.store.Credentials = map[string]jujucloud.CloudCredential{
		"test-cloud": {
			DefaultRegion: "west",
			AuthCredentials: map[string]jujucloud.Credential{
				"test": jujucloud.NewCredential(jujucloud.AccessKeyAuthType, nil),
			},
		},
	}
	s.assertDetectCredential(c, "test-cloud", "west", "")
}

func (s *detectCredentialsSuite) TestDetectCredentialDefaultCloud(c *gc.C) {
	s.assertDetectCredential(c, "", "", "")
}

func (s *detectCredentialsSuite) TestDetectCredentialUnknownCloud(c *gc.C) {
	s.assertDetectCredential(c, "foo", "", "cloud foo not found")
}

func (s *detectCredentialsSuite) TestDetectCredentialInvalidCloud(c *gc.C) {
	s.assertDetectCredential(c, "another-cloud", "", "chosen credentials not compatible with a another-provider cloud")
}

func (s *detectCredentialsSuite) TestNewDetectCredentialNoneFound(c *gc.C) {
	stdin := strings.NewReader("")
	ctx, err := s.run(c, stdin, nil)
	c.Assert(err, jc.ErrorIsNil)
	output := strings.Replace(testing.Stderr(ctx), "\n", "", -1)
	c.Assert(output, gc.Matches, ".*No cloud credentials found.*")
	c.Assert(s.store.Credentials, gc.HasLen, 0)
}

func (s *detectCredentialsSuite) TestDetectCredentialInvalidChoice(c *gc.C) {
	s.aCredential = jujucloud.CloudCredential{
		DefaultRegion: "detected region",
		AuthCredentials: map[string]jujucloud.Credential{
			"test":    s.credentialWithLabel(jujucloud.AccessKeyAuthType, "credential 1"),
			"another": s.credentialWithLabel(jujucloud.AccessKeyAuthType, "credential 2")},
	}

	stdin := strings.NewReader("3\nQ\n")
	ctx, err := s.run(c, stdin, nil)
	c.Assert(err, jc.ErrorIsNil)
	output := strings.Replace(testing.Stderr(ctx), "\n", "", -1)
	c.Assert(output, gc.Matches, ".*Invalid choice, enter a number between 1 and 2.*")
	c.Assert(s.store.Credentials, gc.HasLen, 0)
}
