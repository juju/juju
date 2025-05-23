// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes_test

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/caas"
	k8s "github.com/juju/juju/caas/kubernetes"
	k8stesting "github.com/juju/juju/caas/kubernetes/testing"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	provider "github.com/juju/juju/internal/provider/kubernetes"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/uuid"
)

func fakeConfig(c *tc.C, attrs ...coretesting.Attrs) *config.Config {
	cfg, err := coretesting.ModelConfig(c).Apply(fakeConfigAttrs(attrs...))
	c.Assert(err, tc.ErrorIsNil)
	return cfg
}

func fakeConfigAttrs(attrs ...coretesting.Attrs) coretesting.Attrs {
	merged := coretesting.FakeConfig().Merge(coretesting.Attrs{
		"type":             "kubernetes",
		"uuid":             uuid.MustNewUUID().String(),
		"workload-storage": "",
	})
	for _, attrs := range attrs {
		merged = merged.Merge(attrs)
	}
	return merged
}

func fakeCloudSpec() environscloudspec.CloudSpec {
	cred := fakeCredential()
	return environscloudspec.CloudSpec{
		Type:       "kubernetes",
		Name:       "k8s",
		Endpoint:   "host1",
		Credential: &cred,
	}
}

func fakeCredential() cloud.Credential {
	return cloud.NewCredential(cloud.UserPassAuthType, map[string]string{
		"username": "user1",
		"password": "password1",
	})
}

type providerSuite struct {
	testhelpers.IsolationSuite
	dialStub testhelpers.Stub
	provider caas.ContainerEnvironProvider
}

func TestProviderSuite(t *testing.T) {
	tc.Run(t, &providerSuite{})
}

func (s *providerSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.dialStub.ResetCalls()
	s.provider = provider.NewProvider()
}

func (s *providerSuite) TestRegistered(c *tc.C) {
	provider, err := environs.Provider("kubernetes")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(provider, tc.NotNil)
}

func (s *providerSuite) TestOpen(c *tc.C) {
	s.PatchValue(&k8s.NewK8sClients, k8stesting.NoopFakeK8sClients)
	config := fakeConfig(c)
	broker, err := s.provider.Open(c.Context(), environs.OpenParams{
		Cloud:  fakeCloudSpec(),
		Config: config,
	}, environs.NoopCredentialInvalidator())
	c.Check(err, tc.ErrorIsNil)
	c.Assert(broker, tc.NotNil)
}

func (s *providerSuite) TestOpenInvalidCloudSpec(c *tc.C) {
	spec := fakeCloudSpec()
	spec.Name = ""
	s.testOpenError(c, spec, `validating cloud spec: cloud name "" not valid`)
}

func (s *providerSuite) TestOpenMissingCredential(c *tc.C) {
	spec := fakeCloudSpec()
	spec.Credential = nil
	s.testOpenError(c, spec, `validating cloud spec: missing credential not valid`)
}

func (s *providerSuite) TestOpenUnsupportedCredential(c *tc.C) {
	credential := cloud.NewCredential(cloud.OAuth1AuthType, map[string]string{})
	spec := fakeCloudSpec()
	spec.Credential = &credential
	s.testOpenError(c, spec, `validating cloud spec: "oauth1" auth-type not supported`)
}

func (s *providerSuite) testOpenError(c *tc.C, spec environscloudspec.CloudSpec, expect string) {
	_, err := s.provider.Open(c.Context(), environs.OpenParams{
		Cloud:  spec,
		Config: fakeConfig(c),
	}, environs.NoopCredentialInvalidator())
	c.Assert(err, tc.ErrorMatches, expect)
}

func (s *providerSuite) TestValidateCloud(c *tc.C) {
	err := s.provider.ValidateCloud(c.Context(), fakeCloudSpec())
	c.Check(err, tc.ErrorIsNil)
}

func (s *providerSuite) TestValidate(c *tc.C) {
	config := fakeConfig(c)
	validCfg, err := s.provider.Validate(c.Context(), config, nil)
	c.Check(err, tc.ErrorIsNil)

	validAttrs := validCfg.AllAttrs()
	c.Assert(config.AllAttrs(), tc.DeepEquals, validAttrs)
}
