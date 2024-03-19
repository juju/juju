// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"sort"
	"time"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	gomock "go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/domain/secretbackend"
	"github.com/juju/juju/internal/uuid"
	jujutesting "github.com/juju/juju/testing"
)

func (s *stateSuite) TestWatchSecretBackendRotationChanges(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	backendID1 := uuid.MustNewUUID().String()
	backendID2 := uuid.MustNewUUID().String()
	nextRotateTime1 := time.Now().Add(12 * time.Hour)
	nextRotateTime2 := time.Now().Add(24 * time.Hour)

	_, err := s.state.UpsertSecretBackend(context.Background(), secretbackend.UpsertSecretBackendParams{
		ID:             backendID1,
		Name:           "my-backend1",
		BackendType:    "vault",
		NextRotateTime: &nextRotateTime1,
	})
	c.Assert(err, gc.IsNil)

	_, err = s.state.UpsertSecretBackend(context.Background(), secretbackend.UpsertSecretBackendParams{
		ID:             backendID2,
		Name:           "my-backend2",
		BackendType:    "kubernetes",
		NextRotateTime: &nextRotateTime2,
	})
	c.Assert(err, gc.IsNil)

	ch := make(chan []string)
	mockWatcher := NewMockStringsWatcher(ctrl)
	mockWatcher.EXPECT().Changes().Return(ch).AnyTimes()

	watcherFactory := NewMockWatcherFactory(ctrl)
	watcherFactory.EXPECT().NewNamespaceWatcher(
		"secret_backend_rotation", changestream.All, `SELECT backend_uuid FROM secret_backend_rotation`,
	).Return(mockWatcher, nil)

	w, err := s.state.WatchSecretBackendRotationChanges(watcherFactory)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(w, gc.NotNil)
	s.AddCleanup(func(c *gc.C) { workertest.CleanKill(c, w) })
	select {
	case <-w.Changes():
		// consume the initial empty change then send the backend IDs
		ch <- []string{backendID1, backendID2}
	case <-time.After(jujutesting.ShortWait):
		c.Fatalf("timed out waiting for the initial changes")
	}

	select {
	case changes, ok := <-w.Changes():
		c.Assert(ok, gc.Equals, true)
		c.Assert(changes, gc.HasLen, 2)
		sort.Slice(changes, func(i, j int) bool {
			return changes[i].Name < changes[j].Name
		})

		c.Assert(changes[0].ID, gc.Equals, backendID1)
		c.Assert(changes[0].Name, gc.Equals, "my-backend1")
		c.Assert(changes[0].NextTriggerTime.Equal(nextRotateTime1), jc.IsTrue)
		c.Assert(changes[1].ID, gc.Equals, backendID2)
		c.Assert(changes[1].Name, gc.Equals, "my-backend2")
		c.Assert(changes[1].NextTriggerTime.Equal(nextRotateTime2), jc.IsTrue)
	case <-time.After(jujutesting.LongWait):
		c.Fatalf("timed out waiting for backend rotation changes")
	}
}
