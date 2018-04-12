// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package filesystemwatcher_test

import (
	"errors"

	"github.com/juju/testing"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/facades/agent/storageprovisioner/internal/filesystemwatcher"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
)

var _ = gc.Suite(&WatchersSuite{})

type WatchersSuite struct {
	testing.IsolationSuite
	backend  *mockBackend
	watchers filesystemwatcher.Watchers
}

func (s *WatchersSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.backend = &mockBackend{
		machineFilesystemsW:           newStringsWatcher(),
		machineFilesystemAttachmentsW: newStringsWatcher(),
		modelFilesystemsW:             newStringsWatcher(),
		modelFilesystemAttachmentsW:   newStringsWatcher(),
		modelVolumeAttachmentsW:       newStringsWatcher(),
		filesystems: map[string]*mockFilesystem{
			// filesystem 0 has no backing volume.
			"0": {},
			// filesystem 1 is backed by volume 1.
			"1": {volume: names.NewVolumeTag("1")},
			// filesystem 2 is backed by volume 2.
			"2": {volume: names.NewVolumeTag("2")},
		},
		volumeAttachments: map[string]*mockVolumeAttachment{
			"1": {life: state.Alive},
			"2": {life: state.Alive},
		},
		volumeAttachmentRequested: make(chan names.VolumeTag, 10),
	}
	s.AddCleanup(func(*gc.C) {
		s.backend.machineFilesystemsW.Stop()
		s.backend.machineFilesystemAttachmentsW.Stop()
		s.backend.modelFilesystemsW.Stop()
		s.backend.modelFilesystemAttachmentsW.Stop()
		s.backend.modelVolumeAttachmentsW.Stop()
	})
	s.watchers.Backend = s.backend
}

func (s *WatchersSuite) TestWatchModelManagedFilesystems(c *gc.C) {
	w := s.watchers.WatchModelManagedFilesystems()
	defer statetesting.AssertKillAndWait(c, w)
	s.backend.modelFilesystemsW.C <- []string{"0", "1"}

	// Filesystem 1 has a backing volume, so should not be reported.
	wc := statetesting.NewStringsWatcherC(c, nopSyncStarter{}, w)
	wc.AssertChangeInSingleEvent("0")
	wc.AssertNoChange()
}

func (s *WatchersSuite) TestWatchModelManagedFilesystemsWatcherErrorsPropagate(c *gc.C) {
	w := s.watchers.WatchModelManagedFilesystems()
	s.backend.modelFilesystemsW.T.Kill(errors.New("rah"))
	c.Assert(w.Wait(), gc.ErrorMatches, "rah")
}

func (s *WatchersSuite) TestWatchModelManagedFilesystemAttachments(c *gc.C) {
	w := s.watchers.WatchModelManagedFilesystemAttachments()
	defer statetesting.AssertKillAndWait(c, w)
	s.backend.modelFilesystemAttachmentsW.C <- []string{"0:0", "0:1"}

	// Filesystem 1 has a backing volume, so should not be reported.
	wc := statetesting.NewStringsWatcherC(c, nopSyncStarter{}, w)
	wc.AssertChangeInSingleEvent("0:0")
	wc.AssertNoChange()
}

func (s *WatchersSuite) TestWatchModelManagedFilesystemAttachmentsWatcherErrorsPropagate(c *gc.C) {
	w := s.watchers.WatchModelManagedFilesystemAttachments()
	s.backend.modelFilesystemAttachmentsW.T.Kill(errors.New("rah"))
	c.Assert(w.Wait(), gc.ErrorMatches, "rah")
}

func (s *WatchersSuite) TestWatchMachineManagedFilesystems(c *gc.C) {
	w := s.watchers.WatchMachineManagedFilesystems(names.NewMachineTag("0"))
	defer statetesting.AssertKillAndWait(c, w)
	s.backend.modelFilesystemsW.C <- []string{"0", "1"}
	s.backend.machineFilesystemsW.C <- []string{"0/2", "0/3"}
	s.backend.modelVolumeAttachmentsW.C <- []string{"0:1", "0:2", "1:3"}

	wc := statetesting.NewStringsWatcherC(c, nopSyncStarter{}, w)
	wc.AssertChangeInSingleEvent("0/2", "0/3", "1")
	wc.AssertNoChange()
}

