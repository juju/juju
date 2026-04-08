// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machineactions

import (
	"context"
	stdtesting "testing"
	"time"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"

	apimachineactions "github.com/juju/juju/api/agent/machineactions"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/worker/machineactions/mocks"
	"github.com/juju/juju/rpc/params"
)

type internalWorkerSuite struct{}

func TestInternalWorkerSuite(t *stdtesting.T) {
	tc.Run(t, &internalWorkerSuite{})
}

func (*internalWorkerSuite) TestTearDownTimesOut(c *tc.C) {
	oldWait := tearDownWait
	tearDownWait = time.Millisecond
	defer func() {
		tearDownWait = oldWait
	}()

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	facade := mocks.NewMockFacade(ctrl)
	tag := names.NewMachineTag("0")
	actionTag := names.NewActionTag("1")
	action := apimachineactions.NewAction("1", "foo", nil, true, "")

	changes := make(chan []string, 1)
	changes <- []string{actionTag.Id()}

	started := make(chan struct{})
	unblock := make(chan struct{})
	finished := make(chan struct{})

	facade.EXPECT().RunningActions(gomock.Any(), tag).Return([]params.ActionResult{}, nil)
	facade.EXPECT().WatchActionNotifications(gomock.Any(), tag).Return(
		&stringsWatcher{
			Worker:  workertest.NewErrorWorker(nil),
			changes: changes,
		},
		nil,
	)
	facade.EXPECT().Action(gomock.Any(), actionTag).Return(action, nil)
	facade.EXPECT().ActionBegin(gomock.Any(), actionTag).DoAndReturn(
		func(context.Context, names.ActionTag) error {
			close(started)
			return nil
		},
	)
	facade.EXPECT().ActionFinish(
		gomock.Any(),
		actionTag,
		params.ActionCompleted,
		nil,
		"",
	).DoAndReturn(func(context.Context, names.ActionTag, string, map[string]any, string) error {
		close(finished)
		return nil
	})

	worker, err := NewMachineActionsWorker(WorkerConfig{
		Facade:     facade,
		MachineTag: tag,
		HandleAction: func(string, map[string]any) (map[string]any, error) {
			<-unblock
			return nil, nil
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	select {
	case <-started:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for action to start")
	}

	waitDone := make(chan error, 1)
	go func() {
		worker.Kill()
		waitDone <- worker.Wait()
	}()

	select {
	case err := <-waitDone:
		c.Assert(err, tc.ErrorIsNil)
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for worker shutdown")
	}

	close(unblock)

	select {
	case <-finished:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for blocked action to finish")
	}
}

type stringsWatcher struct {
	worker.Worker
	changes chan []string
}

func (s *stringsWatcher) Changes() watcher.StringsChannel {
	return s.changes
}
