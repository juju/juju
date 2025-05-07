// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package space_test

import (
	"bytes"
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/cmd/juju/space"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/rpc/params"
)

type ShowSuite struct {
	BaseSpaceSuite
}

var _ = tc.Suite(&ShowSuite{})

func (s *ShowSuite) SetUpTest(c *tc.C) {
	s.BaseSpaceSuite.SetUpTest(c)
	s.newCommand = space.NewShowSpaceCommand
}

func (s *ShowSuite) getDefaultSpace() params.ShowSpaceResult {
	return params.ShowSpaceResult{
		Space: params.Space{
			Id:   "1",
			Name: "default",
			Subnets: []params.Subnet{{
				CIDR:       "4.3.2.0/28",
				ProviderId: "abc",
				VLANTag:    0,
			}},
		},
		Applications: []string{"ubuntu,mysql"},
		MachineCount: 4,
	}
}

func (s *ShowSuite) TestRunShowSpaceSucceeds(c *tc.C) {
	ctrl, api := setUpMocks(c)
	defer ctrl.Finish()
	spaceName := "default"
	expectedStdout := `space:
  id: "1"
  name: default
  subnets:
  - cidr: 4.3.2.0/28
    provider-id: abc
    vlan-tag: 0
applications:
- ubuntu,mysql
machine-count: 4
`
	api.EXPECT().ShowSpace(gomock.Any(), spaceName).Return(s.getDefaultSpace(), nil)

	ctx, err := s.runCommand(c, api, spaceName)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, expectedStdout)

}
func (s *ShowSuite) runCommand(c *tc.C, api space.API, name string) (*cmd.Context, error) {
	base := space.NewSpaceCommandBase(api)
	command := space.ShowSpaceCommand{
		SpaceCommandBase: base,
		Name:             "",
	}
	return cmdtesting.RunCommand(c, &command, name)
}

func (s *ShowSuite) TestRunWhenShowSpacesNotSupported(c *tc.C) {
	ctrl, api := setUpMocks(c)
	defer ctrl.Finish()
	spaceName := "default"

	expectedErr := errors.NewNotSupported(nil, "spaces not supported")
	api.EXPECT().ShowSpace(gomock.Any(), spaceName).Return(params.ShowSpaceResult{}, expectedErr)

	_, err := s.runCommand(c, api, spaceName)

	c.Assert(err, tc.ErrorIs, errors.NotSupported)
}

func (s *ShowSuite) TestRunWhenShowSpacesAPIFails(c *tc.C) {
	ctrl, api := setUpMocks(c)
	defer ctrl.Finish()
	spaceName := "default"

	apiErr := errors.New("API error")
	api.EXPECT().ShowSpace(gomock.Any(), spaceName).Return(params.ShowSpaceResult{}, apiErr)

	_, err := s.runCommand(c, api, spaceName)
	expectedMsg := fmt.Sprintf("cannot retrieve space %q: API error", spaceName)
	c.Assert(err, tc.ErrorMatches, expectedMsg)
}

func (s *ShowSuite) TestRunUnauthorizedMentionsJujuGrant(c *tc.C) {
	apiErr := &params.Error{
		Message: "permission denied",
		Code:    params.CodeUnauthorized,
	}

	ctrl, api := setUpMocks(c)
	defer ctrl.Finish()
	spaceName := "default"
	api.EXPECT().ShowSpace(gomock.Any(), spaceName).Return(params.ShowSpaceResult{}, apiErr)

	_, err := s.runCommand(c, api, spaceName)
	expectedErrMsg := fmt.Sprintf("cannot retrieve space %q: permission denied", spaceName)
	c.Assert(err, tc.ErrorMatches, expectedErrMsg)

}

func (s *ShowSuite) TestRunWhenSpacesBlocked(c *tc.C) {
	apiErr := &params.Error{Code: params.CodeOperationBlocked, Message: "nope"}
	ctrl, api := setUpMocks(c)
	defer ctrl.Finish()

	spaceName := "default"
	api.EXPECT().ShowSpace(gomock.Any(), spaceName).Return(params.ShowSpaceResult{}, apiErr)
	ctx, err := s.runCommand(c, api, spaceName)

	c.Assert(err, tc.ErrorMatches, `
cannot retrieve space "default": nope

All operations that change model have been disabled for the current model.
To enable changes, run

    juju enable-command all

`[1:])
	c.Assert(ctx.Stderr.(*bytes.Buffer).String(), tc.Equals, "")
	c.Assert(ctx.Stdout.(*bytes.Buffer).String(), tc.Equals, "")
}
