// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/state"
	"github.com/juju/juju/storage"
)

type PrecheckerSuite struct {
	ConnSuite
	prechecker mockPrechecker
}

var _ = gc.Suite(&PrecheckerSuite{})

type mockPrechecker struct {
	precheckInstanceError error
	precheckInstanceArgs  environs.PrecheckInstanceParams
}

func (p *mockPrechecker) PrecheckInstance(ctx context.ProviderCallContext, args environs.PrecheckInstanceParams) error {
	p.precheckInstanceArgs = args
	return p.precheckInstanceError
}

func (s *PrecheckerSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	s.prechecker = mockPrechecker{}
	s.policy.GetPrechecker = func() (environs.InstancePrechecker, error) {
		return &s.prechecker, nil
	}
}

func (s *PrecheckerSuite) TestPrecheckInstance(c *gc.C) {
	// PrecheckInstance should be called with the specified
	// series and no placement, and the specified constraints
	// merged with the model constraints, when attempting
	// to create an instance.
	modelCons := constraints.MustParse("mem=4G")
	placement := ""
	template, err := s.addOneMachine(c, modelCons, placement)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.prechecker.precheckInstanceArgs.Series, gc.Equals, template.Series)
	c.Assert(s.prechecker.precheckInstanceArgs.Placement, gc.Equals, placement)
	validator := constraints.NewValidator()
	cons, err := validator.Merge(modelCons, template.Constraints)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.prechecker.precheckInstanceArgs.Constraints, gc.DeepEquals, cons)
}

func (s *PrecheckerSuite) TestPrecheckInstanceWithPlacement(c *gc.C) {
	// PrecheckInstance should be called with the specified
	// series and placement. If placement is provided all
	// model constraints should be ignored, otherwise they
	// should be merged with provided constraints, when
	// attempting to create an instance
	modelCons := constraints.MustParse("mem=4G")
	placement := "abc123"
	template, err := s.addOneMachine(c, modelCons, placement)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.prechecker.precheckInstanceArgs.Series, gc.Equals, template.Series)
	c.Assert(s.prechecker.precheckInstanceArgs.Placement, gc.Equals, placement)
	c.Assert(s.prechecker.precheckInstanceArgs.Constraints, gc.DeepEquals, template.Constraints)
}

func (s *PrecheckerSuite) TestPrecheckErrors(c *gc.C) {
	// Ensure that AddOneMachine fails when PrecheckInstance returns an error.
	s.prechecker.precheckInstanceError = fmt.Errorf("no instance for you")
	_, err := s.addOneMachine(c, constraints.Value{}, "placement")
	c.Assert(err, gc.ErrorMatches, ".*no instance for you")

	// If the policy's Prechecker method fails, that will be returned first.
	s.policy.GetPrechecker = func() (environs.InstancePrechecker, error) {
		return nil, fmt.Errorf("no prechecker for you")
	}
	_, err = s.addOneMachine(c, constraints.Value{}, "placement")
	c.Assert(err, gc.ErrorMatches, ".*no prechecker for you")
}

func (s *PrecheckerSuite) TestPrecheckPrecheckerUnimplemented(c *gc.C) {
	var precheckerErr error
	s.policy.GetPrechecker = func() (environs.InstancePrechecker, error) {
		return nil, precheckerErr
	}
	_, err := s.addOneMachine(c, constraints.Value{}, "placement")
	c.Assert(err, gc.ErrorMatches, "cannot add a new machine: policy returned nil prechecker without an error")
	precheckerErr = errors.NotImplementedf("Prechecker")
	_, err = s.addOneMachine(c, constraints.Value{}, "placement")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *PrecheckerSuite) TestPrecheckNoPolicy(c *gc.C) {
	s.policy.GetPrechecker = func() (environs.InstancePrechecker, error) {
		c.Errorf("should not have been invoked")
		return nil, nil
	}
	state.SetPolicy(s.State, nil)
	_, err := s.addOneMachine(c, constraints.Value{}, "placement")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *PrecheckerSuite) addOneMachine(c *gc.C, modelCons constraints.Value, placement string) (state.MachineTemplate, error) {
	_, template, err := s.addMachine(c, modelCons, placement)
	return template, err
}

