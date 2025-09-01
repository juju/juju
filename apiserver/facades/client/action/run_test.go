// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"errors"
	stdtesting "testing"

	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"

	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type runSuite struct {
	MockBaseSuite
}

func TestRunSuite(t *stdtesting.T) {
	tc.Run(t, &runSuite{})
}

func (s *runSuite) TestBlockRunOnAllMachines(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	client := s.NewActionAPI(c)

	// block all changes
	s.blockAllChanges(c, "TestBlockRunOnAllMachines")
	_, err := client.RunOnAllMachines(
		c.Context(),
		params.RunParams{
			Commands: "hostname",
			Timeout:  testing.LongWait,
		})
	s.assertBlocked(c, err, "TestBlockRunOnAllMachines")
}

func (s *runSuite) TestBlockRunMachineAndApplication(c *tc.C) {
	defer s.setupMocks(c).Finish()
	client := s.NewActionAPI(c)

	// block all changes
	s.blockAllChanges(c, "TestBlockRunMachineAndApplication")
	_, err := client.Run(
		c.Context(),
		params.RunParams{
			Commands:     "hostname",
			Timeout:      testing.LongWait,
			Machines:     []string{"0"},
			Applications: []string{"magic"},
		})
	s.assertBlocked(c, err, "TestBlockRunMachineAndApplication")
}

func (s *runSuite) TestStub(c *tc.C) {
	c.Skip(`This suite is missing tests for the following scenarios:
Running juju-exec with machine, application and unit targets.
Running juju-exec against all machines.
Running "Run" requires administrator privilege.
Running "RunOnAllMachines" requires administrator privilege.
`)
}

func (s *runSuite) assertBlocked(c *tc.C, err error, msg string) {
	c.Assert(params.IsCodeOperationBlocked(err), tc.IsTrue, tc.Commentf("error: %#v", err))
	var obtained *params.Error
	c.Assert(errors.As(err, &obtained), tc.IsTrue)
	c.Assert(obtained, tc.DeepEquals, &params.Error{
		Message: msg,
		Code:    "operation is blocked",
	})
}

func (s *runSuite) blockAllChanges(c *tc.C, msg string) {
	s.BlockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), gomock.Any()).Return(msg, nil)
}
