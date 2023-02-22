// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/client/annotations"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing/factory"
)

type annotationsSuite struct {
	jujutesting.JujuConnSuite
	annotationsClient *annotations.Client
}

func (s *annotationsSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.annotationsClient = annotations.NewClient(s.APIState)
	c.Assert(s.annotationsClient, gc.NotNil)
}

func (s *annotationsSuite) TearDownTest(c *gc.C) {
	_ = s.annotationsClient.ClientFacade.Close()
	s.JujuConnSuite.TearDownTest(c)
}

func (s *annotationsSuite) TestAnnotationFacadeCall(c *gc.C) {
	machine := s.Factory.MakeMachine(c, &factory.MachineParams{
		Base: state.UbuntuBase("12.10"),
		Jobs: []state.MachineJob{state.JobHostUnits},
	})
	machineTag := machine.Tag().String()

	annts := map[string]string{"annotation": "test"}
	callErrs, err := s.annotationsClient.Set(
		map[string]map[string]string{
			machineTag: annts,
		})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(callErrs, gc.HasLen, 0)

	found, err := s.annotationsClient.Get([]string{machineTag})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found, gc.HasLen, 1)

	firstFound := found[0]
	c.Assert(firstFound.EntityTag, gc.Equals, machineTag)
	c.Assert(firstFound.Annotations, gc.DeepEquals, annts)
	c.Assert(firstFound.Error.Error, gc.IsNil)
}

func (s *annotationsSuite) TestSetCallGettingErrors(c *gc.C) {
	machine := s.Factory.MakeMachine(c, &factory.MachineParams{
		Base: state.UbuntuBase("12.10"),
		Jobs: []state.MachineJob{state.JobHostUnits},
	})
	machineTag := machine.Tag().String()

	annts := map[string]string{"invalid.key": "test"}
	callErrs, err := s.annotationsClient.Set(
		map[string]map[string]string{
			machineTag: annts,
		})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(callErrs, gc.HasLen, 1)
	c.Assert(callErrs[0].Error.Error(), gc.Matches, `.*: invalid key "invalid.key"`)

	found, err := s.annotationsClient.Get([]string{machineTag})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found, gc.HasLen, 1)

	firstFound := found[0]
	c.Assert(firstFound.EntityTag, gc.Equals, machineTag)
	c.Assert(firstFound.Annotations, gc.HasLen, 0)
	c.Assert(firstFound.Error.Error, gc.IsNil)
}
