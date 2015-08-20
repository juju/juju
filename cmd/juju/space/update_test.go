// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package space_test

import (
	"github.com/juju/errors"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/space"
	"github.com/juju/juju/feature"
)

type UpdateSuite struct {
	BaseSpaceSuite
}

var _ = gc.Suite(&UpdateSuite{})

func (s *UpdateSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetFeatureFlags(feature.PostNetCLIMVP)
	s.BaseSpaceSuite.SetUpTest(c)
	s.command = space.NewUpdateCommand(s.api)
	c.Assert(s.command, gc.NotNil)
}

func (s *UpdateSuite) TestRunWithSubnetsSucceeds(c *gc.C) {
	s.AssertRunSucceeds(c,
		`updated space "myspace": changed subnets to 10.1.2.0/24, 4.3.2.0/28\n`,
		"", // no stdout, just stderr
		"myspace", "10.1.2.0/24", "4.3.2.0/28",
	)

	s.api.CheckCallNames(c, "UpdateSpace", "Close")
	s.api.CheckCall(c,
		0, "UpdateSpace",
		"myspace", s.Strings("10.1.2.0/24", "4.3.2.0/28"),
	)
}

func (s *UpdateSuite) TestRunWhenSpacesAPIFails(c *gc.C) {
	s.api.SetErrors(errors.New("boom"))

	s.AssertRunFails(c,
		`cannot update space "foo": boom`,
		"foo", "10.1.2.0/24",
	)

	s.api.CheckCallNames(c, "UpdateSpace", "Close")
	s.api.CheckCall(c, 0, "UpdateSpace", "foo", s.Strings("10.1.2.0/24"))
}

func (s *UpdateSuite) TestRunAPIConnectFails(c *gc.C) {
	s.command = space.NewUpdateCommand(nil)
	s.AssertRunFails(c,
		"cannot connect to the API server: no environment specified",
		"myname", "10.0.0.0/8", // Drop the args once RunWitnAPI is called internally.
	)
	// No API calls recoreded.
	s.api.CheckCallNames(c)
}
