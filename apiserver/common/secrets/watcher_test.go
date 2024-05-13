// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets_test

import (
	"context"
	"time"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common/secrets"
	"github.com/juju/juju/apiserver/common/secrets/mocks"
	"github.com/juju/juju/core/watcher/watchertest"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	coretesting "github.com/juju/juju/testing"
)

type watcherSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&watcherSuite{})

func (s *watcherSuite) TestSecretBackendModelConfigWatcher(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ch := make(chan struct{}, 3)
	done := make(chan struct{})
	receiverReady := make(chan struct{})
	defer close(receiverReady)

	backendWatcher := watchertest.NewMockNotifyWatcher(ch)
	backendGetter := mocks.NewMockSecretBackendGetter(ctrl)

	go func() {
		for {
			_, ok := <-receiverReady
			if !ok {
				return
			}
			ch <- struct{}{}
		}
	}()
	receiverReady <- struct{}{}

	gomock.InOrder(
		// Initial call to get the current secret backend.
		backendGetter.EXPECT().GetSecretBackendID(gomock.Any()).Return("backend-id", nil),
		// Call to get the current secret backend after the first change(no change, but we always send the initial event).
		backendGetter.EXPECT().GetSecretBackendID(gomock.Any()).Return("backend-id", nil),
		// Call to get the current secret backend after the first change(no change, we won't send the event).
		backendGetter.EXPECT().GetSecretBackendID(gomock.Any()).Return("backend-id", nil),
		// Call to get the current secret backend after the second change - backend changed.
		backendGetter.EXPECT().GetSecretBackendID(gomock.Any()).Return("a-different-backend-id", nil),
	)

	w, err := secrets.NewSecretBackendModelConfigWatcher(context.Background(), backendGetter, backendWatcher, loggertesting.WrapCheckLog(c))
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(c *gc.C) { workertest.DirtyKill(c, w) })

	received := 0
ensureReceived:
	for a := coretesting.LongAttempt.Start(); a.Next(); {
		select {
		case _, ok := <-w.Changes():
			if !ok {
				break ensureReceived
			}
			received++
		case <-time.After(coretesting.ShortWait):
		}

		if received == 2 {
			return
		}

		select {
		case receiverReady <- struct{}{}:
		case <-done:
			break ensureReceived
		case <-time.After(coretesting.ShortWait):
		}

	}
	c.Fatalf("expected 2 events, got %d", received)
}