func (s *WatchersSuite) TestWatchMachineManagedFilesystemsErrorsPropagate(c *gc.C) {
	w := s.watchers.WatchMachineManagedFilesystems(names.NewMachineTag("0"))
	s.backend.modelFilesystemsW.T.Kill(errors.New("rah"))
	c.Assert(w.Wait(), gc.ErrorMatches, "rah")
}

// TestWatchMachineManagedFilesystemsVolumeAttachedFirst is the same as
// TestWatchMachineManagedFilesystems, but the order of volume attachment
// and model filesystem events is swapped.
func (s *WatchersSuite) TestWatchMachineManagedFilesystemsVolumeAttachedFirst(c *gc.C) {
	w := s.watchers.WatchMachineManagedFilesystems(names.NewMachineTag("0"))
	defer statetesting.AssertKillAndWait(c, w)
	s.backend.modelVolumeAttachmentsW.C <- []string{"0:1", "0:2", "1:3"}
	s.backend.modelFilesystemsW.C <- []string{"0", "1"}
	s.backend.machineFilesystemsW.C <- []string{"0/2", "0/3"}

	wc := statetesting.NewStringsWatcherC(c, nopSyncStarter{}, w)
	wc.AssertChangeInSingleEvent("0/2", "0/3", "1")
	wc.AssertNoChange()
}

func (s *WatchersSuite) TestWatchMachineManagedFilesystemsVolumeAttachedLater(c *gc.C) {
	w := s.watchers.WatchMachineManagedFilesystems(names.NewMachineTag("0"))
	defer statetesting.AssertKillAndWait(c, w)
	s.backend.modelFilesystemsW.C <- []string{"0", "1"}
	s.backend.machineFilesystemsW.C <- []string{"0/2", "0/3"}
	// No volumes are attached to begin with.
	s.backend.modelVolumeAttachmentsW.C <- []string{}

	wc := statetesting.NewStringsWatcherC(c, nopSyncStarter{}, w)
	wc.AssertChangeInSingleEvent("0/2", "0/3")
	wc.AssertNoChange()

	s.backend.modelVolumeAttachmentsW.C <- []string{"0:1", "0:2", "1:3"}
	wc.AssertChangeInSingleEvent("1")
	wc.AssertNoChange()
}

func (s *WatchersSuite) TestWatchMachineManagedFilesystemsVolumeAttachmentDead(c *gc.C) {
	w := s.watchers.WatchMachineManagedFilesystems(names.NewMachineTag("0"))
	defer statetesting.AssertKillAndWait(c, w)

	s.backend.machineFilesystemsW.C <- []string{}
	// Volume-backed filesystems 1 and 2 change.
	s.backend.modelFilesystemsW.C <- []string{"1", "2"}
	// The volumes are attached initially...
	s.backend.modelVolumeAttachmentsW.C <- []string{"0:1", "0:2"}
	// ... but before the client consumes the event, the backing volume
	// attachments 0:1 and 0:2 become Dead and removed respectively,
	// negating the previous change.
	<-s.backend.volumeAttachmentRequested
	<-s.backend.volumeAttachmentRequested
	s.backend.volumeAttachments["1"].life = state.Dead
	delete(s.backend.volumeAttachments, "2")
	s.backend.modelVolumeAttachmentsW.C <- []string{"0:1", "0:2"}

	// In order to not start the watcher until it has finished processing
	// the previous event, we send another empty list through the channel
	// which does nothing.
	s.backend.modelVolumeAttachmentsW.C <- []string{}

	wc := statetesting.NewStringsWatcherC(c, nopSyncStarter{}, w)
	wc.AssertChangeInSingleEvent()
	wc.AssertNoChange()
}

func (s *WatchersSuite) TestWatchMachineManagedFilesystemAttachments(c *gc.C) {
	w := s.watchers.WatchMachineManagedFilesystemAttachments(names.NewMachineTag("0"))
	defer statetesting.AssertKillAndWait(c, w)
	s.backend.modelFilesystemAttachmentsW.C <- []string{"0:0", "0:1"}
	s.backend.machineFilesystemAttachmentsW.C <- []string{"0:0/2", "0:0/3"}
	s.backend.modelVolumeAttachmentsW.C <- []string{"0:1", "0:2", "1:3"}

	wc := statetesting.NewStringsWatcherC(c, nopSyncStarter{}, w)
	wc.AssertChangeInSingleEvent("0:0/2", "0:0/3", "0:1")
	wc.AssertNoChange()
}

