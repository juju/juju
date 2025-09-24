// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/environs"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/internal/provider/gce"
)

type identitySuite struct {
	gce.BaseSuite
}

var _ = gc.Suite(&identitySuite{})

func (s *identitySuite) TestFinaliseBootstrapCredentialInstanceRole(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	ctx := envtesting.BootstrapTODOContext(c)
	args := environs.BootstrapParams{
		BootstrapConstraints: constraints.MustParse("instance-role=fred@googledev.com"),
	}
	cred := &jujucloud.Credential{}
	got, err := env.FinaliseBootstrapCredential(ctx, args, cred)
	c.Assert(err, jc.ErrorIsNil)
	want := jujucloud.NewCredential("service-account", map[string]string{
		"service-account": "fred@googledev.com",
	})
	c.Assert(got, jc.DeepEquals, &want)
}

func (s *identitySuite) TestFinaliseBootstrapCredentialInstanceRoleAndServiceAccount(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	ctx := envtesting.BootstrapTODOContext(c)
	args := environs.BootstrapParams{
		BootstrapConstraints: constraints.MustParse("instance-role=fred@googledev.com"),
	}
	cred := jujucloud.NewCredential(jujucloud.ServiceAccountAuthType, map[string]string{
		"service-account": "fred@googledev.com",
	})
	got, err := env.FinaliseBootstrapCredential(ctx, args, &cred)
	c.Assert(err, jc.ErrorIsNil)
	want := jujucloud.NewCredential("service-account", map[string]string{
		"service-account": "fred@googledev.com",
	})
	c.Assert(got, jc.DeepEquals, &want)
}

func (s *identitySuite) TestFinaliseBootstrapCredentialNoInstanceRole(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	ctx := envtesting.BootstrapTODOContext(c)
	args := environs.BootstrapParams{}
	cred := &jujucloud.Credential{}
	got, err := env.FinaliseBootstrapCredential(ctx, args, cred)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, jc.DeepEquals, cred)
}
