// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd_test

import (
	"time"

	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/clock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/provider/lxd"
	"github.com/juju/juju/provider/lxd/lxdnames"
)

var (
	_ = gc.Suite(&providerSuite{})
	_ = gc.Suite(&ProviderFunctionalSuite{})
)

type providerSuite struct {
	lxd.BaseSuite

	provider environs.EnvironProvider
}

func (s *providerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
}

func (s *providerSuite) TestDetectClouds(c *gc.C) {
	clouds, err := s.Provider.DetectClouds()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(clouds, gc.HasLen, 1)
	s.assertLocalhostCloud(c, clouds[0])
}

func (s *providerSuite) TestDetectCloud(c *gc.C) {
	cloud, err := s.Provider.DetectCloud("localhost")
	c.Assert(err, jc.ErrorIsNil)
	s.assertLocalhostCloud(c, cloud)
	cloud, err = s.Provider.DetectCloud("lxd")
	c.Assert(err, jc.ErrorIsNil)
	s.assertLocalhostCloud(c, cloud)
}

func (s *providerSuite) TestDetectCloudError(c *gc.C) {
	_, err := s.Provider.DetectCloud("foo")
	c.Assert(err, gc.ErrorMatches, `cloud foo not found`)
}

func (s *providerSuite) assertLocalhostCloud(c *gc.C, found cloud.Cloud) {
	c.Assert(found, jc.DeepEquals, cloud.Cloud{
		Name: "localhost",
		Type: "lxd",
		AuthTypes: []cloud.AuthType{
			"interactive",
			cloud.CertificateAuthType,
		},
		Regions: []cloud.Region{{
			Name: "localhost",
		}},
		Description: "LXD Container Hypervisor",
	})
}

func (s *providerSuite) TestFinalizeCloud(c *gc.C) {
	c.Skip("To be rewritten during LXD code refactoring for cluster support")

	in := cloud.Cloud{
		Name:      "foo",
		Type:      "lxd",
		AuthTypes: []cloud.AuthType{cloud.CertificateAuthType},
		Regions: []cloud.Region{{
			Name: "bar",
		}},
	}

	var ctx mockContext
	out, err := s.Provider.FinalizeCloud(&ctx, in)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(out, jc.DeepEquals, cloud.Cloud{
		Name:      "foo",
		Type:      "lxd",
		AuthTypes: []cloud.AuthType{cloud.CertificateAuthType},
		Endpoint:  "1.2.3.4:1234",
		Regions: []cloud.Region{{
			Name:     "bar",
			Endpoint: "1.2.3.4:1234",
		}},
	})
	ctx.CheckCallNames(c, "Verbosef")
	ctx.CheckCall(
		c, 0, "Verbosef", "Resolved LXD host address on bridge %s: %s",
		[]interface{}{"test-bridge", "1.2.3.4:1234"},
	)

	// Finalizing a CloudSpec with an empty endpoint involves
	// configuring the local LXD to listen for HTTPS.
	s.Stub.CheckCalls(c, []gitjujutesting.StubCall{
		{"DefaultProfileBridgeName", nil},
		{"InterfaceAddress", []interface{}{"test-bridge"}},
		{"ServerStatus", nil},
		{"SetServerConfig", []interface{}{"core.https_address", "[::]"}},
		{"ServerAddresses", nil},
	})
}

func (s *providerSuite) TestFinalizeCloudNotListening(c *gc.C) {
	s.Provider.Clock = &mockClock{now: time.Now()}
	var ctx mockContext
	s.PatchValue(&s.InterfaceAddr, "8.8.8.8")
	_, err := s.Provider.FinalizeCloud(&ctx, cloud.Cloud{
		Name:      "foo",
		Type:      "lxd",
		AuthTypes: []cloud.AuthType{cloud.CertificateAuthType},
		Regions: []cloud.Region{{
			Name: "bar",
		}},
	})
	c.Assert(err, gc.NotNil)
	c.Assert(err, gc.ErrorMatches, `LXD is not listening on address https:\/\/8\.8\.8\.8 \(reported addresses\: \[.+\]\)`)
}

func (s *providerSuite) TestFinalizeCloudAlreadyListeningHTTPS(c *gc.C) {
	c.Skip("To be rewritten during LXD code refactoring for cluster support")

	s.Client.Server.Config["core.https_address"] = "[::]:9999"
	var ctx mockContext
	_, err := s.Provider.FinalizeCloud(&ctx, cloud.Cloud{
		Name:      "foo",
		Type:      "lxd",
		AuthTypes: []cloud.AuthType{cloud.CertificateAuthType},
	})
	c.Assert(err, jc.ErrorIsNil)

	// The LXD is already listening on HTTPS, so there should be
	// no SetServerConfig call.
	s.Stub.CheckCalls(c, []gitjujutesting.StubCall{
		{"DefaultProfileBridgeName", nil},
		{"InterfaceAddress", []interface{}{"test-bridge"}},
		{"ServerStatus", nil},
		{"SetServerConfig", []interface{}{"core.https_address", "[::]"}},
		{"ServerAddresses", nil},
	})
}

