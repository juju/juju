// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/envcontext"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/state"
)

type PrecheckerSuite struct {
	ConnSuite
	prechecker *mockPrechecker
}

var _ = gc.Suite(&PrecheckerSuite{})

func (s *PrecheckerSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	s.prechecker = &mockPrechecker{}
}

func (s *PrecheckerSuite) TestPrecheckInstance(c *gc.C) {
	// PrecheckInstance should be called with the specified
	// series and no placement, and the specified constraints
	// merged with the model constraints, when attempting
	// to create an instance.
	modelCons := constraints.MustParse("mem=4G")
	placement := ""
	template, err := s.addOneMachine(c, s.prechecker, modelCons, placement)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.prechecker.precheckInstanceArgs.Base.String(), gc.Equals, template.Base.String())
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
	template, err := s.addOneMachine(c, s.prechecker, modelCons, placement)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.prechecker.precheckInstanceArgs.Base.String(), gc.Equals, template.Base.String())
	c.Assert(s.prechecker.precheckInstanceArgs.Placement, gc.Equals, placement)
	c.Assert(s.prechecker.precheckInstanceArgs.Constraints, gc.DeepEquals, template.Constraints)
}

func (s *PrecheckerSuite) TestPrecheckErrors(c *gc.C) {
	// Ensure that AddOneMachine fails when PrecheckInstance returns an error.
	s.prechecker.precheckInstanceError = fmt.Errorf("no instance for you")
	_, err := s.addOneMachine(c, s.prechecker, constraints.Value{}, "placement")
	c.Assert(err, gc.ErrorMatches, ".*no instance for you")
}

func (s *PrecheckerSuite) TestPrecheckInstanceInjectMachine(c *gc.C) {
	template := state.MachineTemplate{
		InstanceId: instance.Id("bootstrap"),
		Base:       state.UbuntuBase("22.04"),
		Nonce:      agent.BootstrapNonce,
		Jobs:       []state.MachineJob{state.JobManageModel},
		Placement:  "anyoldthing",
	}
	_, err := s.State.AddOneMachine(s.prechecker, template)
	c.Assert(err, jc.ErrorIsNil)
	// PrecheckInstance should not have been called, as we've
	// injected a machine with an existing instance.
	c.Assert(s.prechecker.precheckInstanceArgs.Base.String(), gc.Equals, "")
	c.Assert(s.prechecker.precheckInstanceArgs.Placement, gc.Equals, "")
}

func (s *PrecheckerSuite) TestPrecheckContainerNewMachine(c *gc.C) {
	// Attempting to add a container to a new machine should cause
	// PrecheckInstance to be called.
	template := state.MachineTemplate{
		Base:      state.UbuntuBase("22.04"),
		Jobs:      []state.MachineJob{state.JobHostUnits},
		Placement: "intertubes",
	}
	_, err := s.State.AddMachineInsideNewMachine(s.prechecker, template, template, instance.LXD)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.prechecker.precheckInstanceArgs.Base.String(), gc.Equals, template.Base.String())
	c.Assert(s.prechecker.precheckInstanceArgs.Placement, gc.Equals, template.Placement)
}

func (s *PrecheckerSuite) TestPrecheckAddApplication(c *gc.C) {
	// Deploy an application for the purpose of creating a
	// storage instance. We'll then destroy the unit and detach
	// the storage, so that it can be attached to a new
	// application unit.
	ch := s.AddTestingCharm(c, "storage-block")
	app, err := s.State.AddApplication(s.prechecker, state.AddApplicationArgs{
		Name:  "storage-block",
		Charm: ch,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "20.04/stable",
		}},
		NumUnits: 1,
		Storage: map[string]state.StorageConstraints{
			"data":    {Count: 1, Pool: "modelscoped"},
			"allecto": {Count: 1, Pool: "modelscoped"},
		},
	}, mockApplicationSaver{}, state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)

	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = unit.AssignToNewMachine(s.prechecker)
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

	err = unit.Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
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

	_, err = s.State.AddApplication(s.prechecker, state.AddApplicationArgs{
		Name:  "storage-block-the-second",
		Charm: ch,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "20.04/stable",
		}},
		NumUnits: 1,
		Placement: []*instance.Placement{{
			Scope:     s.State.ModelUUID(),
			Directive: "whatever",
		}},
		AttachStorage: storageTags,
	}, mockApplicationSaver{}, state.NewObjectStore(c, s.State.ModelUUID()))
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
	_, err := s.State.AddApplication(s.prechecker, state.AddApplicationArgs{
		Name:  "wordpress",
		Charm: ch,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "12.10/stable",
		}},
		NumUnits:    1,
		Constraints: constraints.MustParse("root-disk=20G"),
	}, mockApplicationSaver{}, state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, gc.ErrorMatches, `cannot add application "wordpress": failed for some reason`)
	c.Assert(s.prechecker.precheckInstanceArgs, jc.DeepEquals, environs.PrecheckInstanceParams{
		Base:        corebase.MakeDefaultBase("ubuntu", "12.10"),
		Constraints: constraints.MustParse("arch=amd64 root-disk=20G"),
	})
}

