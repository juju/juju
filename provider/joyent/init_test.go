// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package joyent_test

import (
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/provider/joyent"
	jp "github.com/juju/juju/provider/joyent"
	"github.com/juju/juju/testing"
)

type initSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&initSuite{})

func (s *initSuite) SetUpSuite(c *gc.C) {
	restoreSdcAccount := jujutesting.PatchEnvironment(jp.SdcAccount, "tester")
	s.AddSuiteCleanup(func(*gc.C) { restoreSdcAccount() })
	restoreSdcKeyId := jujutesting.PatchEnvironment(jp.SdcKeyId, "ff:ee:dd:cc:bb:aa:99:88:77:66:55:44:33:22:11:00")
	s.AddSuiteCleanup(func(*gc.C) { restoreSdcKeyId() })
	restoreMantaUser := jujutesting.PatchEnvironment(jp.MantaUser, "tester")
	s.AddSuiteCleanup(func(*gc.C) { restoreMantaUser() })
	restoreMantaKeyId := jujutesting.PatchEnvironment(jp.MantaKeyId, "ff:ee:dd:cc:bb:aa:99:88:77:66:55:44:33:22:11:00")
	s.AddSuiteCleanup(func(*gc.C) { restoreMantaKeyId() })

	jp.RegisterMachinesEndpoint()
	s.AddSuiteCleanup(func(*gc.C) { jp.UnregisterMachinesEndpoint() })
}

func (s *initSuite) SetUpTest(c *gc.C) {
	for _, envVar := range jp.EnvironmentVariables {
		s.PatchEnvironment(envVar, "")
	}
	s.AddCleanup(CreateTestKey(c))
}

func (s *initSuite) TestImageMetadataDatasourceAdded(c *gc.C) {
	env := joyent.MakeEnviron(c, validAttrs())
	dss, err := environs.ImageMetadataSources(env)
	c.Assert(err, jc.ErrorIsNil)

	expected := "cloud local storage"
	found := false
	for i, ds := range dss {
		c.Logf("datasource %d: %+v", i, ds)
		if ds.Description() == expected {
			found = true
			break
		}
	}
	c.Assert(found, jc.IsTrue)
}
