// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package snap

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type confinementSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&confinementSuite{})

func (s *confinementSuite) TestConfinementPolicy(c *gc.C) {
	tests := []struct {
		Policy ConfinementPolicy
		Err    error
	}{{
		Policy: StrictPolicy,
	}, {
		Policy: ClassicPolicy,
	}, {
		Policy: DevModePolicy,
	}, {
		Policy: JailModePolicy,
	}, {
		Policy: ConfinementPolicy("yolo"),
		Err:    errors.NotValidf("yolo confinement"),
	}}
	for i, test := range tests {
		c.Logf("test %d - %s", i, test.Policy.String())

		err := test.Policy.Validate()
		if err == nil && test.Err == nil {
			continue
		}
		c.Assert(err, gc.ErrorMatches, test.Err.Error())
	}
}

type appSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&appSuite{})

func (s *appSuite) TestValidate(c *gc.C) {
	app := &App{name: "meshuggah"}
	err := app.Validate()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *appSuite) TestValidateWithConfinement(c *gc.C) {
	app := &App{name: "meshuggah"}
	app.confinementPolicy = StrictPolicy

	err := app.Validate()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *appSuite) TestNestedValidate(c *gc.C) {
	app := &App{name: "meshuggah"}
	app.prerequisites = []Installable{&App{name: "faceless"}}

	err := app.Validate()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *appSuite) TestInvalidNestedValidate(c *gc.C) {
	nested := &App{name: "faceless"}
	nested.confinementPolicy = ConfinementPolicy("yolo")

	app := &App{name: "meshuggah"}
	app.prerequisites = []Installable{nested}

	err := app.Validate()
	c.Assert(err, gc.ErrorMatches, "yolo confinement not valid")
}

func (s *appSuite) TestInstall(c *gc.C) {
	app := &App{name: "meshuggah"}
	cmd := app.InstallArgs()
	c.Assert(cmd, gc.DeepEquals, []string{"install", "meshuggah"})
}

func (s *appSuite) TestNestedInstall(c *gc.C) {
	nested := &App{name: "faceless"}

	app := &App{name: "meshuggah"}
	app.prerequisites = []Installable{nested}
	cmd := app.InstallArgs()
	c.Assert(cmd, gc.DeepEquals, []string{"install", "meshuggah"})
}

func (s *appSuite) TestInstallLocal(c *gc.C) {
	app := &App{name: "meshuggah", channel: "latest/stable", path: "/path/to/meshuggah", assertsPath: "/path/to/asserts"}
	cmd := app.InstallArgs()
	c.Assert(cmd, gc.DeepEquals, []string{"install", "/path/to/meshuggah"})
}

func (s *appSuite) TestInstallLocalWithAsserts(c *gc.C) {
	app := &App{name: "meshuggah", channel: "latest/stable", path: "/path/to/meshuggah", assertsPath: "/path/to/asserts"}
	cmd := app.AcknowledgeAssertsArgs()
	c.Assert(cmd, gc.DeepEquals, []string{"ack", "/path/to/asserts"})
}
