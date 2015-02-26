// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/apiserver/uniter"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
)

type storageSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&storageSuite{})

func (s *storageSuite) TestStorageAttachments(c *gc.C) {
	resources := common.NewResources()
	getCanAccess := func() (common.AuthFunc, error) {
		return func(names.Tag) bool {
			return true
		}, nil
	}
	unitTag := names.NewUnitTag("mysql/0")
	watcher := &mockStringsWatcher{
		changes: make(chan []string, 1),
	}
	watcher.changes <- []string{"storage/0", "storage/1"}
	state := &mockStorageState{
		watchStorageAttachments: func(u names.UnitTag) state.StringsWatcher {
			c.Assert(u, gc.DeepEquals, unitTag)
			return watcher
		},
	}

	storage, err := uniter.NewStorageAPI(state, resources, getCanAccess)
	c.Assert(err, jc.ErrorIsNil)
	watches, err := storage.WatchStorageAttachments(params.Entities{
		Entities: []params.Entity{{unitTag.String()}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(watches, gc.DeepEquals, params.StringsWatchResults{
		Results: []params.StringsWatchResult{{
			StringsWatcherId: "1",
			Changes:          []string{"storage/0", "storage/1"},
		}},
	})
	c.Assert(resources.Get("1"), gc.Equals, watcher)
}

type mockStorageState struct {
	uniter.StorageStateInterface
	watchStorageAttachments func(names.UnitTag) state.StringsWatcher
}

func (m *mockStorageState) WatchStorageAttachments(u names.UnitTag) state.StringsWatcher {
	return m.watchStorageAttachments(u)
}

type mockStringsWatcher struct {
	state.StringsWatcher
	changes chan []string
}

func (m *mockStringsWatcher) Changes() <-chan []string {
	return m.changes
}
