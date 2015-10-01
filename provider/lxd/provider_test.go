// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/provider/gce"
)

type providerSuite struct {
	gce.BaseSuite

	provider environs.EnvironProvider
}

var _ = gc.Suite(&providerSuite{})

func (s *providerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	var err error
	s.provider, err = environs.Provider("gce")
	c.Check(err, jc.ErrorIsNil)
}

func (s *providerSuite) TestRegistered(c *gc.C) {
	c.Assert(s.provider, gc.Equals, gce.Provider)
}

func (s *providerSuite) TestOpen(c *gc.C) {
	env, err := s.provider.Open(s.Config)
	c.Check(err, jc.ErrorIsNil)

	envConfig := env.Config()
	c.Assert(envConfig.Name(), gc.Equals, "testenv")
}

func (s *providerSuite) TestPrepareForBootstrap(c *gc.C) {
	env, err := s.provider.PrepareForBootstrap(envtesting.BootstrapContext(c), s.Config)
	c.Check(err, jc.ErrorIsNil)
	c.Check(env, gc.NotNil)
}

func (s *providerSuite) TestValidate(c *gc.C) {
	validCfg, err := s.provider.Validate(s.Config, nil)
	c.Check(err, jc.ErrorIsNil)

	validAttrs := validCfg.AllAttrs()
	c.Assert(s.Config.AllAttrs(), gc.DeepEquals, validAttrs)
}

func (s *providerSuite) TestSecretAttrs(c *gc.C) {
	obtainedAttrs, err := s.provider.SecretAttrs(s.Config)
	c.Check(err, jc.ErrorIsNil)

	expectedAttrs := map[string]string{"private-key": gce.PrivateKey}
	c.Assert(obtainedAttrs, gc.DeepEquals, expectedAttrs)

}

func (s *providerSuite) TestBoilerplateConfig(c *gc.C) {
	// (wwitzel3) purposefully duplicate here so that this test will
	// fail if someone updates gce/config.go without updating this test.
	var boilerplateConfig = `
gce:
  type: gce

  # Google Auth Info
  # The GCE provider uses OAuth to authenticate. This requires that
  # you set it up and get the relevant credentials. For more information
  # see https://cloud.google.com/compute/docs/api/how-tos/authorization.
  # The key information can be downloaded as a JSON file, or copied, from:
  #   https://console.developers.google.com/project/<projet>/apiui/credential
  # Either set the path to the downloaded JSON file here:
  auth-file:

  # ...or set the individual fields for the credentials. Either way, all
  # three of these are required and have specific meaning to GCE.
  # private-key:
  # client-email:
  # client-id:

  # Google instance info
  # To provision instances and perform related operations, the provider
  # will need to know which GCE project to use and into which region to
  # provision. While the region has a default, the project ID is
  # required. For information on the project ID, see
  # https://cloud.google.com/compute/docs/projects and regarding regions
  # see https://cloud.google.com/compute/docs/zones.
  project-id:
  # region: us-central1

  # The GCE provider uses pre-built images when provisioning instances.
  # You can customize the location in which to find them with the
  # image-endpoint setting. The default value is the a location within
  # GCE, so it will give you the best speed when bootstrapping or adding
  # machines. For more information on the image cache see
  # https://cloud-images.ubuntu.com/.
  # image-endpoint: https://www.googleapis.com
`[1:]
	c.Assert(s.provider.BoilerplateConfig(), gc.Equals, boilerplateConfig)
}
