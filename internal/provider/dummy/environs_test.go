// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dummy_test

import (
	"context"
	stdtesting "testing"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

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

func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
}

func init() {
	gc.Suite(&suite{
		Tests: jujutest.Tests{
			TestConfig: testing.FakeConfig(),
		},
	})
}

type suite struct {
	testing.BaseSuite
	jujutest.Tests
}

func (s *suite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
	s.PatchValue(&keys.JujuPublicKey, sstesting.SignedMetadataPublicKey)
}

func (s *suite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.PatchValue(&jujuversion.Current, testing.FakeVersionNumber)
	s.Tests.SetUpTest(c)
}

func (s *suite) TearDownTest(c *gc.C) {
	s.Tests.TearDownTest(c)
	s.BaseSuite.TearDownTest(c)
}

func (s *suite) bootstrapTestEnviron(c *gc.C) environs.NetworkingEnviron {
	e, err := bootstrap.PrepareController(
		false,
		envtesting.BootstrapContext(context.Background(), c),
		s.ControllerStore,
		bootstrap.PrepareParams{
			ControllerConfig: testing.FakeControllerConfig(),
			ModelConfig:      s.TestConfig,
			ControllerName:   s.TestConfig["name"].(string),
			Cloud:            testing.FakeCloudSpec(),
			AdminSecret:      adminSecret,
		},
	)
	c.Assert(err, gc.IsNil, gc.Commentf("preparing environ %#v", s.TestConfig))
	c.Assert(e, gc.NotNil)
	env := e.(environs.Environ)
	netenv, supported := environs.SupportsNetworking(env)
	c.Assert(supported, jc.IsTrue)

	err = bootstrap.Bootstrap(envtesting.BootstrapContext(context.Background(), c), netenv,
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
	c.Assert(err, jc.ErrorIsNil)
	return netenv
}

func (s *suite) TestAvailabilityZone(c *gc.C) {
	e := s.bootstrapTestEnviron(c)
	defer func() {
		err := e.Destroy(context.Background())
		c.Assert(err, jc.ErrorIsNil)
	}()

	inst, hwc := jujutesting.AssertStartInstance(c, e, s.ControllerUUID, "0")
	c.Assert(inst, gc.NotNil)
	c.Check(hwc.Arch, gc.NotNil)
}

func (s *suite) TestSupportsSpaces(c *gc.C) {
	e := s.bootstrapTestEnviron(c)
	defer func() {
		err := e.Destroy(context.Background())
		c.Assert(err, jc.ErrorIsNil)
	}()

	// Without change spaces are supported.
	ok, err := e.SupportsSpaces()
	c.Assert(ok, jc.IsTrue)
	c.Assert(err, jc.ErrorIsNil)

	// Now turn it off.
	isEnabled := dummy.SetSupportsSpaces(false)
	c.Assert(isEnabled, jc.IsTrue)
	ok, err = e.SupportsSpaces()
	c.Assert(ok, jc.IsFalse)
	c.Assert(err, jc.ErrorIs, errors.NotSupported)

	// And finally turn it on again.
	isEnabled = dummy.SetSupportsSpaces(true)
	c.Assert(isEnabled, jc.IsFalse)
	ok, err = e.SupportsSpaces()
	c.Assert(ok, jc.IsTrue)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *suite) TestSupportsSpaceDiscovery(c *gc.C) {
	e := s.bootstrapTestEnviron(c)
	defer func() {
		err := e.Destroy(context.Background())
		c.Assert(err, jc.ErrorIsNil)
	}()

	// Without change space discovery is not supported.
	ok, err := e.SupportsSpaceDiscovery()
	c.Assert(ok, jc.IsFalse)
	c.Assert(err, jc.ErrorIsNil)

	// Now turn it on.
	isEnabled := dummy.SetSupportsSpaceDiscovery(true)
	c.Assert(isEnabled, jc.IsFalse)
	ok, err = e.SupportsSpaceDiscovery()
	c.Assert(ok, jc.IsTrue)
	c.Assert(err, jc.ErrorIsNil)

	// And finally turn it off again.
	isEnabled = dummy.SetSupportsSpaceDiscovery(false)
	c.Assert(isEnabled, jc.IsTrue)
	ok, err = e.SupportsSpaceDiscovery()
	c.Assert(ok, jc.IsFalse)
	c.Assert(err, jc.ErrorIsNil)
}
