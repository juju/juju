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

type filesystemEventsSuite struct{}

func TestFilesystemEventsSuite(t *testing.T) {
	tc.Run(t, &filesystemEventsSuite{})
}

func (*filesystemEventsSuite) TestUpdatesPendingFilesystemOperation(c *tc.C) {
	clock := testclock.NewClock(time.Time{})
	filesystem := names.NewFilesystemTag("0")
	deps := dependencies{
		config:   Config{Clock: clock, Logger: loggertesting.WrapCheckLog(c)},
		schedule: schedule.NewSchedule(clock),
	}
	first := storage.FilesystemParams{
		Tag:  filesystem,
		Size: 1024,
	}
	updated := first
	updated.Size = 2048

	updatePendingFilesystem(c.Context(), &deps, first)
	updatePendingFilesystem(c.Context(), &deps, updated)

	ready := deps.schedule.Ready(clock.Now())
	c.Assert(ready, tc.HasLen, 1)
	op, ok := ready[0].(*createFilesystemOp)
	c.Assert(ok, tc.IsTrue)
	c.Check(op.args, tc.DeepEquals, updated)
}

func (*filesystemEventsSuite) TestUpdatesPendingFilesystemAttachmentOperation(c *tc.C) {
	clock := testclock.NewClock(time.Time{})
	machine := names.NewMachineTag("0")
	filesystem := names.NewFilesystemTag("0")
	id := params.MachineStorageId{
		MachineTag:    machine.String(),
		AttachmentTag: filesystem.String(),
	}
	deps := dependencies{
		config: Config{Clock: clock},
		filesystems: map[names.FilesystemTag]storage.Filesystem{
			filesystem: {FilesystemInfo: storage.FilesystemInfo{ProviderId: "filesystem-1"}},
		},
		schedule: schedule.NewSchedule(clock),
	}
	first := storage.FilesystemAttachmentParams{
		AttachmentParams: storage.AttachmentParams{
			Machine:    machine,
			InstanceId: "instance-1",
		},
		Filesystem: filesystem,
		Path:       "/first",
	}
	updated := first
	updated.InstanceId = "instance-2"
	updated.Path = "/updated"

	updatePendingFilesystemAttachment(c.Context(), &deps, id, first)
	updatePendingFilesystemAttachment(c.Context(), &deps, id, updated)

	ready := deps.schedule.Ready(clock.Now())
	c.Assert(ready, tc.HasLen, 1)
	op, ok := ready[0].(*attachFilesystemOp)
	c.Assert(ok, tc.IsTrue)
	c.Check(op.args, tc.DeepEquals, storage.FilesystemAttachmentParams{
		AttachmentParams: storage.AttachmentParams{
			Machine:    machine,
			InstanceId: "instance-2",
		},
		Filesystem:           filesystem,
		FilesystemProviderId: "filesystem-1",
		Path:                 "/updated",
	})
}

func (*filesystemEventsSuite) TestIncompleteFilesystemAttachmentRemainsPending(c *tc.C) {
	clock := testclock.NewClock(time.Time{})
	machine := names.NewUnitTag("app/0")
	filesystem := names.NewFilesystemTag("0")
	id := params.MachineStorageId{
		MachineTag:    machine.String(),
		AttachmentTag: filesystem.String(),
	}
	attachment := storage.FilesystemAttachmentParams{
		AttachmentParams: storage.AttachmentParams{Machine: machine},
		Filesystem:       filesystem,
	}
	deps := dependencies{
		config: Config{
			Clock:  clock,
			Logger: loggertesting.WrapCheckLog(c),
		},
		incompleteFilesystemAttachmentParams: map[params.MachineStorageId]storage.FilesystemAttachmentParams{},
		schedule:                             schedule.NewSchedule(clock),
	}

	updatePendingFilesystemAttachment(c.Context(), &deps, id, attachment)

	_, ok := deps.schedule.Get(id)
	c.Check(ok, tc.IsFalse)
	c.Check(deps.incompleteFilesystemAttachmentParams, tc.DeepEquals,
		map[params.MachineStorageId]storage.FilesystemAttachmentParams{id: attachment})
}

func (*filesystemEventsSuite) TestIncompleteFilesystemAttachmentDoesNotReplaceScheduledOperation(c *tc.C) {
	clock := testclock.NewClock(time.Time{})
	machine := names.NewUnitTag("app/0")
	filesystem := names.NewFilesystemTag("0")
	id := params.MachineStorageId{
		MachineTag:    machine.String(),
		AttachmentTag: filesystem.String(),
	}
	first := storage.FilesystemAttachmentParams{
		AttachmentParams: storage.AttachmentParams{Machine: machine},
		Filesystem:       filesystem,
		Path:             "/first",
	}
	incomplete := first
	incomplete.Path = "/incomplete"
	deps := dependencies{
		config: Config{
			Clock:  clock,
			Logger: loggertesting.WrapCheckLog(c),
		},
		filesystems: map[names.FilesystemTag]storage.Filesystem{
			filesystem: {FilesystemInfo: storage.FilesystemInfo{ProviderId: "filesystem-1"}},
		},
		incompleteFilesystemAttachmentParams: map[params.MachineStorageId]storage.FilesystemAttachmentParams{},
		schedule:                             schedule.NewSchedule(clock),
	}

	updatePendingFilesystemAttachment(c.Context(), &deps, id, first)
	delete(deps.filesystems, filesystem)
	updatePendingFilesystemAttachment(c.Context(), &deps, id, incomplete)

	pending, ok := deps.schedule.Get(id)
	c.Assert(ok, tc.IsTrue)
	op, ok := pending.(*attachFilesystemOp)
	c.Assert(ok, tc.IsTrue)
	c.Check(op.args, tc.DeepEquals, storage.FilesystemAttachmentParams{
		AttachmentParams:     storage.AttachmentParams{Machine: machine},
		Filesystem:           filesystem,
		FilesystemProviderId: "filesystem-1",
		Path:                 "/first",
	})
	c.Check(deps.incompleteFilesystemAttachmentParams, tc.DeepEquals,
		map[params.MachineStorageId]storage.FilesystemAttachmentParams{id: incomplete})
}
