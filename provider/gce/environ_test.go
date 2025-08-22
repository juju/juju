// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce_test

import (
	"context"
	"strconv"

	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	"google.golang.org/api/compute/v1"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/base"
	"github.com/juju/juju/environs"
	envcontext "github.com/juju/juju/environs/context"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/provider/gce"
	"github.com/juju/juju/testing"
)

type environSuite struct {
	gce.BaseSuite
}

var _ = gc.Suite(&environSuite{})

func (s *environSuite) TestName(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	name := env.Name()
	c.Assert(name, gc.Equals, "google")
}

func (s *environSuite) TestProvider(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	provider := env.Provider()
	c.Assert(provider, gc.Equals, gce.Provider)
}

func (s *environSuite) TestRegion(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	cloudSpec, err := env.Region()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cloudSpec.Region, gc.Equals, "us-east1")
	c.Assert(cloudSpec.Endpoint, gc.Equals, "https://www.googleapis.com")
}

func (s *environSuite) TestSetConfig(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	cfg := s.NewConfig(c, testing.Attrs{"vpi-id": "foo"})
	err := env.SetConfig(cfg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.Config().AllAttrs(), jc.DeepEquals, cfg.AllAttrs())
}

func (s *environSuite) TestConfig(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)
	c.Assert(env.Config().AllAttrs(), jc.DeepEquals, s.NewConfig(c, nil).AllAttrs())
}

func (s *environSuite) TestBootstrapInvalidCredentialError(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)
	c.Assert(s.InvalidatedCredentials, jc.IsFalse)

	fwName := "juju-" + s.ModelUUID
	s.MockService.EXPECT().Firewalls(gomock.Any(), fwName).Return(nil, gce.InvalidCredentialError)

	params := environs.BootstrapParams{
		ControllerConfig:        testing.FakeControllerConfig(),
		SupportedBootstrapBases: testing.FakeSupportedJujuBases,
	}
	_, err := env.Bootstrap(envtesting.BootstrapContext(context.Background(), c), s.CallCtx, params)
	c.Assert(err, gc.NotNil)
	c.Assert(s.InvalidatedCredentials, jc.IsTrue)
}

func (s *environSuite) TestBootstrap(c *gc.C) {
	config := testing.FakeControllerConfig()
	s.assertBootstrap(c, config, []int{config.APIPort()})
}

func (s *environSuite) TestBootstrapOpensAPIPortsWithAutocert(c *gc.C) {
	config := testing.FakeControllerConfig()
	config["api-port"] = 443
	config["autocert-dns-name"] = "example.com"
	s.assertBootstrap(c, config, []int{443, 80})
}

func (s *environSuite) assertBootstrap(c *gc.C, config controller.Config, expectedPorts []int) {
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
		callCtx envcontext.ProviderCallContext,
		args environs.BootstrapParams,
	) (*environs.BootstrapResult, error) {
		c.Assert(env, gc.Equals, e)
		c.Assert(args, jc.DeepEquals, params)
		return &environs.BootstrapResult{
			Arch: "amd64",
			Base: base.MakeDefaultBase("ubuntu", "22.04"),
		}, nil
	})

	fwName := gce.GlobalFirewallName(env)
	s.MockService.EXPECT().Firewalls(gomock.Any(), fwName).Return([]*compute.Firewall{{
		Name:         fwName,
		TargetTags:   []string{fwName},
		SourceRanges: []string{"0.0.0.0/0"},
	}}, nil)

	expectedPortsStr := make([]string, len(expectedPorts))
	for i, port := range expectedPorts {
		expectedPortsStr[i] = strconv.Itoa(port)
	}
	s.MockService.EXPECT().UpdateFirewall(gomock.Any(), fwName, &compute.Firewall{
		Name:         fwName,
		TargetTags:   []string{fwName},
		SourceRanges: []string{"0.0.0.0/0"},
		Allowed: []*compute.FirewallAllowed{{
			IPProtocol: "tcp",
			Ports:      expectedPortsStr,
		}},
	})

	result, err := env.Bootstrap(envtesting.BootstrapContext(context.Background(), c), s.CallCtx, params)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Arch, gc.Equals, "amd64")
	c.Assert(result.Base.DisplayString(), gc.Equals, "ubuntu@22.04")
}

func (s *environSuite) TestCreateInvalidCredentialError(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)
	c.Assert(s.InvalidatedCredentials, jc.IsFalse)

	s.MockService.EXPECT().VerifyCredentials(gomock.Any()).Return(gce.InvalidCredentialError)

	err := env.Create(s.CallCtx, environs.CreateParams{})
	c.Assert(err, gc.NotNil)
	c.Assert(s.InvalidatedCredentials, jc.IsTrue)
}

func (s *environSuite) TestDestroyInvalidCredentialError(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)
	c.Assert(s.InvalidatedCredentials, jc.IsFalse)

	s.MockService.EXPECT().Instances(gomock.Any(), s.Prefix(env),
		"PENDING", "STAGING", "RUNNING", "DONE", "DOWN", "PROVISIONING", "STOPPED", "STOPPING", "UP").
		Return(nil, gce.InvalidCredentialError)

	err := env.Destroy(s.CallCtx)
	c.Assert(err, gc.NotNil)
	c.Assert(s.InvalidatedCredentials, jc.IsTrue)
}

func (s *environSuite) TestDestroy(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	s.MockService.EXPECT().Instances(gomock.Any(), s.Prefix(env),
		"PENDING", "STAGING", "RUNNING", "DONE", "DOWN", "PROVISIONING", "STOPPED", "STOPPING", "UP").
		Return([]*compute.Instance{{
			Name: "inst-0",
			Zone: "home-zone",
		}, {
			Name: "inst-1",
			Zone: "home-a-zone",
		}}, nil)
	s.MockService.EXPECT().RemoveInstances(gomock.Any(), s.Prefix(env), "inst-0", "inst-1")

	s.MockService.EXPECT().Disks(gomock.Any()).Return([]*compute.Disk{{
		Name:   "zone--566fe7b2-c026-4a86-a2cc-84cb7f9a4868",
		Status: "READY",
		Labels: map[string]string{
			"juju-model-uuid": s.ModelUUID,
		},
	}}, nil)
	s.MockService.EXPECT().RemoveDisk(gomock.Any(), "zone", "zone--566fe7b2-c026-4a86-a2cc-84cb7f9a4868")
	s.MockService.EXPECT().RemoveFirewall(gomock.Any(), gce.GlobalFirewallName(env))

	err := env.Destroy(s.CallCtx)
	c.Assert(err, jc.ErrorIsNil)
}
