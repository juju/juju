// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logfwd_test

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/logfwd"
)

type OriginSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&OriginSuite{})

func (s *OriginSuite) TestOriginForMachineAgent(c *gc.C) {
	tag := names.NewMachineTag("99")

	origin := logfwd.OriginForMachineAgent(tag, validOrigin.ControllerUUID, validOrigin.ModelUUID, validOrigin.Software.Version)

	c.Check(origin, jc.DeepEquals, logfwd.Origin{
		ControllerUUID: validOrigin.ControllerUUID,
		ModelUUID:      validOrigin.ModelUUID,
		Hostname:       "machine-99." + validOrigin.ModelUUID,
		Type:           logfwd.OriginTypeMachine,
		Name:           "99",
		Software: logfwd.Software{
			PrivateEnterpriseNumber: 28978,
			Name:                    "jujud-machine-agent",
			Version:                 version.MustParse("2.0.1"),
		},
	})
}

func (s *OriginSuite) TestOriginForUnitAgent(c *gc.C) {
	tag := names.NewUnitTag("svc-a/0")

	origin := logfwd.OriginForUnitAgent(tag, validOrigin.ControllerUUID, validOrigin.ModelUUID, validOrigin.Software.Version)

	c.Check(origin, jc.DeepEquals, logfwd.Origin{
		ControllerUUID: validOrigin.ControllerUUID,
		ModelUUID:      validOrigin.ModelUUID,
		Hostname:       "unit-svc-a-0." + validOrigin.ModelUUID,
		Type:           logfwd.OriginTypeUnit,
		Name:           "svc-a/0",
		Software: logfwd.Software{
			PrivateEnterpriseNumber: 28978,
			Name:                    "jujud-unit-agent",
			Version:                 version.MustParse("2.0.1"),
		},
	})
}

func (s *OriginSuite) TestOriginForJuju(c *gc.C) {
	tag := names.NewUserTag("bob")

	origin, err := logfwd.OriginForJuju(tag, validOrigin.ControllerUUID, validOrigin.ModelUUID, validOrigin.Software.Version)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(origin, jc.DeepEquals, logfwd.Origin{
		ControllerUUID: validOrigin.ControllerUUID,
		ModelUUID:      validOrigin.ModelUUID,
		Hostname:       "",
		Type:           logfwd.OriginTypeUser,
		Name:           "bob",
		Software: logfwd.Software{
			PrivateEnterpriseNumber: 28978,
			Name:                    "juju",
			Version:                 version.MustParse("2.0.1"),
		},
	})
}

func (s *OriginSuite) TestValidateValid(c *gc.C) {
	origin := validOrigin

	err := origin.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (s *OriginSuite) TestValidateEmpty(c *gc.C) {
	var origin logfwd.Origin

	err := origin.Validate()

	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

func (s *OriginSuite) TestValidateEmptyControllerUUID(c *gc.C) {
	origin := validOrigin
	origin.ControllerUUID = ""

	err := origin.Validate()

	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `empty ControllerUUID`)
}

func (s *OriginSuite) TestValidateBadControllerUUID(c *gc.C) {
	origin := validOrigin
	origin.ControllerUUID = "..."

	err := origin.Validate()

	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `ControllerUUID "..." not a valid UUID`)
}

func (s *OriginSuite) TestValidateEmptyModelUUID(c *gc.C) {
	origin := validOrigin
	origin.ModelUUID = ""

	err := origin.Validate()

	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `empty ModelUUID`)
}

func (s *OriginSuite) TestValidateBadModelUUID(c *gc.C) {
	origin := validOrigin
	origin.ModelUUID = "..."

	err := origin.Validate()

	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `ModelUUID "..." not a valid UUID`)
}

func (s *OriginSuite) TestValidateEmptyHostname(c *gc.C) {
	origin := validOrigin
	origin.Hostname = ""

	err := origin.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (s *OriginSuite) TestValidateBadOriginType(c *gc.C) {
	origin := validOrigin
	origin.Type = logfwd.OriginType(999)

	err := origin.Validate()

	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `invalid Type: unsupported origin type`)
}

func (s *OriginSuite) TestValidateEmptyName(c *gc.C) {
	origin := validOrigin
	origin.Name = ""

	err := origin.Validate()

	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `empty Name`)
}

func (s *OriginSuite) TestValidateBadName(c *gc.C) {
	origin := validOrigin
	origin.Name = "..."

	err := origin.Validate()

	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `invalid Name "...": bad user name`)
}

func (s *OriginSuite) TestValidateEmptySoftware(c *gc.C) {
	origin := validOrigin
	origin.Software = logfwd.Software{}

	err := origin.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (s *OriginSuite) TestValidateBadSoftware(c *gc.C) {
	origin := validOrigin
	origin.Software.Version = version.Zero

	err := origin.Validate()

	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `invalid Software: empty Version`)
}

var validOrigin = logfwd.Origin{
	ControllerUUID: "9f484882-2f18-4fd2-967d-db9663db7bea",
	ModelUUID:      "deadbeef-2f18-4fd2-967d-db9663db7bea",
	Hostname:       "spam.x.y.z.com",
	Type:           logfwd.OriginTypeUser,
	Name:           "a-user",
	Software: logfwd.Software{
		PrivateEnterpriseNumber: 28978,
		Name:                    "juju",
		Version:                 version.MustParse("2.0.1"),
	},
}
