// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !gccgo

package vsphere_test

import (
	"os"

	"github.com/juju/errors"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/provider/vsphere"
)

type environSuite struct {
	vsphere.BaseSuite
}

var _ = gc.Suite(&environSuite{})

func (s *environSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
}

func (s *environSuite) TestBootstrap(c *gc.C) {
	s.PatchValue(&vsphere.Bootstrap, func(ctx environs.BootstrapContext, env environs.Environ, args environs.BootstrapParams,
	) (string, string, environs.BootstrapFinalizer, error) {
		return "", "", nil, errors.New("Bootstrap called")
	})

	os.Setenv(osenv.JujuFeatureFlagEnvKey, feature.VSphereProvider)
	_, _, _, err := s.Env.Bootstrap(nil, environs.BootstrapParams{})
	c.Assert(err, gc.ErrorMatches, "Bootstrap called")
}

func (s *environSuite) TestDestroy(c *gc.C) {
	s.PatchValue(&vsphere.DestroyEnv, func(env environs.Environ) error {
		return errors.New("Destroy called")
	})
	err := s.Env.Destroy()
	c.Assert(err, gc.ErrorMatches, "Destroy called")
}
