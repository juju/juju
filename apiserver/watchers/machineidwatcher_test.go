// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watchers_test

import (
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/rpc/params"
)

type machineIDWatcherSuite struct {
	baseSuite
}

var _ = gc.Suite(&machineIDWatcherSuite{})

func (s *machineIDWatcherSuite) TestVolumeAttachmentsWatcher(c *gc.C) {
	ch := make(chan []string, 1)
	id, err := s.watcherRegistry.Register(&fakeStringsWatcher{ch: ch})
	c.Assert(err, jc.ErrorIsNil)
	s.authorizer.Tag = names.NewMachineTag("123")

	ch <- []string{"0:1", "1:2"}
	facade := s.getFacade(c, "VolumeAttachmentsWatcher", 2, id, nopDispose).(machineStorageIdsWatcher)
	result, err := facade.Next()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(result, jc.DeepEquals, params.MachineStorageIdsWatchResult{
		Changes: []params.MachineStorageId{
			{MachineTag: "machine-0", AttachmentTag: "volume-1"},
			{MachineTag: "machine-1", AttachmentTag: "volume-2"},
		},
	})
}

func (s *machineIDWatcherSuite) TestFilesystemAttachmentsWatcher(c *gc.C) {
	ch := make(chan []string, 1)
	id, err := s.watcherRegistry.Register(&fakeStringsWatcher{ch: ch})
	c.Assert(err, jc.ErrorIsNil)
	s.authorizer.Tag = names.NewMachineTag("123")

	ch <- []string{"0:1", "1:2"}
	facade := s.getFacade(c, "FilesystemAttachmentsWatcher", 2, id, nopDispose).(machineStorageIdsWatcher)
	result, err := facade.Next()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(result, jc.DeepEquals, params.MachineStorageIdsWatchResult{
		Changes: []params.MachineStorageId{
			{MachineTag: "machine-0", AttachmentTag: "filesystem-1"},
			{MachineTag: "machine-1", AttachmentTag: "filesystem-2"},
		},
	})
}
