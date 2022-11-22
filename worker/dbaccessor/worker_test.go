// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dbaccessor

import (
	"database/sql"

	"github.com/canonical/go-dqlite/app"
	"github.com/golang/mock/gomock"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/workertest"
	gc "gopkg.in/check.v1"
)

type workerSuite struct {
	baseSuite

	optFactory *MockOptionFactory
}

var _ = gc.Suite(&workerSuite{})

func (s *workerSuite) TestGetControllerDBSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAnyLogs()

	optExp := s.optFactory.EXPECT()
	optExp.EnsureDataDir().Return(c.MkDir(), nil)
	optExp.WithAddressOption().Return(nil, nil)
	optExp.WithTLSOption().Return(nil, nil)
	optExp.WithClusterOption().Return(nil, nil)
	optExp.WithLogFuncOption().Return(nil)

	appExp := s.dbApp.EXPECT()
	appExp.Ready(gomock.Any()).Return(nil)
	appExp.ID().Return(uint64(666))
	appExp.Open(gomock.Any(), "controller").Return(&sql.DB{}, nil)
	appExp.Handover(gomock.Any()).Return(nil)
	appExp.Close().Return(nil)

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	getter, ok := w.(DBGetter)
	c.Assert(ok, jc.IsTrue, gc.Commentf("worker does not implement DBGetter"))

	_, err := getter.GetDB("controller")
	c.Assert(err, jc.ErrorIsNil)

	workertest.CleanKill(c, w)
}

func (s *workerSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := s.baseSuite.setupMocks(c)
	s.optFactory = NewMockOptionFactory(ctrl)
	return ctrl
}

func (s *workerSuite) newWorker(c *gc.C) worker.Worker {
	cfg := WorkerConfig{
		OptionFactory: s.optFactory,
		Clock:         s.clock,
		Logger:        s.logger,
		NewApp: func(string, ...app.Option) (DBApp, error) {
			return s.dbApp, nil
		},
	}

	w, err := newWorker(cfg)
	c.Assert(err, jc.ErrorIsNil)
	return w
}
