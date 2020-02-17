// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package space_test

import (
	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/juju/core/network"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/space"
)

type MoveToSpaceSuite struct {
}

var _ = gc.Suite(&MoveToSpaceSuite{})

func (s *MoveToSpaceSuite) SetUpTest(c *gc.C) {
}

func (s *MoveToSpaceSuite) runCommand(c *gc.C, api space.SpaceAPI, args ...string) (*cmd.Context, *space.MoveCommand, error) {
	spaceCmd := &space.MoveCommand{
		SpaceCommandBase: space.NewSpaceCommandBase(api),
	}
	ctx, err := cmdtesting.RunCommand(c, spaceCmd, args...)
	return ctx, spaceCmd, err
}

func (s *MoveToSpaceSuite) TestRunWithSubnetsSucceeds(c *gc.C) {
	ctrl, api := setUpMocks(c)
	defer ctrl.Finish()
	spaceName := "default"
	force := false
	CIDRs := []string{"10.1.2.0/24", "15.3.2.0/24"}
	var args []string

	movedChangelog := []network.MovedSpace{{
		Space: "fromb",
		CIDR:  "10.1.2.0/24",
	}, {
		Space: "froma",
		CIDR:  "15.3.2.0/24",
	}}

	expectedMsg := `
Subnet "10.1.2.0/24" moved from "fromb" to "default"
Subnet "15.3.2.0/24" moved from "froma" to "default"
`[1:]

	api.EXPECT().MoveToSpace(spaceName, CIDRs, force).Return(movedChangelog, nil)
	args = append([]string{spaceName}, CIDRs...)
	ctx, _, err := s.runCommand(c, api, args...)

	c.Assert(cmdtesting.Stderr(ctx), jc.DeepEquals, expectedMsg)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *MoveToSpaceSuite) TestRunWhenSpacesAPIFails(c *gc.C) {
	ctrl, api := setUpMocks(c)
	defer ctrl.Finish()
	spaceName := "default"
	force := false
	CIDRs := []string{"10.1.2.0/24", "15.3.2.0/24"}
	args := append([]string{spaceName}, CIDRs...)
	bam := errors.New("boom")

	api.EXPECT().MoveToSpace(spaceName, CIDRs, force).Return(nil, bam)

	ctx, _, err := s.runCommand(c, api, args...)
	c.Assert(err, gc.ErrorMatches, `cannot update space "default": boom`)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "")
}

// TODO: add tests for the force option