func (s *PrecheckerSuite) TestPrecheckAddApplicationAllMachinePlacement(c *gc.C) {
	m1, _, err := s.addMachine(c, s.prechecker, constraints.MustParse(""), "")
	c.Assert(err, jc.ErrorIsNil)
	m2, _, err := s.addMachine(c, s.prechecker, constraints.MustParse(""), "")
	c.Assert(err, jc.ErrorIsNil)

	// Make sure the prechecker isn't called.
	s.prechecker.precheckInstanceError = errors.Errorf("boom!")

	ch := s.AddTestingCharm(c, "wordpress")
	_, err = s.State.AddApplication(s.prechecker, state.AddApplicationArgs{
		Name: "wordpress",
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "20.04/stable",
		}},
		Charm:    ch,
		NumUnits: 2,
		Placement: []*instance.Placement{
			instance.MustParsePlacement(m1.Id()),
			instance.MustParsePlacement(m2.Id()),
		},
	}, mockApplicationSaver{}, state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *PrecheckerSuite) TestPrecheckAddApplicationMixedPlacement(c *gc.C) {
	m1, _, err := s.addMachine(c, s.prechecker, constraints.MustParse(""), "")
	c.Assert(err, jc.ErrorIsNil)

	// Make sure the prechecker still gets called if there's a machine
	// placement and a directive that needs to be passed to the
	// provider.

	s.prechecker.precheckInstanceError = errors.Errorf("hey now")
	ch := s.AddTestingCharm(c, "wordpress")
	_, err = s.State.AddApplication(s.prechecker, state.AddApplicationArgs{
		Name: "wordpress",
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "20.04/stable",
		}},
		Charm:    ch,
		NumUnits: 2,
		Placement: []*instance.Placement{
			{Scope: instance.MachineScope, Directive: m1.Id()},
			{Scope: s.State.ModelUUID(), Directive: "somewhere"},
		},
	}, mockApplicationSaver{}, state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, gc.ErrorMatches, `cannot add application "wordpress": hey now`)
	c.Assert(s.prechecker.precheckInstanceArgs, jc.DeepEquals, environs.PrecheckInstanceParams{
		Base:        corebase.MakeDefaultBase("ubuntu", "20.04"),
		Placement:   "somewhere",
		Constraints: constraints.MustParse("arch=amd64"),
	})
}

func (s *PrecheckerSuite) addOneMachine(c *gc.C, prechecker environs.InstancePrechecker, modelCons constraints.Value, placement string) (state.MachineTemplate, error) {
	_, template, err := s.addMachine(c, prechecker, modelCons, placement)
	return template, err
}

func (s *PrecheckerSuite) addMachine(c *gc.C, prechecker environs.InstancePrechecker, modelCons constraints.Value, placement string) (*state.Machine, state.MachineTemplate, error) {
	err := s.State.SetModelConstraints(modelCons)
	c.Assert(err, jc.ErrorIsNil)
	oneJob := []state.MachineJob{state.JobHostUnits}
	extraCons := constraints.MustParse("cores=4")
	template := state.MachineTemplate{
		Base:        state.UbuntuBase("20.04"),
		Constraints: extraCons,
		Jobs:        oneJob,
		Placement:   placement,
	}
	machine, err := s.State.AddOneMachine(prechecker, template)
	return machine, template, err
}

type mockPrechecker struct {
	precheckInstanceError error
	precheckInstanceArgs  environs.PrecheckInstanceParams
}

func (p *mockPrechecker) PrecheckInstance(ctx envcontext.ProviderCallContext, args environs.PrecheckInstanceParams) error {
	p.precheckInstanceArgs = args
	return p.precheckInstanceError
}
