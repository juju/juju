// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner_test

import (
	"testing"

	"github.com/juju/tc"

	sstesting "github.com/juju/juju/environs/simplestreams/testing"
)

// useTestImageData causes the given content to be served when published metadata is requested.
func useTestImageData(c *tc.C, files map[string]string) {
	if files != nil {
		sstesting.SetRoundTripperFiles(sstesting.AddSignedFiles(c, files), nil)
	} else {
		sstesting.SetRoundTripperFiles(nil, nil)
	}
}

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
