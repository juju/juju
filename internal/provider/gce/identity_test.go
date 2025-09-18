// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce_test

import (
	stdtesting "testing"

	"github.com/juju/tc"

	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/environs"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/internal/provider/gce"
)

type identitySuite struct {
	gce.BaseSuite
}

func TestIdentitySuite(t *stdtesting.T) {
	tc.Run(t, &identitySuite{})
}

func (s *identitySuite) TestFinaliseBootstrapCredentialInstanceRole(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	ctx := envtesting.BootstrapContext(c.Context(), c)
	args := environs.BootstrapParams{
		BootstrapConstraints: constraints.MustParse("instance-role=fred@googledev.com"),
	}
	cred := &jujucloud.Credential{}
	got, err := env.FinaliseBootstrapCredential(ctx, args, cred)
	c.Assert(err, tc.ErrorIsNil)
	want := jujucloud.NewCredential("service-account", map[string]string{
		"service-account": "fred@googledev.com",
	})
	c.Assert(got, tc.DeepEquals, &want)
}

func (s *identitySuite) TestFinaliseBootstrapCredentialInstanceRoleAndServiceAccount(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	ctx := envtesting.BootstrapTestContext(c)
	args := environs.BootstrapParams{
		BootstrapConstraints: constraints.MustParse("instance-role=fred@googledev.com"),
	}
	cred := jujucloud.NewCredential(jujucloud.ServiceAccountAuthType, map[string]string{
		"service-account": "fred@googledev.com",
	})
	got, err := env.FinaliseBootstrapCredential(ctx, args, &cred)
	c.Assert(err, tc.ErrorIsNil)
	want := jujucloud.NewCredential("service-account", map[string]string{
		"service-account": "fred@googledev.com",
	})
	c.Assert(got, tc.DeepEquals, &want)
}

func (s *identitySuite) TestFinaliseBootstrapCredentialNoInstanceRole(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	ctx := envtesting.BootstrapContext(c.Context(), c)
	args := environs.BootstrapParams{}
	cred := &jujucloud.Credential{}
	got, err := env.FinaliseBootstrapCredential(ctx, args, cred)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.DeepEquals, cred)
}
