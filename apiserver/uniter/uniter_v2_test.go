// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/apiserver/uniter"
	"github.com/juju/juju/state"
	jujufactory "github.com/juju/juju/testing/factory"
)

//TODO run all common V0 and V1 tests.
type uniterV2Suite struct {
	uniterBaseSuite
	uniter *uniter.UniterAPIV2
}

var _ = gc.Suite(&uniterV2Suite{})

func (s *uniterV2Suite) SetUpTest(c *gc.C) {
	s.uniterBaseSuite.setUpTest(c)

	uniterAPIV2, err := uniter.NewUniterAPIV2(
		s.State,
		s.resources,
		s.authorizer,
	)
	c.Assert(err, jc.ErrorIsNil)
	s.uniter = uniterAPIV2
}

func (s *uniterV2Suite) TestStorageAttachments(c *gc.C) {
	// We need to set up a unit that has storage metadata defined.
	ch := s.AddTestingCharm(c, "storage-block")
	sCons := map[string]state.StorageConstraints{
		"data": {Pool: "", Size: 1024, Count: 1},
	}
	service := s.AddTestingServiceWithStorage(c, "storage-block", ch, sCons)
	factory := jujufactory.NewFactory(s.State)
	unit := factory.MakeUnit(c, &jujufactory.UnitParams{
		Service: service,
	})

	stateStorageAttachments, err := s.State.StorageAttachments(unit.UnitTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(stateStorageAttachments, gc.HasLen, 1)
	err = s.State.SetStorageAttachmentInfo(
		stateStorageAttachments[0].StorageInstance(),
		unit.UnitTag(),
		state.StorageAttachmentInfo{Location: "outerspace"},
	)
	c.Assert(err, jc.ErrorIsNil)

	password, err := utils.RandomPassword()
	err = unit.SetPassword(password)
	c.Assert(err, jc.ErrorIsNil)
	st := s.OpenAPIAs(c, unit.Tag(), password)
	uniter, err := st.Uniter()
	c.Assert(err, jc.ErrorIsNil)

	attachments, err := uniter.StorageAttachments(unit.Tag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(attachments, gc.DeepEquals, []params.StorageAttachment{{
		StorageTag: "storage-data-0",
		OwnerTag:   unit.Tag().String(),
		UnitTag:    unit.Tag().String(),
		Kind:       params.StorageKindBlock,
		Location:   "outerspace",
	}})
}

// TestSetStatus tests backwards compatibility for
// set status has been properly implemented.
func (s *uniterV2Suite) TestSetStatus(c *gc.C) {
	s.testSetStatus(c, s.uniter)
}

// TestSetAgentStatus tests agent part of set status
// implemented for this version.
func (s *uniterV2Suite) TestSetAgentStatus(c *gc.C) {
	s.testSetAgentStatus(c, s.uniter)
}

// TestSetUnitStatus tests unit part of set status
// implemented for this version.
func (s *uniterV2Suite) TestSetUnitStatus(c *gc.C) {
	s.testSetUnitStatus(c, s.uniter)
}