func (s *PrecheckerSuite) addMachine(c *gc.C, modelCons constraints.Value, placement string) (*state.Machine, state.MachineTemplate, error) {
	err := s.State.SetModelConstraints(modelCons)
	c.Assert(err, jc.ErrorIsNil)
	oneJob := []state.MachineJob{state.JobHostUnits}
	extraCons := constraints.MustParse("cores=4")
	template := state.MachineTemplate{
		Series:      "precise",
		Constraints: extraCons,
		Jobs:        oneJob,
		Placement:   placement,
	}
	machine, err := s.State.AddOneMachine(template)
	return machine, template, err
}

func (s *PrecheckerSuite) TestPrecheckInstanceInjectMachine(c *gc.C) {
	template := state.MachineTemplate{
		InstanceId: instance.Id("bootstrap"),
		Series:     "precise",
		Nonce:      agent.BootstrapNonce,
		Jobs:       []state.MachineJob{state.JobManageModel},
		Placement:  "anyoldthing",
	}
	_, err := s.State.AddOneMachine(template)
	c.Assert(err, jc.ErrorIsNil)
	// PrecheckInstance should not have been called, as we've
	// injected a machine with an existing instance.
	c.Assert(s.prechecker.precheckInstanceArgs.Series, gc.Equals, "")
	c.Assert(s.prechecker.precheckInstanceArgs.Placement, gc.Equals, "")
}

func (s *PrecheckerSuite) TestPrecheckContainerNewMachine(c *gc.C) {
	// Attempting to add a container to a new machine should cause
	// PrecheckInstance to be called.
	template := state.MachineTemplate{
		Series:    "precise",
		Jobs:      []state.MachineJob{state.JobHostUnits},
		Placement: "intertubes",
	}
	_, err := s.State.AddMachineInsideNewMachine(template, template, instance.LXD)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.prechecker.precheckInstanceArgs.Series, gc.Equals, template.Series)
	c.Assert(s.prechecker.precheckInstanceArgs.Placement, gc.Equals, template.Placement)
}

