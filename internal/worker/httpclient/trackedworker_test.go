// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpclient

import (
	stdtesting "testing"

	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"

	internalhttp "github.com/juju/juju/internal/http"
)

type trackedWorkerSuite struct {
	baseSuite

	client *internalhttp.Client
	states chan string
}

func TestTrackedWorkerSuite(t *stdtesting.T) {
	defer goleak.VerifyNone(t)
	tc.Run(t, &trackedWorkerSuite{})
}

func (s *trackedWorkerSuite) TestKilled(c *tc.C) {
	defer s.setupMocks(c).Finish()

	w, err := NewTrackedWorker(s.client)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CheckKill(c, w)

	w.Kill()
}

func (s *trackedWorkerSuite) setupMocks(c *tc.C) *gomock.Controller {
	// Ensure we buffer the channel, this is because we might miss the
	// event if we're too quick at starting up.
	s.states = make(chan string, 1)

	ctrl := s.baseSuite.setupMocks(c)

	s.client = internalhttp.NewClient()

	return ctrl
}
