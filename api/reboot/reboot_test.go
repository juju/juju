// Copyright 2014 Cloudbase Solutions
// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !ppc64

package reboot_test

import (
	stdtesting "testing"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/reboot"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

func TestAll(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type machineRebootSuite struct {
	testing.JujuConnSuite

	machine *state.Machine
	st      api.Connection
	reboot  *reboot.State
}

var _ = gc.Suite(&machineRebootSuite{})

func (s *machineRebootSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	var err error
	s.st, s.machine = s.OpenAPIAsNewMachine(c)
	s.reboot, err = s.st.Reboot()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.reboot, gc.NotNil)
}

func (s *machineRebootSuite) TestWatchForRebootEvent(c *gc.C) {
	reboot.PatchFacadeCall(s, s.reboot, func(facade string, p interface{}, resp interface{}) error {
		return nil
	})
	_, err := s.reboot.WatchForRebootEvent()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *machineRebootSuite) TestWatchForRebootEventError(c *gc.C) {
	reboot.PatchFacadeCall(s, s.reboot, func(facade string, p interface{}, resp interface{}) error {
		if resp, ok := resp.(*params.NotifyWatchResult); ok {
			resp.Error = &params.Error{
				Message: "Some error.",
				Code:    params.CodeNotAssigned,
			}
		}
		return nil
	})
	_, err := s.reboot.WatchForRebootEvent()
	c.Assert(err.Error(), gc.Equals, "Some error.")
}

func (s *machineRebootSuite) TestRequestReboot(c *gc.C) {
	reboot.PatchFacadeCall(s, s.reboot, func(facade string, p interface{}, resp interface{}) error {
		if entities, ok := p.(params.Entities); ok {
			if len(entities.Entities) != 1 {
				return errors.Errorf("Expected 1 machine, got: %d", len(entities.Entities))
			}
			if entities.Entities[0].Tag != s.machine.Tag().String() {
				return errors.Errorf("Expecting machineTag %s, got %s", entities.Entities[0].Tag, s.machine.Tag().String())
			}
		}
		if resp, ok := resp.(*params.ErrorResults); ok {
			resp.Results = []params.ErrorResult{
				{},
			}
		}
		return nil
	})
	err := s.reboot.RequestReboot()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *machineRebootSuite) TestRequestRebootError(c *gc.C) {
	reboot.PatchFacadeCall(s, s.reboot, func(facade string, p interface{}, resp interface{}) error {
		if entities, ok := p.(params.Entities); ok {
			if len(entities.Entities) != 1 {
				return errors.Errorf("Expected 1 machine, got: %d", len(entities.Entities))
			}
			if entities.Entities[0].Tag != s.machine.Tag().String() {
				return errors.Errorf("Expecting machineTag %s, got %s", entities.Entities[0].Tag, s.machine.Tag().String())
			}
		}
		if resp, ok := resp.(*params.ErrorResults); ok {
			resp.Results = []params.ErrorResult{
				{
					Error: &params.Error{
						Message: "Some error.",
						Code:    params.CodeNotAssigned,
					},
				},
			}
		}
		return nil
	})
	err := s.reboot.RequestReboot()
	c.Assert(err.Error(), gc.Equals, "Some error.")
}

func (s *machineRebootSuite) TestGetRebootAction(c *gc.C) {
	reboot.PatchFacadeCall(s, s.reboot, func(facade string, p interface{}, resp interface{}) error {
		if resp, ok := resp.(*params.RebootActionResults); ok {
			resp.Results = []params.RebootActionResult{
				{Result: params.ShouldDoNothing},
			}
		}
		return nil
	})
	rAction, err := s.reboot.GetRebootAction()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rAction, gc.Equals, params.ShouldDoNothing)
}

func (s *machineRebootSuite) TestGetRebootActionMultipleResults(c *gc.C) {
	reboot.PatchFacadeCall(s, s.reboot, func(facade string, p interface{}, resp interface{}) error {
		if resp, ok := resp.(*params.RebootActionResults); ok {
			resp.Results = []params.RebootActionResult{
				{Result: params.ShouldDoNothing},
				{Result: params.ShouldDoNothing},
			}
		}
		return nil
	})
	_, err := s.reboot.GetRebootAction()
	c.Assert(err.Error(), gc.Equals, "expected 1 result, got 2")
}

func (s *machineRebootSuite) TestClearReboot(c *gc.C) {
	reboot.PatchFacadeCall(s, s.reboot, func(facade string, p interface{}, resp interface{}) error {
		if entities, ok := p.(params.Entities); ok {
			if len(entities.Entities) != 1 {
				return errors.Errorf("Expected 1 machine, got: %d", len(entities.Entities))
			}
			if entities.Entities[0].Tag != s.machine.Tag().String() {
				return errors.Errorf("Expecting machineTag %s, got %s", entities.Entities[0].Tag, s.machine.Tag().String())
			}
		}
		if resp, ok := resp.(*params.ErrorResults); ok {
			resp.Results = []params.ErrorResult{
				{},
			}
		}
		return nil
	})
	err := s.reboot.ClearReboot()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *machineRebootSuite) TestClearRebootError(c *gc.C) {
	reboot.PatchFacadeCall(s, s.reboot, func(facade string, p interface{}, resp interface{}) error {
		if resp, ok := resp.(*params.ErrorResults); ok {
			resp.Results = []params.ErrorResult{
				{
					Error: &params.Error{
						Message: "Some error.",
						Code:    params.CodeNotAssigned,
					},
				},
			}
		}
		return nil
	})
	err := s.reboot.ClearReboot()
	c.Assert(err.Error(), gc.Equals, "Some error.")
}