func (s *WatchersSuite) TestWatchMachineManagedFilesystemAttachmentsErrorsPropagate(c *gc.C) {
	w := s.watchers.WatchMachineManagedFilesystemAttachments(names.NewMachineTag("0"))
	s.backend.modelFilesystemAttachmentsW.T.Kill(errors.New("rah"))
	c.Assert(w.Wait(), gc.ErrorMatches, "rah")
}

// TestWatchMachineManagedFilesystemAttachmentsVolumeAttachedFirst is the same as
// TestWatchMachineManagedFilesystemAttachments, but the order of volume attachment
// and model filesystem attachment events is swapped.
func (s *WatchersSuite) TestWatchMachineManagedFilesystemAttachmentsVolumeAttachedFirst(c *gc.C) {
	w := s.watchers.WatchMachineManagedFilesystemAttachments(names.NewMachineTag("0"))
	defer statetesting.AssertKillAndWait(c, w)
	s.backend.modelVolumeAttachmentsW.C <- []string{"0:1", "0:2", "1:3"}
	s.backend.modelFilesystemAttachmentsW.C <- []string{"0:0", "0:1"}
	s.backend.machineFilesystemAttachmentsW.C <- []string{"0:0/2", "0:0/3"}

	wc := statetesting.NewStringsWatcherC(c, nopSyncStarter{}, w)
	wc.AssertChangeInSingleEvent("0:0/2", "0:0/3", "0:1")
	wc.AssertNoChange()
}

func (s *WatchersSuite) TestWatchMachineManagedFilesystemAttachmentsVolumeAttachedLater(c *gc.C) {
	w := s.watchers.WatchMachineManagedFilesystemAttachments(names.NewMachineTag("0"))
	defer statetesting.AssertKillAndWait(c, w)
	s.backend.modelFilesystemAttachmentsW.C <- []string{"0:0", "0:1"}
	s.backend.machineFilesystemAttachmentsW.C <- []string{"0:0/2", "0:0/3"}
	// No volumes are attached to begin with.
	s.backend.modelVolumeAttachmentsW.C <- []string{}

	wc := statetesting.NewStringsWatcherC(c, nopSyncStarter{}, w)
	wc.AssertChangeInSingleEvent("0:0/2", "0:0/3")
	wc.AssertNoChange()

	s.backend.modelVolumeAttachmentsW.C <- []string{"0:1", "0:2", "1:3"}
	wc.AssertChangeInSingleEvent("0:1")
	wc.AssertNoChange()
}

func (s *WatchersSuite) TestWatchMachineManagedFilesystemAttachmentsVolumeAttachmentDead(c *gc.C) {
	w := s.watchers.WatchMachineManagedFilesystemAttachments(names.NewMachineTag("0"))
	defer statetesting.AssertKillAndWait(c, w)

	s.backend.machineFilesystemAttachmentsW.C <- []string{}
	// Volume-backed filesystems attachments 0:1 and 0:2 change.
	s.backend.modelFilesystemAttachmentsW.C <- []string{"0:1", "0:2"}
	// The volumes are attached initially...
	s.backend.modelVolumeAttachmentsW.C <- []string{"0:1", "0:2"}
	// ... but before the client consumes the event, the backing volume
	// attachments 0:1 and 0:2 become Dead and removed respectively,
	// negating the previous change.
	<-s.backend.volumeAttachmentRequested
	<-s.backend.volumeAttachmentRequested
	s.backend.volumeAttachments["1"].life = state.Dead
	delete(s.backend.volumeAttachments, "2")
	s.backend.modelVolumeAttachmentsW.C <- []string{"0:1", "0:2"}

	// In order to not start the watcher until it has finished processing
	// the previous event, we send another empty list through the channel
	// which does nothing.
	s.backend.modelVolumeAttachmentsW.C <- []string{}

	wc := statetesting.NewStringsWatcherC(c, nopSyncStarter{}, w)
	wc.AssertChangeInSingleEvent()
	wc.AssertNoChange()
}
