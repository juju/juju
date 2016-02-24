// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud_test

import (
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
	store       jujuclient.CredentialsStore
	aCredential jujucloud.Credential
}

var _ = gc.Suite(&detectCredentialsSuite{})

type mockProvider struct {
	environs.EnvironProvider
	aCredential jujucloud.Credential
}

func (p *mockProvider) DetectCredentials() ([]environs.NamedCredential, error) {
	return []environs.NamedCredential{{
		Label:      "label",
		Credential: p.aCredential,
	}}, nil
}

func (s *detectCredentialsSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.store = jujuclienttesting.NewMemStore()
	s.aCredential = jujucloud.NewCredential(jujucloud.AccessKeyAuthType, nil)
	environs.RegisterProvider("test", &mockProvider{aCredential: s.aCredential})
	err := jujucloud.WritePersonalCloudMetadata(map[string]jujucloud.Cloud{
		"test": jujucloud.Cloud{Type: "test"},
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *detectCredentialsSuite) TestDetectCredentials(c *gc.C) {
	s.detectCredentials(c, "test")
	creds, err := s.store.CredentialsForCloud("test")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(creds.AuthCredentials["label"], jc.DeepEquals, s.aCredential)
}

// TODO(wallyworld) - add more test coverage

func (s *detectCredentialsSuite) detectCredentials(c *gc.C, args ...string) string {
	ctx, err := testing.RunCommand(c, cloud.NewDetectCredentialsCommandForTest(s.store), args...)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stderr(ctx), gc.Equals, "")
	return testing.Stdout(ctx)
}
