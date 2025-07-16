// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner

import (
	context "context"
	"fmt"
	"testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"
	gomock "go.uber.org/mock/gomock"

	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	corewatcher "github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/rpc/params"
)

type stringSourcedWatcherSuite struct{}

func TestStringSourcedWatcherSuite(t *testing.T) {
	tc.Run(t, &stringSourcedWatcherSuite{})
}

func (s *stringSourcedWatcherSuite) TestWatch(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ch := make(chan []string, 1)
	ch <- []string{"foo", "bar"}
	mockStringWatcher := NewMockStringsWatcher(ctrl)
	mockStringWatcher.EXPECT().Changes().Return(ch).AnyTimes()
	mockStringWatcher.EXPECT().Wait().Return(nil).AnyTimes()
	mockStringWatcher.EXPECT().Kill().AnyTimes()

	w, err := newStringSourcedWatcher(
		mockStringWatcher,
		func(_ context.Context, events ...string) ([]string, error) {
			out := make([]string, len(events))
			for i, event := range events {
				out[i] = fmt.Sprintf("processed-%s", event)
			}
			return out, nil
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(w, tc.NotNil)
	defer workertest.CleanKill(c, w)
	wc := watchertest.NewStringsWatcherC(c, w)

	wc.AssertChange(
		"processed-foo",
		"processed-bar",
	)
	wc.AssertNoChange()
}

type machineStorageIdsWatcherSuite struct {
	watcherRegistry *facademocks.MockWatcherRegistry
	authorizer      apiservertesting.FakeAuthorizer
}

func TestMachineStorageIdsWatcherSuite(t *testing.T) {
	tc.Run(t, &machineStorageIdsWatcherSuite{})
}

func (s *machineStorageIdsWatcherSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.watcherRegistry = facademocks.NewMockWatcherRegistry(ctrl)
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: names.NewMachineTag("99"),
	}

	c.Cleanup(func() {
		s.authorizer = apiservertesting.FakeAuthorizer{}
		s.watcherRegistry = nil
	})
	return ctrl
}

func (s *machineStorageIdsWatcherSuite) TestWatcherNext(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	ch := make(chan []corewatcher.MachineStorageID, 1)
	ch <- []corewatcher.MachineStorageID{
		{MachineTag: "machine-0", AttachmentTag: "volume-1"},
		{MachineTag: "machine-1", AttachmentTag: "volume-2"},
	}
	sourceW := NewMockMachineStorageIDsWatcher(ctrl)
	sourceW.EXPECT().Changes().Return(ch).AnyTimes()
	sourceW.EXPECT().Wait().Return(nil).AnyTimes()
	sourceW.EXPECT().Kill().AnyTimes()
	s.watcherRegistry.EXPECT().Get("123").Return(sourceW, nil)

	w, err := newMachineStorageIdsWatcher(
		s.watcherRegistry, s.authorizer, "123", nil,
	)
	c.Assert(err, tc.ErrorIsNil)

	result, err := w.Next(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(result, tc.DeepEquals, params.MachineStorageIdsWatchResult{
		Changes: []params.MachineStorageId{
			{MachineTag: "machine-0", AttachmentTag: "volume-1"},
			{MachineTag: "machine-1", AttachmentTag: "volume-2"},
		},
	})
}
