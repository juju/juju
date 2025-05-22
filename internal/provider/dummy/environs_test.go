// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dummy_test

import (
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/cloud"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/environs/jujutest"
	sstesting "github.com/juju/juju/environs/simplestreams/testing"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/internal/provider/dummy"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/juju/keys"
	jujutesting "github.com/juju/juju/juju/testing"
)

const adminSecret = "admin-secret"

func TestSuite(t *stdtesting.T) {
	tc.Run(t, &suite{
		Tests: jujutest.Tests{
			TestConfig: testing.FakeConfig(),
		},
	})
}

type suite struct {
	testing.BaseSuite
	jujutest.Tests
}

func (s *suite) SetUpSuite(c *tc.C) {
	s.BaseSuite.SetUpSuite(c)
	s.PatchValue(&keys.JujuPublicKey, sstesting.SignedMetadataPublicKey)
}

func (s *suite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
	s.PatchValue(&jujuversion.Current, testing.FakeVersionNumber)
	s.Tests.SetUpTest(c)
}

func (s *suite) TearDownTest(c *tc.C) {
	s.Tests.TearDownTest(c)
	s.BaseSuite.TearDownTest(c)
}

func (s *suite) bootstrapTestEnviron(c *tc.C) environs.NetworkingEnviron {
	e, err := bootstrap.PrepareController(
		false,
		envtesting.BootstrapContext(c.Context(), c),
		s.ControllerStore,
		bootstrap.PrepareParams{
			ControllerConfig: testing.FakeControllerConfig(),
			ModelConfig:      s.TestConfig,
			ControllerName:   s.TestConfig["name"].(string),
			Cloud:            testing.FakeCloudSpec(),
			AdminSecret:      adminSecret,
		},
	)
	c.Assert(err, tc.IsNil, tc.Commentf("preparing environ %#v", s.TestConfig))
	c.Assert(e, tc.NotNil)
	env := e.(environs.Environ)
	netenv, supported := environs.SupportsNetworking(env)
	c.Assert(supported, tc.IsTrue)

	err = bootstrap.Bootstrap(envtesting.BootstrapContext(c.Context(), c), netenv,
		bootstrap.BootstrapParams{
			ControllerConfig: testing.FakeControllerConfig(),
			Cloud: cloud.Cloud{
				Name:      "dummy",
				Type:      "dummy",
				AuthTypes: []cloud.AuthType{cloud.EmptyAuthType},
			},
			AdminSecret:             adminSecret,
			CAPrivateKey:            testing.CAKey,
			SupportedBootstrapBases: testing.FakeSupportedJujuBases,
		})
	c.Assert(err, tc.ErrorIsNil)
	return netenv
}

func (s *suite) TestAvailabilityZone(c *tc.C) {
	e := s.bootstrapTestEnviron(c)
	defer func() {
		err := e.Destroy(c.Context())
		c.Assert(err, tc.ErrorIsNil)
	}()

	inst, hwc := jujutesting.AssertStartInstance(c, e, s.ControllerUUID, "0")
	c.Assert(inst, tc.NotNil)
	c.Check(hwc.Arch, tc.NotNil)
}

func (s *suite) TestSupportsSpaces(c *tc.C) {
	e := s.bootstrapTestEnviron(c)
	defer func() {
		err := e.Destroy(c.Context())
		c.Assert(err, tc.ErrorIsNil)
	}()

	// Without change spaces are supported.
	ok, err := e.SupportsSpaces()
	c.Assert(ok, tc.IsTrue)
	c.Assert(err, tc.ErrorIsNil)

	// Now turn it off.
	isEnabled := dummy.SetSupportsSpaces(false)
	c.Assert(isEnabled, tc.IsTrue)
	ok, err = e.SupportsSpaces()
	c.Assert(ok, tc.IsFalse)
	c.Assert(err, tc.ErrorIs, errors.NotSupported)

	// And finally turn it on again.
	isEnabled = dummy.SetSupportsSpaces(true)
	c.Assert(isEnabled, tc.IsFalse)
	ok, err = e.SupportsSpaces()
	c.Assert(ok, tc.IsTrue)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *suite) TestSupportsSpaceDiscovery(c *tc.C) {
	e := s.bootstrapTestEnviron(c)
	defer func() {
		err := e.Destroy(c.Context())
		c.Assert(err, tc.ErrorIsNil)
	}()

	// Without change space discovery is not supported.
	ok, err := e.SupportsSpaceDiscovery()
	c.Assert(ok, tc.IsFalse)
	c.Assert(err, tc.ErrorIsNil)

	// Now turn it on.
	isEnabled := dummy.SetSupportsSpaceDiscovery(true)
	c.Assert(isEnabled, tc.IsFalse)
	ok, err = e.SupportsSpaceDiscovery()
	c.Assert(ok, tc.IsTrue)
	c.Assert(err, tc.ErrorIsNil)

	// And finally turn it off again.
	isEnabled = dummy.SetSupportsSpaceDiscovery(false)
	c.Assert(isEnabled, tc.IsTrue)
	ok, err = e.SupportsSpaceDiscovery()
	c.Assert(ok, tc.IsFalse)
	c.Assert(err, tc.ErrorIsNil)
}
