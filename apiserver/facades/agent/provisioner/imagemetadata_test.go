// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner_test

import (
	"testing"

	"github.com/juju/tc"
)

type ImageMetadataSuite struct {
	provisionerSuite
}

func TestImageMetadataSuite(t *testing.T) {
	tc.Run(t, &ImageMetadataSuite{})
}

func (s *ImageMetadataSuite) TestStub(c *tc.C) {
	c.Skip(`This suite is missing tests for the following scenarios:

 - Test provisioning info with metadata from data sources.
 - Test provisioning info with metadata from state.
 `)
}
