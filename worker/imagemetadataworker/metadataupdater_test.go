// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadataworker_test

import (
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/worker/imagemetadataworker"
)

var _ = gc.Suite(&imageMetadataUpdateSuite{})

type imageMetadataUpdateSuite struct {
	baseMetadataSuite
}

func (s *imageMetadataUpdateSuite) TestWorker(c *gc.C) {
	done := make(chan struct{})
	client := s.ImageClient(done)

	w := imagemetadataworker.NewWorker(client)

	defer w.Wait()
	defer w.Kill()

	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for images metadata to update")
	}
	c.Assert(s.apiCalled, jc.IsTrue)
}
