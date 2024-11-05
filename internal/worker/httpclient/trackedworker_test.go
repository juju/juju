// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpclient

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	internalhttp "github.com/juju/juju/internal/http"
)

type trackedWorkerSuite struct {
	baseSuite

	client *internalhttp.Client
	states chan string
}

var _ = gc.Suite(&trackedWorkerSuite{})

func (s *trackedWorkerSuite) TestKilled(c *gc.C) {
	defer s.setupMocks(c).Finish()

	w, err := NewTrackedWorker(s.client)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CheckKill(c, w)

	w.Kill()
}

func (s *trackedWorkerSuite) setupMocks(c *gc.C) *gomock.Controller {
	// Ensure we buffer the channel, this is because we might miss the
	// event if we're too quick at starting up.
	s.states = make(chan string, 1)

	ctrl := s.baseSuite.setupMocks(c)

	s.client = internalhttp.NewClient()

	return ctrl
}
