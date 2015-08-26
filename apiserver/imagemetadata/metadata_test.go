// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadata_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state/cloudimagemetadata"
)

type metadataSuite struct {
	baseImageMetadataSuite
}

var _ = gc.Suite(&metadataSuite{})

func (s *metadataSuite) TestFindNil(c *gc.C) {
	found, err := s.api.List(params.ImageMetadataFilter{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Result, gc.HasLen, 0)
	s.assertCalls(c, []string{findMetadata})
}

func (s *metadataSuite) TestFindEmpty(c *gc.C) {
	s.state.findMetadata = func(f cloudimagemetadata.MetadataFilter) (map[cloudimagemetadata.SourceType][]cloudimagemetadata.Metadata, error) {
		s.calls = append(s.calls, findMetadata)
		return map[cloudimagemetadata.SourceType][]cloudimagemetadata.Metadata{}, nil
	}

	found, err := s.api.List(params.ImageMetadataFilter{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Result, gc.HasLen, 0)
	s.assertCalls(c, []string{findMetadata})
}

func (s *metadataSuite) TestFindEmptyGroups(c *gc.C) {
	s.state.findMetadata = func(f cloudimagemetadata.MetadataFilter) (map[cloudimagemetadata.SourceType][]cloudimagemetadata.Metadata, error) {
		s.calls = append(s.calls, findMetadata)
		return map[cloudimagemetadata.SourceType][]cloudimagemetadata.Metadata{
			cloudimagemetadata.Public: []cloudimagemetadata.Metadata{},
			cloudimagemetadata.Custom: []cloudimagemetadata.Metadata{},
		}, nil
	}

	found, err := s.api.List(params.ImageMetadataFilter{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Result, gc.HasLen, 0)
	s.assertCalls(c, []string{findMetadata})
}

func (s *metadataSuite) TestFindError(c *gc.C) {
	msg := "find error"
	s.state.findMetadata = func(f cloudimagemetadata.MetadataFilter) (map[cloudimagemetadata.SourceType][]cloudimagemetadata.Metadata, error) {
		s.calls = append(s.calls, findMetadata)
		return nil, errors.New(msg)
	}

	found, err := s.api.List(params.ImageMetadataFilter{})
	c.Assert(err, gc.ErrorMatches, msg)
	c.Assert(found.Result, gc.HasLen, 0)
	s.assertCalls(c, []string{findMetadata})
}

func (s *metadataSuite) TestFindOrder(c *gc.C) {
	customImageId := "custom1"
	customImageId2 := "custom2"
	customImageId3 := "custom3"
	publicImageId := "public1"

	s.state.findMetadata = func(f cloudimagemetadata.MetadataFilter) (map[cloudimagemetadata.SourceType][]cloudimagemetadata.Metadata, error) {
		s.calls = append(s.calls, findMetadata)
		return map[cloudimagemetadata.SourceType][]cloudimagemetadata.Metadata{
				cloudimagemetadata.Public: []cloudimagemetadata.Metadata{
					cloudimagemetadata.Metadata{ImageId: publicImageId},
				},
				cloudimagemetadata.Custom: []cloudimagemetadata.Metadata{
					cloudimagemetadata.Metadata{ImageId: customImageId},
					cloudimagemetadata.Metadata{ImageId: customImageId2},
					cloudimagemetadata.Metadata{ImageId: customImageId3},
				},
			},
			nil
	}

	found, err := s.api.List(params.ImageMetadataFilter{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Result, gc.HasLen, 4)
	c.Assert(found.Result, jc.SameContents, []params.CloudImageMetadata{
		params.CloudImageMetadata{ImageId: customImageId},
		params.CloudImageMetadata{ImageId: customImageId2},
		params.CloudImageMetadata{ImageId: customImageId3},
		params.CloudImageMetadata{ImageId: publicImageId},
	})
	s.assertCalls(c, []string{findMetadata})
}

func (s *metadataSuite) TestSaveEmpty(c *gc.C) {
	errs, err := s.api.Save(params.MetadataSaveParams{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errs.Results, gc.HasLen, 0)
	// not expected to call state :D
	s.assertCalls(c, []string{})
}

func (s *metadataSuite) TestSave(c *gc.C) {
	m := params.CloudImageMetadata{
		Source: "custom",
	}
	msg := "save error"

	s.state.saveMetadata = func(m cloudimagemetadata.Metadata) error {
		s.calls = append(s.calls, saveMetadata)
		if len(s.calls) == 1 {
			// don't err on first call
			return nil
		}
		return errors.New(msg)
	}

	errs, err := s.api.Save(params.MetadataSaveParams{Metadata: []params.CloudImageMetadata{m, m}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errs.Results, gc.HasLen, 2)
	c.Assert(errs.Results[0].Error, gc.IsNil)
	c.Assert(errs.Results[1].Error, gc.ErrorMatches, msg)
	s.assertCalls(c, []string{saveMetadata, saveMetadata})
}
