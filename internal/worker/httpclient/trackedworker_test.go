// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpclient

import (
	"testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v5/workertest"

	corehttp "github.com/juju/juju/core/http"
	internalhttp "github.com/juju/juju/internal/http"
)

type trackedWorkerSuite struct{}

func TestTrackedWorkerSuite(t *testing.T) {
	tc.Run(t, &trackedWorkerSuite{})
}

func (s *trackedWorkerSuite) TestImplementsCACertUpdater(c *tc.C) {
	w, err := NewTrackedWorker(internalhttp.NewClient())
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	updater, ok := w.(corehttp.CACertUpdater)
	c.Assert(ok, tc.IsTrue)

	err = updater.UpdateCACert("not a cert")
	c.Assert(err, tc.ErrorIs, errors.NotValid)
}
