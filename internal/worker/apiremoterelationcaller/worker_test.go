// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiremoterelationcaller

import (
	context "context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/juju/api"
	"github.com/juju/juju/core/model"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/goleak"
	gomock "go.uber.org/mock/gomock"
)

type workerSuite struct {
	baseSuite
}

func TestWorkerSuite(t *testing.T) {
	defer goleak.VerifyNone(t)
	tc.Run(t, &workerSuite{})
}

func (s *workerSuite) TestWorkerKill(c *tc.C) {
	defer s.setupMocks(c).Finish()

	w := s.newWorker(c)
	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestGetConnectionForModelAlreadyCancelled(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := model.UUID("test-model-uuid")

	ctx, cancel := context.WithCancel(c.Context())

	cancel()

	w := s.newWorker(c)
	defer workertest.CleanKill(c, w)

	_, err := w.GetConnectionForModel(ctx, modelUUID)
	c.Assert(err, tc.ErrorIs, context.Canceled)
}

func (s *workerSuite) TestGetConnectionForModelAlreadyDead(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := model.UUID("test-model-uuid")

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	w.Kill()

	_, err := w.GetConnectionForModel(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIs, ErrAPIRemoteRelationCallerDead)
}

func (s *workerSuite) TestGetConnectionForModel(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := model.UUID("test-model-uuid")
	apiInfo := api.Info{
		Tag: names.NewUserTag("fred"),
	}

	s.apiInfoGetter.EXPECT().GetAPIInfoForModel(gomock.Any(), modelUUID).Return(apiInfo, nil)
	s.connectionGetter.EXPECT().GetConnectionForModel(gomock.Any(), modelUUID, apiInfo).Return(s.connection, nil)

	done := make(chan struct{})
	s.connection.EXPECT().Broken().DoAndReturn(func() <-chan struct{} {
		close(done)
		return make(chan struct{})
	})

	w := s.newWorker(c)
	defer workertest.CleanKill(c, w)

	conn, err := w.GetConnectionForModel(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(conn, tc.NotNil)

	select {
	case <-done:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for done channel")
	}
}

func (s *workerSuite) TestGetConnectionForModelMultipleTimes(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := model.UUID("test-model-uuid")
	apiInfo := api.Info{
		Tag: names.NewUserTag("fred"),
	}

	s.apiInfoGetter.EXPECT().GetAPIInfoForModel(gomock.Any(), modelUUID).Return(apiInfo, nil)
	s.connectionGetter.EXPECT().GetConnectionForModel(gomock.Any(), modelUUID, apiInfo).Return(s.connection, nil)

	done := make(chan struct{})
	s.connection.EXPECT().Broken().DoAndReturn(func() <-chan struct{} {
		close(done)
		return make(chan struct{})
	})

	w := s.newWorker(c)
	defer workertest.CleanKill(c, w)

	conn, err := w.GetConnectionForModel(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(conn, tc.NotNil)

	conn, err = w.GetConnectionForModel(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(conn, tc.NotNil)

	select {
	case <-done:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for done channel")
	}
}

func (s *workerSuite) TestGetConnectionForModelDifferent(c *tc.C) {
	defer s.setupMocks(c).Finish()

	model1UUID := model.UUID("test-model-1-uuid")
	model2UUID := model.UUID("test-model-2-uuid")

	apiInfo1 := api.Info{
		Tag: names.NewUserTag("fred"),
	}
	apiInfo2 := api.Info{
		Tag: names.NewUserTag("bob"),
	}

	s.apiInfoGetter.EXPECT().GetAPIInfoForModel(gomock.Any(), model1UUID).Return(apiInfo1, nil)
	s.connectionGetter.EXPECT().GetConnectionForModel(gomock.Any(), model1UUID, apiInfo1).Return(s.connection, nil)

	s.apiInfoGetter.EXPECT().GetAPIInfoForModel(gomock.Any(), model2UUID).Return(apiInfo2, nil)
	s.connectionGetter.EXPECT().GetConnectionForModel(gomock.Any(), model2UUID, apiInfo2).Return(s.connection, nil)

	done := make(chan struct{})
	var called uint64
	s.connection.EXPECT().Broken().DoAndReturn(func() <-chan struct{} {
		if atomic.AddUint64(&called, 1) == 2 {
			close(done)
		}
		return make(chan struct{})
	}).Times(2)

	w := s.newWorker(c)
	defer workertest.CleanKill(c, w)

	conn, err := w.GetConnectionForModel(c.Context(), model1UUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(conn, tc.NotNil)

	conn, err = w.GetConnectionForModel(c.Context(), model2UUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(conn, tc.NotNil)

	select {
	case <-done:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for done channel")
	}
}

func (s *workerSuite) TestGetConnectionForModelDifferentMultipleTimes(c *tc.C) {
	defer s.setupMocks(c).Finish()

	model1UUID := model.UUID("test-model-1-uuid")
	model2UUID := model.UUID("test-model-2-uuid")

	apiInfo1 := api.Info{
		Tag: names.NewUserTag("fred"),
	}
	apiInfo2 := api.Info{
		Tag: names.NewUserTag("bob"),
	}

	s.apiInfoGetter.EXPECT().GetAPIInfoForModel(gomock.Any(), model1UUID).Return(apiInfo1, nil)
	s.connectionGetter.EXPECT().GetConnectionForModel(gomock.Any(), model1UUID, apiInfo1).Return(s.connection, nil)

	s.apiInfoGetter.EXPECT().GetAPIInfoForModel(gomock.Any(), model2UUID).Return(apiInfo2, nil)
	s.connectionGetter.EXPECT().GetConnectionForModel(gomock.Any(), model2UUID, apiInfo2).Return(s.connection, nil)

	done := make(chan struct{})
	var called uint64
	s.connection.EXPECT().Broken().DoAndReturn(func() <-chan struct{} {
		if atomic.AddUint64(&called, 1) == 2 {
			close(done)
		}
		return make(chan struct{})
	}).Times(2)

	w := s.newWorker(c)
	defer workertest.CleanKill(c, w)

	conn, err := w.GetConnectionForModel(c.Context(), model1UUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(conn, tc.NotNil)

	conn, err = w.GetConnectionForModel(c.Context(), model2UUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(conn, tc.NotNil)

	conn, err = w.GetConnectionForModel(c.Context(), model2UUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(conn, tc.NotNil)

	select {
	case <-done:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for done channel")
	}
}

func (s *workerSuite) TestGetConnectionForModelBrokenConnection(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := model.UUID("test-model-uuid")
	apiInfo := api.Info{
		Tag: names.NewUserTag("fred"),
	}

	s.apiInfoGetter.EXPECT().GetAPIInfoForModel(gomock.Any(), modelUUID).Return(apiInfo, nil).Times(2)
	s.connectionGetter.EXPECT().GetConnectionForModel(gomock.Any(), modelUUID, apiInfo).Return(s.connection, nil).Times(2)

	done := make(chan struct{})
	var called uint64
	s.connection.EXPECT().Broken().DoAndReturn(func() <-chan struct{} {
		if atomic.AddUint64(&called, 1) == 2 {
			close(done)
		}

		ch := make(chan struct{})
		close(ch)
		return ch
	}).Times(2)

	w := s.newWorker(c)
	defer workertest.CleanKill(c, w)

	conn, err := w.GetConnectionForModel(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(conn, tc.NotNil)

	select {
	case <-done:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for done channel")
	}
}

func (s *workerSuite) newWorker(c *tc.C) *remoteWorker {
	w, err := NewWorker(s.newConfig(c))
	c.Assert(err, tc.ErrorIsNil)

	return w.(*remoteWorker)
}

func (s *workerSuite) newConfig(c *tc.C) Config {
	return Config{
		APIInfoGetter:    s.apiInfoGetter,
		ConnectionGetter: s.connectionGetter,
		Clock:            clock.WallClock,
		Logger:           s.logger,
		RestartDelay:     time.Millisecond * 100,
	}
}
