// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstore

import (
	stdtesting "testing"

	"github.com/juju/tc"
	"github.com/juju/worker/v5/workertest"
	"go.uber.org/goleak"

	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/trace"
)

type controllerWorkerSuite struct {
	baseSuite
}

var _ objectstore.ObjectStore = (*controllerWorker)(nil)

func TestControllerWorkerSuite(t *stdtesting.T) {
	defer goleak.VerifyNone(t)
	tc.Run(t, &controllerWorkerSuite{})
}

func (s *controllerWorkerSuite) TestWorkerStartup(c *tc.C) {
	defer s.setupMocks(c).Finish()

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	workertest.CleanKill(c, w)
}

func (s *controllerWorkerSuite) newWorker(c *tc.C) *controllerWorker {
	w, err := NewControllerWorker(
		newStubTrackedObjectStore(s.trackedObjectStore),
		trace.NoopTracer{},
	)
	c.Assert(err, tc.ErrorIsNil)
	return w.(*controllerWorker)
}