func (s *providerSuite) TestDetectRegions(c *gc.C) {
	regions, err := s.Provider.DetectRegions()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(regions, jc.DeepEquals, []cloud.Region{{Name: lxdnames.DefaultRegion}})
}

func (s *providerSuite) TestValidate(c *gc.C) {
	validCfg, err := s.Provider.Validate(s.Config, nil)
	c.Assert(err, jc.ErrorIsNil)
	validAttrs := validCfg.AllAttrs()

	c.Check(s.Config.AllAttrs(), gc.DeepEquals, validAttrs)
}

type ProviderFunctionalSuite struct {
	lxd.BaseSuite

	provider environs.EnvironProvider
}

func (s *ProviderFunctionalSuite) SetUpTest(c *gc.C) {
	if !s.IsRunningLocally(c) {
		c.Skip("LXD not running locally")
	}

	s.BaseSuite.SetUpTest(c)

	provider, err := environs.Provider("lxd")
	c.Assert(err, jc.ErrorIsNil)

	s.provider = provider
}

func (s *ProviderFunctionalSuite) TestOpen(c *gc.C) {
	env, err := environs.Open(s.provider, environs.OpenParams{
		Cloud:  lxdCloudSpec(),
		Config: s.Config,
	})
	c.Assert(err, jc.ErrorIsNil)
	envConfig := env.Config()

	c.Check(envConfig.Name(), gc.Equals, "testenv")
}

func (s *ProviderFunctionalSuite) TestPrepareConfig(c *gc.C) {
	cfg, err := s.provider.PrepareConfig(environs.PrepareConfigParams{
		Cloud:  lxdCloudSpec(),
		Config: s.Config,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(cfg, gc.NotNil)
}

func (s *ProviderFunctionalSuite) TestPrepareConfigUnsupportedEndpointScheme(c *gc.C) {
	cloudSpec := lxdCloudSpec()
	cloudSpec.Endpoint = "unix://foo"
	_, err := s.provider.PrepareConfig(environs.PrepareConfigParams{
		Cloud:  cloudSpec,
		Config: s.Config,
	})
	c.Assert(err, gc.ErrorMatches, `validating cloud spec: invalid URL "unix://foo": only HTTPS is supported`)
}

func (s *ProviderFunctionalSuite) TestPrepareConfigUnsupportedAuthType(c *gc.C) {
	cred := cloud.NewCredential("foo", nil)
	_, err := s.provider.PrepareConfig(environs.PrepareConfigParams{
		Cloud: environs.CloudSpec{
			Type:       "lxd",
			Name:       "remotehost",
			Credential: &cred,
		},
	})
	c.Assert(err, gc.ErrorMatches, `validating cloud spec: "foo" auth-type not supported`)
}

func (s *ProviderFunctionalSuite) TestPrepareConfigInvalidCertificateAttrs(c *gc.C) {
	cred := cloud.NewCredential(cloud.CertificateAuthType, map[string]string{})
	_, err := s.provider.PrepareConfig(environs.PrepareConfigParams{
		Cloud: environs.CloudSpec{
			Type:       "lxd",
			Name:       "remotehost",
			Credential: &cred,
		},
	})
	c.Assert(err, gc.ErrorMatches, `validating cloud spec: certificate credentials not valid`)
}

func (s *ProviderFunctionalSuite) TestPrepareConfigEmptyAuthNonLocal(c *gc.C) {
	cred := cloud.NewEmptyCredential()
	_, err := s.provider.PrepareConfig(environs.PrepareConfigParams{
		Cloud: environs.CloudSpec{
			Type:       "lxd",
			Name:       "remotehost",
			Endpoint:   "8.8.8.8",
			Credential: &cred,
		},
	})
	c.Assert(err, gc.ErrorMatches, `validating cloud spec: "empty" auth-type not supported`)
}

type mockContext struct {
	gitjujutesting.Stub
}

func (c *mockContext) Verbosef(f string, args ...interface{}) {
	c.MethodCall(c, "Verbosef", f, args)
}

type mockClock struct {
	clock.Clock
	now time.Time
}

func (m *mockClock) Now() time.Time {
	return m.now
}

func (m *mockClock) After(delay time.Duration) <-chan time.Time {
	return time.After(time.Millisecond)
}
