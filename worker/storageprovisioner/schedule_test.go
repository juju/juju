// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner_test

import (
	"time"

	"github.com/benbjohnson/clock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/storage"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/storageprovisioner"
	"github.com/juju/names"
)

type scheduleSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&scheduleSuite{})

func (s *scheduleSuite) TestNextNoEvents(c *gc.C) {
	s.testOps(c, assertNothingNextOp{})
}

func (s *scheduleSuite) TestNext(c *gc.C) {
	s.testOps(c,
		addVolumeOp{"0", 3 * time.Second},
		addVolumeOp{"1", 1500 * time.Millisecond},
		addVolumeOp{"2", 2 * time.Second},
		addVolumeOp{"4", 2500 * time.Millisecond},

		assertNextOp(1500*time.Millisecond),
		addTimeOp(1500*time.Millisecond),
		assertReadyVolumesOp{"1"},

		addTimeOp(500*time.Millisecond),
		assertNextOp(0),
		assertReadyVolumesOp{"2"},

		removeVolumeOp("4"),

		addTimeOp(2*time.Second), // T+4
		assertNextOp(0),
		assertReadyVolumesOp{"0"},
	)
}

func (*scheduleSuite) TestReadyNoEvents(c *gc.C) {
	s := storageprovisioner.NewSchedule(storageprovisioner.WallClock)
	volumes, volumeAttachments := s.Ready(time.Now())
	c.Assert(volumes, gc.HasLen, 0)
	c.Assert(volumeAttachments, gc.HasLen, 0)
}

func (s *scheduleSuite) TestAddVolume(c *gc.C) {
	s.testOps(c,
		addVolumeOp{"0", 3 * time.Second},
		addVolumeOp{"1", 1500 * time.Millisecond},
		addVolumeOp{"2", 2 * time.Second},
		addTimeOp(time.Second), // T+1
		assertReadyVolumesOp{},
		addTimeOp(time.Second), // T+2
		assertReadyVolumesOp{"1", "2"},
		assertReadyVolumesOp{},
		addTimeOp(500*time.Millisecond), // T+2.5
		assertReadyVolumesOp{},
		addTimeOp(time.Second), // T+3.5
		assertReadyVolumesOp{"0"},
	)
}

func (s *scheduleSuite) TestRemoveVolume(c *gc.C) {
	s.testOps(c,
		addVolumeOp{"0", 3 * time.Second},
		addVolumeOp{"1", 2 * time.Second},
		removeVolumeOp("0"),
		assertReadyVolumesOp{},
		addTimeOp(3*time.Second),
		assertReadyVolumesOp{"1"},
	)
}

func (s *scheduleSuite) TestRemoveVolumeNotFound(c *gc.C) {
	s.testOps(c,
		removeVolumeOp("0"), // does not explode
	)
}

func (s *scheduleSuite) TestAddRemoveVolumeAttachment(c *gc.C) {
	s.testOps(c,
		removeVolumeAttachmentOp{"machine-0", "volume-0"}, // does not explode

		addVolumeAttachmentOp{
			"0", // machine-0
			"0", // volume-0
			time.Second,
		},
		removeVolumeAttachmentOp{"machine-0", "volume-0"},

		addVolumeAttachmentOp{
			"0", // machine-0
			"1", // volume-0
			time.Second,
		},

		addTimeOp(time.Second),
		assertReadyVolumeAttachmentsOp{{
			"machine-0", "volume-1",
		}},
		assertReadyVolumeAttachmentsOp{},
	)
}

func (s *scheduleSuite) testOps(c *gc.C, ops ...scheduleTestOp) {
	ctx := newScheduleTestContext(c)
	ctx.run(ops...)
}

type scheduleTestContext struct {
	c        *gc.C
	clock    *clock.Mock
	schedule storageprovisioner.Schedule
}

func newScheduleTestContext(c *gc.C) *scheduleTestContext {
	clock := clock.NewMock()
	schedule := storageprovisioner.NewSchedule(clock)
	return &scheduleTestContext{c, clock, schedule}
}

type scheduleTestOp interface {
	apply(*scheduleTestContext)
}

func (ctx *scheduleTestContext) run(ops ...scheduleTestOp) {
	for _, op := range ops {
		op.apply(ctx)
	}
}

type addVolumeOp struct {
	name  string
	delay time.Duration // delay from ctx.clock.Now()
}

func (op addVolumeOp) apply(ctx *scheduleTestContext) {
	v := storage.VolumeParams{Tag: names.NewVolumeTag(op.name)}
	ctx.schedule.AddVolume(v, ctx.clock.Now().Add(op.delay))
}

type removeVolumeOp string

func (op removeVolumeOp) apply(ctx *scheduleTestContext) {
	ctx.schedule.RemoveVolume(names.NewVolumeTag(string(op)))
}

type addVolumeAttachmentOp struct {
	machineId  string
	volumeName string
	delay      time.Duration // delay from ctx.clock.Now()
}

func (op addVolumeAttachmentOp) apply(ctx *scheduleTestContext) {
	a := storage.VolumeAttachmentParams{
		AttachmentParams: storage.AttachmentParams{
			Machine: names.NewMachineTag(op.machineId),
		},
		Volume: names.NewVolumeTag(op.volumeName),
	}
	ctx.schedule.AddVolumeAttachment(a, ctx.clock.Now().Add(op.delay))
}

type removeVolumeAttachmentOp params.MachineStorageId

func (op removeVolumeAttachmentOp) apply(ctx *scheduleTestContext) {
	ctx.schedule.RemoveVolumeAttachment(params.MachineStorageId(op))
}

type addTimeOp time.Duration

func (op addTimeOp) apply(ctx *scheduleTestContext) {
	ctx.clock.Add(time.Duration(op))
}

type assertNothingNextOp struct{}

func (assertNothingNextOp) apply(ctx *scheduleTestContext) {
	next := ctx.schedule.Next()
	ctx.c.Assert(next, gc.IsNil)
}

type assertNextOp time.Duration

func (op assertNextOp) apply(ctx *scheduleTestContext) {
	next := ctx.schedule.Next()
	ctx.c.Assert(next, gc.NotNil)
	if op > 0 {
		select {
		case <-next:
			ctx.c.Fatal("Next channel signalled too soon")
		default:
		}
	}

	// temporarily move time forward
	ctx.clock.Add(time.Duration(op))
	defer ctx.clock.Add(-time.Duration(op))

	select {
	case _, ok := <-next:
		ctx.c.Assert(ok, jc.IsTrue)
		// the time value is unimportant to us
	default:
		ctx.c.Fatal("Next channel not signalled")
	}
}

type assertReadyVolumesOp []string

func (op assertReadyVolumesOp) apply(ctx *scheduleTestContext) {
	volumes, _ := ctx.schedule.Ready(ctx.clock.Now())
	names := make([]string, len(volumes))
	for i, v := range volumes {
		names[i] = v.Tag.Id()
	}
	ctx.c.Assert(names, jc.DeepEquals, []string(op))
}

type assertReadyVolumeAttachmentsOp []params.MachineStorageId

func (op assertReadyVolumeAttachmentsOp) apply(ctx *scheduleTestContext) {
	_, volumeAttachments := ctx.schedule.Ready(ctx.clock.Now())
	ids := make([]params.MachineStorageId, len(volumeAttachments))
	for i, a := range volumeAttachments {
		ids[i] = params.MachineStorageId{
			MachineTag:    a.Machine.String(),
			AttachmentTag: a.Volume.String(),
		}
	}
	ctx.c.Assert(ids, jc.DeepEquals, []params.MachineStorageId(op))
}
