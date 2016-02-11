// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"errors"

	"github.com/juju/cmd"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/testing"
)

type MigrateSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	api *fakeMigrateAPI
}

var _ = gc.Suite(&MigrateSuite{})

const sourceModelUUID = "deadbeef-0bad-400d-8000-4b1d0d06f00d"
const targetControllerUUID = "beefdead-0bad-400d-8000-4b1d0d06f00d"

func (s *MigrateSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)

	err := modelcmd.WriteCurrentController("fake")
	c.Assert(err, jc.ErrorIsNil)

	s.api = &fakeMigrateAPI{}
}

func (s *MigrateSuite) TestMissingModel(c *gc.C) {
	_, err := s.runCommand(c)
	c.Assert(err, gc.ErrorMatches, "model not specified")
}

func (s *MigrateSuite) TestMissingTargetController(c *gc.C) {
	_, err := s.runCommand(c, "mymodel")
	c.Assert(err, gc.ErrorMatches, "target controller not specified")
}

func (s *MigrateSuite) TestTooManyArgs(c *gc.C) {
	_, err := s.runCommand(c, "one", "too", "many")
	c.Assert(err, gc.ErrorMatches, "too many arguments specified")
}

func (s *MigrateSuite) TestSuccess(c *gc.C) {
	// Set up fake connection info for a source model and a target controller.
	s.PatchValue(&connectionInfoForName, func(name string) (configstore.EnvironInfo, error) {
		if name == "source-model" {
			return &fakeEnvironInfo{modelUUID: sourceModelUUID}, nil
		} else if name == "target-controller" {
			return &fakeEnvironInfo{modelUUID: targetControllerUUID}, nil
		} else {
			panic("unknown model/controller name")
		}
	})

	ctx, err := s.runCommand(c, "source-model", "target-controller")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(testing.Stderr(ctx), gc.Matches, "Migration started with ID \"uuid:0\"\n")
	c.Check(s.api.specSeen, jc.DeepEquals, &params.ModelMigrationSpec{
		ModelTag: names.NewModelTag(sourceModelUUID).String(),
		TargetInfo: params.ModelMigrationTargetInfo{
			ControllerTag: names.NewModelTag(targetControllerUUID).String(),
			Addrs:         []string{"1.2.3.4:5"},
			CACert:        "cert",
			AuthTag:       names.NewUserTag("admin").String(),
			Password:      "secret",
		}})
}

func (s *MigrateSuite) TestModelDoesntExist(c *gc.C) {
	// Have the connection info lookup for the source model fail.
	s.PatchValue(&connectionInfoForName, func(string) (configstore.EnvironInfo, error) {
		return nil, errors.New("nope")
	})

	_, err := s.runCommand(c, "source-model", "target-controller")
	c.Check(err, gc.ErrorMatches, "model config lookup: .+")
	c.Check(s.api.specSeen, gc.IsNil) // API shouldn't have been called
}

func (s *MigrateSuite) TestControllerDoesntExist(c *gc.C) {
	// Have the connection info lookup for the source model succeed,
	// but fail for the controller.
	s.PatchValue(&connectionInfoForName, func(name string) (configstore.EnvironInfo, error) {
		if name == "source-model" {
			return &fakeEnvironInfo{modelUUID: sourceModelUUID}, nil
		}
		return nil, errors.New("nope")
	})

	_, err := s.runCommand(c, "source-model", "target-controller")
	c.Check(err, gc.ErrorMatches, "target controller config lookup: .+")
	c.Check(s.api.specSeen, gc.IsNil) // API shouldn't have been called
}

func (s *MigrateSuite) runCommand(c *gc.C, args ...string) (*cmd.Context, error) {
	cmd := modelcmd.WrapController(&migrateCommand{
		api: s.api,
	})
	return testing.RunCommand(c, cmd, args...)
}

type fakeMigrateAPI struct {
	specSeen *params.ModelMigrationSpec
}

func (a *fakeMigrateAPI) InitiateModelMigration(spec params.ModelMigrationSpec) (string, error) {
	a.specSeen = &spec
	return "uuid:0", nil
}

type fakeEnvironInfo struct {
	configstore.EnvironInfo
	modelUUID string
}

func (i *fakeEnvironInfo) APIEndpoint() configstore.APIEndpoint {
	return configstore.APIEndpoint{
		ModelUUID: i.modelUUID,
		Addresses: []string{"1.2.3.4:5"},
		CACert:    "cert",
	}
}

func (i *fakeEnvironInfo) APICredentials() configstore.APICredentials {
	return configstore.APICredentials{
		User:     "admin",
		Password: "secret",
	}
}
