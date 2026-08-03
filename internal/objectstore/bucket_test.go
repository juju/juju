// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstore

import (
	"testing"

	"github.com/juju/tc"

	jujutesting "github.com/juju/juju/internal/testing"
)

type bucketSuite struct{}

func TestBucketSuite(t *testing.T) {
	tc.Run(t, &bucketSuite{})
}

func (s *bucketSuite) TestControllerBucketName(c *tc.C) {
	config := jujutesting.FakeControllerConfig()

	name, err := ControllerBucketName(config)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(name, tc.Equals, "juju-"+config.ControllerUUID())
}