func (s *PrecheckerSuite) TestPrecheckAddApplication(c *gc.C) {
	// Deploy an application for the purpose of creating a
	// storage instance. We'll then destroy the unit and detach
	// the storage, so that it can be attached to a new
	// application unit.
	ch := s.AddTestingCharm(c, "storage-block")
	app, err := s.State.AddApplication(state.AddApplicationArgs{
		Name:     "storage-block",
		Charm:    ch,
		NumUnits: 1,
		Storage: map[string]state.StorageConstraints{
			"data":    {Count: 1, Pool: "modelscoped"},
			"allecto": {Count: 1, Pool: "modelscoped"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = unit.AssignToNewMachine()
	c.Assert(err, jc.ErrorIsNil)
	machineId, err := unit.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	machineTag := names.NewMachineTag(machineId)

	sb, err := state.NewStorageBackend(s.State)
	c.Assert(err, jc.ErrorIsNil)
	storageAttachments, err := sb.UnitStorageAttachments(unit.UnitTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(storageAttachments, gc.HasLen, 2)
	storageTags := []names.StorageTag{
		storageAttachments[0].StorageInstance(),
		storageAttachments[1].StorageInstance(),
	}

	volumeTags := make([]names.VolumeTag, len(storageTags))
	for i, storageTag := range storageTags {
		volume, err := sb.StorageInstanceVolume(storageTag)
		c.Assert(err, jc.ErrorIsNil)
		volumeTags[i] = volume.VolumeTag()
	}
	// Provision only the first volume.
	err = sb.SetVolumeInfo(volumeTags[0], state.VolumeInfo{
		VolumeId: "foo",
		Pool:     "modelscoped",
	})
	c.Assert(err, jc.ErrorIsNil)

	err = unit.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	for _, storageTag := range storageTags {
		err = sb.DetachStorage(storageTag, unit.UnitTag(), false, dontWait)
		c.Assert(err, jc.ErrorIsNil)
	}
	for _, volumeTag := range volumeTags {
		err = sb.DetachVolume(machineTag, volumeTag, false)
		c.Assert(err, jc.ErrorIsNil)
		err = sb.RemoveVolumeAttachment(machineTag, volumeTag, false)
		c.Assert(err, jc.ErrorIsNil)
	}

	_, err = s.State.AddApplication(state.AddApplicationArgs{
		Name:     "storage-block-the-second",
		Charm:    ch,
		NumUnits: 1,
		Placement: []*instance.Placement{{
			Scope:     s.State.ModelUUID(),
			Directive: "whatever",
		}},
		AttachStorage: storageTags,
	})
	c.Assert(err, jc.ErrorIsNil)

	// The volume corresponding to the provisioned storage volume (only)
	// should be presented to PrecheckInstance. The unprovisioned volume
	// will be provisioned later by the storage provisioner.
	c.Assert(s.prechecker.precheckInstanceArgs.Placement, gc.Equals, "whatever")
	c.Assert(s.prechecker.precheckInstanceArgs.VolumeAttachments, jc.DeepEquals, []storage.VolumeAttachmentParams{{
		AttachmentParams: storage.AttachmentParams{
			Provider: "modelscoped",
		},
		Volume:   volumeTags[0],
		VolumeId: "foo",
	}})
}

func (s *PrecheckerSuite) TestPrecheckAddApplicationNoPlacement(c *gc.C) {
	s.prechecker.precheckInstanceError = errors.Errorf("failed for some reason")
	ch := s.AddTestingCharm(c, "wordpress")
	_, err := s.State.AddApplication(state.AddApplicationArgs{
		Name:        "wordpress",
		Charm:       ch,
		NumUnits:    1,
		Constraints: constraints.MustParse("root-disk=20G"),
	})
	c.Assert(err, gc.ErrorMatches, `cannot add application "wordpress": failed for some reason`)
	c.Assert(s.prechecker.precheckInstanceArgs, jc.DeepEquals, environs.PrecheckInstanceParams{
		Series:      "quantal",
		Constraints: constraints.MustParse("root-disk=20G"),
	})
}

func (s *PrecheckerSuite) TestPrecheckAddApplicationAllMachinePlacement(c *gc.C) {
	m1, _, err := s.addMachine(c, constraints.MustParse(""), "")
	c.Assert(err, jc.ErrorIsNil)
	m2, _, err := s.addMachine(c, constraints.MustParse(""), "")
	c.Assert(err, jc.ErrorIsNil)

	// Make sure the prechecker isn't called.
	s.prechecker.precheckInstanceError = errors.Errorf("boom!")

	ch := s.AddTestingCharm(c, "wordpress")
	_, err = s.State.AddApplication(state.AddApplicationArgs{
		Name:     "wordpress",
		Series:   "precise",
		Charm:    ch,
		NumUnits: 2,
		Placement: []*instance.Placement{
			instance.MustParsePlacement(m1.Id()),
			instance.MustParsePlacement(m2.Id()),
		},
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *PrecheckerSuite) TestPrecheckAddApplicationMixedPlacement(c *gc.C) {
	m1, _, err := s.addMachine(c, constraints.MustParse(""), "")
	c.Assert(err, jc.ErrorIsNil)

	// Make sure the prechecker still gets called if there's a machine
	// placement and a directive that needs to be passed to the
	// provider.

	s.prechecker.precheckInstanceError = errors.Errorf("hey now")
	ch := s.AddTestingCharm(c, "wordpress")
	_, err = s.State.AddApplication(state.AddApplicationArgs{
		Name:     "wordpress",
		Series:   "precise",
		Charm:    ch,
		NumUnits: 2,
		Placement: []*instance.Placement{
			{Scope: instance.MachineScope, Directive: m1.Id()},
			{Scope: s.State.ModelUUID(), Directive: "somewhere"},
		},
	})
	c.Assert(err, gc.ErrorMatches, `cannot add application "wordpress": hey now`)
	c.Assert(s.prechecker.precheckInstanceArgs, jc.DeepEquals, environs.PrecheckInstanceParams{
		Series:    "precise",
		Placement: "somewhere",
	})
}
