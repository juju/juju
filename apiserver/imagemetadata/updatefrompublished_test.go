// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadata_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/imagemetadata"
	"github.com/juju/juju/state/cloudimagemetadata"
)

var _ = gc.Suite(&imageMetadataUpdateSuite{})

type imageMetadataUpdateSuite struct {
	baseImageMetadataSuite
}

func (s *imageMetadataUpdateSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
	imagemetadata.UseTestImageData(imagemetadata.TestImagesData)
}

func (s *imageMetadataUpdateSuite) TearDownSuite(c *gc.C) {
	imagemetadata.UseTestImageData(nil)
	s.BaseSuite.TearDownSuite(c)
}

func (s *imageMetadataUpdateSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
}

func (s *imageMetadataUpdateSuite) TestUpdateFromPublishedImages(c *gc.C) {
	saved := []cloudimagemetadata.Metadata{}
	expected := []cloudimagemetadata.Metadata{}

	s.state.saveMetadata = func(m cloudimagemetadata.Metadata) error {
		s.calls = append(s.calls, saveMetadata)
		saved = append(saved, m)
		return nil
	}

	err := s.api.UpdateFromPublishedImages()
	c.Assert(err, jc.ErrorIsNil)
	s.assertCalls(c, []string{environConfig, saveMetadata, saveMetadata})

	c.Assert(saved, jc.SameContents, expected)
}
