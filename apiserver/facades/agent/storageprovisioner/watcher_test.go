// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner

import (
	context "context"
	"fmt"
	"testing"

	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"
	gomock "go.uber.org/mock/gomock"

	"github.com/juju/juju/core/watcher/watchertest"
)

type watcherSuite struct{}

func TestWatcherSuite(t *testing.T) {
	tc.Run(t, &watcherSuite{})
}

func (s *watcherSuite) TestWatch(c *tc.C) {
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
