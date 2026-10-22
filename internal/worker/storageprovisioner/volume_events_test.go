// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner

import (
	"testing"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/names/v6"
	"github.com/juju/tc"

	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/worker/storageprovisioner/internal/schedule"
	"github.com/juju/juju/rpc/params"
)

type volumeEventsSuite struct{}

func TestVolumeEventsSuite(t *testing.T) {
	tc.Run(t, &volumeEventsSuite{})
}

func (*volumeEventsSuite) TestUpdatesPendingVolumeOperation(c *tc.C) {
	clock := testclock.NewClock(time.Time{})
	machine := names.NewMachineTag("0")
	volume := names.NewVolumeTag("0")
	deps := dependencies{
		config:   Config{Clock: clock, Logger: loggertesting.WrapCheckLog(c)},
		schedule: schedule.NewSchedule(clock),
	}
	first := storage.VolumeParams{
		Tag: volume,
		Attachment: &storage.VolumeAttachmentParams{
			AttachmentParams: storage.AttachmentParams{
				Machine:    machine,
				InstanceId: "instance-1",
			},
			Volume: volume,
		},
	}
	updated := first
	updated.Attachment = &storage.VolumeAttachmentParams{
		AttachmentParams: storage.AttachmentParams{
			Machine:    machine,
			InstanceId: "instance-2",
		},
		Volume: volume,
	}

	updatePendingVolume(c.Context(), &deps, first)
	updatePendingVolume(c.Context(), &deps, updated)

	ready := deps.schedule.Ready(clock.Now())
	c.Assert(ready, tc.HasLen, 1)
	op, ok := ready[0].(*createVolumeOp)
	c.Assert(ok, tc.IsTrue)
	c.Check(op.args, tc.DeepEquals, updated)
}

func (*volumeEventsSuite) TestUpdatesPendingVolumeAttachmentOperation(c *tc.C) {
	clock := testclock.NewClock(time.Time{})
	machine := names.NewMachineTag("0")
	volume := names.NewVolumeTag("0")
	id := params.MachineStorageId{
		MachineTag:    machine.String(),
		AttachmentTag: volume.String(),
	}
	deps := dependencies{
		config:   Config{Clock: clock, Logger: loggertesting.WrapCheckLog(c)},
		schedule: schedule.NewSchedule(clock),
	}
	first := storage.VolumeAttachmentParams{
		AttachmentParams: storage.AttachmentParams{
			Machine:    machine,
			InstanceId: "instance-1",
		},
		Volume:   volume,
		VolumeId: "volume-1",
	}
	updated := first
	updated.InstanceId = "instance-2"
	updated.VolumeId = "volume-2"

	updatePendingVolumeAttachment(c.Context(), &deps, id, first)
	updatePendingVolumeAttachment(c.Context(), &deps, id, updated)

	ready := deps.schedule.Ready(clock.Now())
	c.Assert(ready, tc.HasLen, 1)
	op, ok := ready[0].(*attachVolumeOp)
	c.Assert(ok, tc.IsTrue)
	c.Check(op.args, tc.DeepEquals, updated)
}
