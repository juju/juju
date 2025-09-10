// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce_test

import (
	"context"
	stdtesting "testing"

	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/base"
	"github.com/juju/juju/environs"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/internal/provider/gce"
	"github.com/juju/juju/internal/testing"
)

type environSuite struct {
	gce.BaseSuite
}

func TestEnvironSuite(t *stdtesting.T) {
	tc.Run(t, &environSuite{})
}

func (s *environSuite) TestName(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	name := env.Name()
	c.Assert(name, tc.Equals, "google")
}

func (s *environSuite) TestProvider(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	provider := env.Provider()
	c.Assert(provider, tc.Equals, gce.Provider)
}

func (s *environSuite) TestRegion(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	cloudSpec, err := env.Region()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cloudSpec.Region, tc.Equals, "us-east1")
	c.Assert(cloudSpec.Endpoint, tc.Equals, "https://www.googleapis.com")
}

func (s *environSuite) TestSetConfig(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	cfg := s.NewConfig(c, testing.Attrs{"vpi-id": "foo"})
	err := env.SetConfig(c.Context(), cfg)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(env.Config().AllAttrs(), tc.DeepEquals, cfg.AllAttrs())
}

func (s *environSuite) TestConfig(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)
	c.Assert(env.Config().AllAttrs(), tc.DeepEquals, s.NewConfig(c, nil).AllAttrs())
}

func (s *environSuite) TestBootstrap(c *tc.C) {
	config := testing.FakeControllerConfig()
	s.assertBootstrap(c, config, []int{config.APIPort()})
}

func (s *environSuite) TestBootstrapOpensAPIPortsWithAutocert(c *tc.C) {
	config := testing.FakeControllerConfig()
	config["api-port"] = 443
	config["autocert-dns-name"] = "example.com"
	s.assertBootstrap(c, config, []int{443, 80})
}

func (s *environSuite) assertBootstrap(c *tc.C, config controller.Config, expectedPorts []int) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	params := environs.BootstrapParams{
		ControllerConfig:        config,
		SupportedBootstrapBases: testing.FakeSupportedJujuBases,
	}
	s.PatchValue(gce.Bootstrap, func(
		ctx environs.BootstrapContext,
		e environs.Environ,
		args environs.BootstrapParams,
	) (*environs.BootstrapResult, error) {
		c.Assert(env, tc.Equals, e)
		c.Assert(args, tc.DeepEquals, params)
		return &environs.BootstrapResult{
			Arch: "amd64",
			Base: base.MakeDefaultBase("ubuntu", "22.04"),
		}, nil
	})

	result, err := env.Bootstrap(envtesting.BootstrapContext(context.Background(), c), params)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Arch, tc.Equals, "amd64")
	c.Assert(result.Base.DisplayString(), tc.Equals, "ubuntu@22.04")
}

func (s *environSuite) TestDestroyInvalidCredentialError(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)
	c.Assert(s.InvalidatedCredentials, tc.IsFalse)

	s.MockService.EXPECT().Instances(gomock.Any(), s.Prefix(env),
		"PENDING", "STAGING", "RUNNING", "DONE", "DOWN", "PROVISIONING", "STOPPED", "STOPPING", "UP").
		Return(nil, gce.InvalidCredentialError)

	err := env.Destroy(c.Context())
	c.Assert(err, tc.NotNil)
	c.Assert(s.InvalidatedCredentials, tc.IsTrue)
}

func (s *environSuite) TestDestroy(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	s.MockService.EXPECT().Instances(gomock.Any(), s.Prefix(env),
		"PENDING", "STAGING", "RUNNING", "DONE", "DOWN", "PROVISIONING", "STOPPED", "STOPPING", "UP").
		Return([]*computepb.Instance{{
			Name: ptr("inst-0"),
			Zone: ptr("home-zone"),
		}, {
			Name: ptr("inst-1"),
			Zone: ptr("home-a-zone"),
		}}, nil)
	s.MockService.EXPECT().RemoveInstances(gomock.Any(), s.Prefix(env), "inst-0", "inst-1")

	s.MockService.EXPECT().Disks(gomock.Any()).Return([]*computepb.Disk{{
		Name:   ptr("zone--566fe7b2-c026-4a86-a2cc-84cb7f9a4868"),
		Status: ptr("READY"),
		Labels: map[string]string{
			"juju-model-uuid": s.ModelUUID,
		},
	}}, nil)
	s.MockService.EXPECT().RemoveDisk(gomock.Any(), "zone", "zone--566fe7b2-c026-4a86-a2cc-84cb7f9a4868")
	s.MockService.EXPECT().RemoveFirewall(gomock.Any(), gce.GlobalFirewallName(env))

	err := env.Destroy(c.Context())
	c.Assert(err, tc.ErrorIsNil)
}
