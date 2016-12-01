// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
)

type watcherSuite struct {
	testing.BaseSuite
	st         *state.State
	resources  *common.Resources
	authorizer apiservertesting.FakeAuthorizer
}

var _ = gc.Suite(&watcherSuite{})

func (s *watcherSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.st = nil // none of the watcher facades use the State object
	s.resources = common.NewResources()
	s.authorizer = apiservertesting.FakeAuthorizer{}
}

func (s *watcherSuite) getFacade(
	c *gc.C,
	name string,
	version int,
	id string,
	dispose func(),
) interface{} {
	factory, err := common.Facades.GetFactory(name, version)
	c.Assert(err, jc.ErrorIsNil)
	facade, err := factory(s.st, s.resources, s.authorizer, id, dispose)
	c.Assert(err, jc.ErrorIsNil)
	return facade
}

func (s *watcherSuite) TestVolumeAttachmentsWatcher(c *gc.C) {
	ch := make(chan []string, 1)
	id := s.resources.Register(&fakeStringsWatcher{ch: ch})
	s.authorizer.Tag = names.NewMachineTag("123")

	ch <- []string{"0:1", "1:2"}
	facade := s.getFacade(c, "VolumeAttachmentsWatcher", 1, id, nopDispose).(machineStorageIdsWatcher)
	result, err := facade.Next()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(result, jc.DeepEquals, params.MachineStorageIdsWatchResult{
		Changes: []params.MachineStorageId{
			{MachineTag: "machine-0", AttachmentTag: "volume-1"},
			{MachineTag: "machine-1", AttachmentTag: "volume-2"},
		},
	})
}

func (s *watcherSuite) TestFilesystemAttachmentsWatcher(c *gc.C) {
	ch := make(chan []string, 1)
	id := s.resources.Register(&fakeStringsWatcher{ch: ch})
	s.authorizer.Tag = names.NewMachineTag("123")

	ch <- []string{"0:1", "1:2"}
	facade := s.getFacade(c, "FilesystemAttachmentsWatcher", 1, id, nopDispose).(machineStorageIdsWatcher)
	result, err := facade.Next()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(result, jc.DeepEquals, params.MachineStorageIdsWatchResult{
		Changes: []params.MachineStorageId{
			{MachineTag: "machine-0", AttachmentTag: "filesystem-1"},
			{MachineTag: "machine-1", AttachmentTag: "filesystem-2"},
		},
	})
}

func (s *watcherSuite) TestStopDiscards(c *gc.C) {
	id := s.resources.Register(&fakeStringsWatcher{})
	s.authorizer.Tag = names.NewMachineTag("123")
	var disposed bool
	facade := s.getFacade(c, "FilesystemAttachmentsWatcher", 1, id, func() {
		disposed = true
	}).(machineStorageIdsWatcher)
	err := facade.Stop()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(disposed, jc.IsTrue)
}

type machineStorageIdsWatcher interface {
	Next() (params.MachineStorageIdsWatchResult, error)
	Stop() error
}

type fakeStringsWatcher struct {
	state.StringsWatcher
	ch chan []string
}

func (w *fakeStringsWatcher) Changes() <-chan []string {
	return w.ch
}

func (w *fakeStringsWatcher) Stop() error {
	return nil
}

func nopDispose() {}
