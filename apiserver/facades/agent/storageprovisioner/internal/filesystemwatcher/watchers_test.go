// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package filesystemwatcher_test

import (
	"errors"
	stdtesting "testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"

	"github.com/juju/juju/apiserver/facades/agent/storageprovisioner/internal/filesystemwatcher"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/state"
)

func TestWatchersSuite(t *stdtesting.T) { tc.Run(t, &WatchersSuite{}) }

type WatchersSuite struct {
	testhelpers.IsolationSuite
	backend  *mockBackend
	watchers filesystemwatcher.Watchers
}

func (s *WatchersSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.backend = &mockBackend{
		machineFilesystemsW:           newStringsWatcher(),
		unitFilesystemsW:              newStringsWatcher(),
		machineFilesystemAttachmentsW: newStringsWatcher(),
		unitFilesystemAttachmentsW:    newStringsWatcher(),
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
	s.AddCleanup(func(*tc.C) {
		s.backend.machineFilesystemsW.Stop()
		s.backend.machineFilesystemAttachmentsW.Stop()
		s.backend.modelFilesystemsW.Stop()
		s.backend.modelFilesystemAttachmentsW.Stop()
		s.backend.modelVolumeAttachmentsW.Stop()
	})
	s.watchers.Backend = s.backend
}

func (s *WatchersSuite) TestWatchModelManagedFilesystems(c *tc.C) {
	w := s.watchers.WatchModelManagedFilesystems()
	defer workertest.CleanKill(c, w)
	s.backend.modelFilesystemsW.C <- []string{"0", "1"}

	// Filesystem 1 has a backing volume, so should not be reported.
	wc := watchertest.NewStringsWatcherC(c, w)
	wc.AssertChangeInSingleEvent("0")
	wc.AssertNoChange()
}

func (s *WatchersSuite) TestWatchModelManagedFilesystemsWatcherErrorsPropagate(c *tc.C) {
	w := s.watchers.WatchModelManagedFilesystems()
	s.backend.modelFilesystemsW.T.Kill(errors.New("rah"))
	c.Assert(w.Wait(), tc.ErrorMatches, "rah")
}

func (s *WatchersSuite) TestWatchModelManagedFilesystemAttachments(c *tc.C) {
	w := s.watchers.WatchModelManagedFilesystemAttachments()
	defer workertest.CleanKill(c, w)
	s.backend.modelFilesystemAttachmentsW.C <- []string{"0:0", "0:1"}

	// Filesystem 1 has a backing volume, so should not be reported.
	wc := watchertest.NewStringsWatcherC(c, w)
	wc.AssertChangeInSingleEvent("0:0")
	wc.AssertNoChange()
}

func (s *WatchersSuite) TestWatchModelManagedFilesystemAttachmentsWatcherErrorsPropagate(c *tc.C) {
	w := s.watchers.WatchModelManagedFilesystemAttachments()
	s.backend.modelFilesystemAttachmentsW.T.Kill(errors.New("rah"))
	c.Assert(w.Wait(), tc.ErrorMatches, "rah")
}

func (s *WatchersSuite) TestWatchMachineManagedFilesystems(c *tc.C) {
	w := s.watchers.WatchMachineManagedFilesystems(names.NewMachineTag("0"))
	defer workertest.CleanKill(c, w)
	s.backend.modelFilesystemsW.C <- []string{"0", "1"}
	s.backend.machineFilesystemsW.C <- []string{"0/2", "0/3"}
	s.backend.modelVolumeAttachmentsW.C <- []string{"0:1", "0:2", "1:3"}

	wc := watchertest.NewStringsWatcherC(c, w)
	wc.AssertChangeInSingleEvent("0/2", "0/3", "1")
	wc.AssertNoChange()
}

func (s *WatchersSuite) TestWatchMachineManagedFilesystemsErrorsPropagate(c *tc.C) {
	w := s.watchers.WatchMachineManagedFilesystems(names.NewMachineTag("0"))
	s.backend.modelFilesystemsW.T.Kill(errors.New("rah"))
	c.Assert(w.Wait(), tc.ErrorMatches, "rah")
}

// TestWatchMachineManagedFilesystemsVolumeAttachedFirst is the same as
// TestWatchMachineManagedFilesystems, but the order of volume attachment
// and model filesystem events is swapped.
func (s *WatchersSuite) TestWatchMachineManagedFilesystemsVolumeAttachedFirst(c *tc.C) {
	w := s.watchers.WatchMachineManagedFilesystems(names.NewMachineTag("0"))
	defer workertest.CleanKill(c, w)
	s.backend.modelVolumeAttachmentsW.C <- []string{"0:1", "0:2", "1:3"}
	s.backend.modelFilesystemsW.C <- []string{"0", "1"}
	s.backend.machineFilesystemsW.C <- []string{"0/2", "0/3"}

	wc := watchertest.NewStringsWatcherC(c, w)
	wc.AssertChangeInSingleEvent("0/2", "0/3", "1")
	wc.AssertNoChange()
}

func (s *WatchersSuite) TestWatchMachineManagedFilesystemsVolumeAttachedLater(c *tc.C) {
	w := s.watchers.WatchMachineManagedFilesystems(names.NewMachineTag("0"))
	defer workertest.CleanKill(c, w)
	s.backend.modelFilesystemsW.C <- []string{"0", "1"}
	s.backend.machineFilesystemsW.C <- []string{"0/2", "0/3"}
	// No volumes are attached to begin with.
	s.backend.modelVolumeAttachmentsW.C <- []string{}

	wc := watchertest.NewStringsWatcherC(c, w)
	wc.AssertChangeInSingleEvent("0/2", "0/3")
	wc.AssertNoChange()

	s.backend.modelVolumeAttachmentsW.C <- []string{"0:1", "0:2", "1:3"}
	wc.AssertChangeInSingleEvent("1")
	wc.AssertNoChange()
}

func (s *WatchersSuite) TestWatchMachineManagedFilesystemsVolumeAttachmentDead(c *tc.C) {
	w := s.watchers.WatchMachineManagedFilesystems(names.NewMachineTag("0"))
	defer workertest.CleanKill(c, w)

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

	wc := watchertest.NewStringsWatcherC(c, w)
	wc.AssertChangeInSingleEvent()
	wc.AssertNoChange()
}

func (s *WatchersSuite) TestWatchMachineManagedFilesystemAttachments(c *tc.C) {
	w := s.watchers.WatchMachineManagedFilesystemAttachments(names.NewMachineTag("0"))
	defer workertest.CleanKill(c, w)
	s.backend.modelFilesystemAttachmentsW.C <- []string{"0:0", "0:1"}
	s.backend.machineFilesystemAttachmentsW.C <- []string{"0:0/2", "0:0/3"}
	s.backend.modelVolumeAttachmentsW.C <- []string{"0:1", "0:2", "1:3"}

	wc := watchertest.NewStringsWatcherC(c, w)
	wc.AssertChangeInSingleEvent("0:0/2", "0:0/3", "0:1")
	wc.AssertNoChange()
}

func (s *WatchersSuite) TestWatchMachineManagedFilesystemAttachmentsErrorsPropagate(c *tc.C) {
	w := s.watchers.WatchMachineManagedFilesystemAttachments(names.NewMachineTag("0"))
	s.backend.modelFilesystemAttachmentsW.T.Kill(errors.New("rah"))
	c.Assert(w.Wait(), tc.ErrorMatches, "rah")
}

// TestWatchMachineManagedFilesystemAttachmentsVolumeAttachedFirst is the same as
// TestWatchMachineManagedFilesystemAttachments, but the order of volume attachment
// and model filesystem attachment events is swapped.
func (s *WatchersSuite) TestWatchMachineManagedFilesystemAttachmentsVolumeAttachedFirst(c *tc.C) {
	w := s.watchers.WatchMachineManagedFilesystemAttachments(names.NewMachineTag("0"))
	defer workertest.CleanKill(c, w)
	s.backend.modelVolumeAttachmentsW.C <- []string{"0:1", "0:2", "1:3"}
	s.backend.modelFilesystemAttachmentsW.C <- []string{"0:0", "0:1"}
	s.backend.machineFilesystemAttachmentsW.C <- []string{"0:0/2", "0:0/3"}

	wc := watchertest.NewStringsWatcherC(c, w)
	wc.AssertChangeInSingleEvent("0:0/2", "0:0/3", "0:1")
	wc.AssertNoChange()
}

func (s *WatchersSuite) TestWatchMachineManagedFilesystemAttachmentsVolumeAttachedLater(c *tc.C) {
	w := s.watchers.WatchMachineManagedFilesystemAttachments(names.NewMachineTag("0"))
	defer workertest.CleanKill(c, w)
	s.backend.modelFilesystemAttachmentsW.C <- []string{"0:0", "0:1"}
	s.backend.machineFilesystemAttachmentsW.C <- []string{"0:0/2", "0:0/3"}
	// No volumes are attached to begin with.
	s.backend.modelVolumeAttachmentsW.C <- []string{}

	wc := watchertest.NewStringsWatcherC(c, w)
	wc.AssertChangeInSingleEvent("0:0/2", "0:0/3")
	wc.AssertNoChange()

	s.backend.modelVolumeAttachmentsW.C <- []string{"0:1", "0:2", "1:3"}
	wc.AssertChangeInSingleEvent("0:1")
	wc.AssertNoChange()
}

func (s *WatchersSuite) TestWatchMachineManagedFilesystemAttachmentsVolumeAttachmentDead(c *tc.C) {
	w := s.watchers.WatchMachineManagedFilesystemAttachments(names.NewMachineTag("0"))
	defer workertest.CleanKill(c, w)

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

	wc := watchertest.NewStringsWatcherC(c, w)
	wc.AssertChangeInSingleEvent()
	wc.AssertNoChange()
}

func (s *WatchersSuite) TestWatchUnitManagedFilesystems(c *tc.C) {
	w := s.watchers.WatchUnitManagedFilesystems(names.NewApplicationTag("mariadb"))
	defer workertest.CleanKill(c, w)
	s.backend.modelFilesystemsW.C <- []string{"0", "1"}
	s.backend.unitFilesystemsW.C <- []string{"mariadb/0/2", "mariadb/0/3"}
	s.backend.modelVolumeAttachmentsW.C <- []string{"mariadb/0:1", "mariadb/0:2", "mysql/1:3"}

	wc := watchertest.NewStringsWatcherC(c, w)
	wc.AssertChangeInSingleEvent("1", "mariadb/0/2", "mariadb/0/3")
	wc.AssertNoChange()
}

func (s *WatchersSuite) TestWatchUnitManagedFilesystemsErrorsPropagate(c *tc.C) {
	w := s.watchers.WatchUnitManagedFilesystems(names.NewApplicationTag("mariadb"))
	s.backend.modelFilesystemsW.T.Kill(errors.New("rah"))
	c.Assert(w.Wait(), tc.ErrorMatches, "rah")
}

func (s *WatchersSuite) TestWatchUnitManagedFilesystemAttachments(c *tc.C) {
	w := s.watchers.WatchUnitManagedFilesystemAttachments(names.NewApplicationTag("mariadb"))
	defer workertest.CleanKill(c, w)
	s.backend.modelFilesystemAttachmentsW.C <- []string{"mariadb/0:0", "mariadb/0:1"}
	s.backend.unitFilesystemAttachmentsW.C <- []string{"mariadb/0:mariadb/0/2", "mariadb/0:mariadb/0/3"}
	s.backend.modelVolumeAttachmentsW.C <- []string{"mariadb/0:1", "mariadb/0:2", "mysql/0:3"}

	wc := watchertest.NewStringsWatcherC(c, w)
	wc.AssertChangeInSingleEvent("mariadb/0:mariadb/0/2", "mariadb/0:mariadb/0/3", "mariadb/0:1")
	wc.AssertNoChange()
}

func (s *WatchersSuite) TestWatchUnitManagedFilesystemAttachmentsErrorsPropagate(c *tc.C) {
	w := s.watchers.WatchUnitManagedFilesystemAttachments(names.NewApplicationTag("mariadb"))
	s.backend.modelFilesystemAttachmentsW.T.Kill(errors.New("rah"))
	c.Assert(w.Wait(), tc.ErrorMatches, "rah")
}
