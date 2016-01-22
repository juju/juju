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
	s.assertCalls(c, findMetadata)
}

func (s *metadataSuite) TestFindEmpty(c *gc.C) {
	s.state.findMetadata = func(f cloudimagemetadata.MetadataFilter) (map[string][]cloudimagemetadata.Metadata, error) {
		return map[string][]cloudimagemetadata.Metadata{}, nil
	}

	found, err := s.api.List(params.ImageMetadataFilter{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Result, gc.HasLen, 0)
	s.assertCalls(c, findMetadata)
}

func (s *metadataSuite) TestFindEmptyGroups(c *gc.C) {
	s.state.findMetadata = func(f cloudimagemetadata.MetadataFilter) (map[string][]cloudimagemetadata.Metadata, error) {
		return map[string][]cloudimagemetadata.Metadata{
			"public": []cloudimagemetadata.Metadata{},
			"custom": []cloudimagemetadata.Metadata{},
		}, nil
	}

	found, err := s.api.List(params.ImageMetadataFilter{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Result, gc.HasLen, 0)
	s.assertCalls(c, findMetadata)
}

func (s *metadataSuite) TestFindError(c *gc.C) {
	msg := "find error"
	s.state.findMetadata = func(f cloudimagemetadata.MetadataFilter) (map[string][]cloudimagemetadata.Metadata, error) {
		return nil, errors.New(msg)
	}

	found, err := s.api.List(params.ImageMetadataFilter{})
	c.Assert(err, gc.ErrorMatches, msg)
	c.Assert(found.Result, gc.HasLen, 0)
	s.assertCalls(c, findMetadata)
}

func (s *metadataSuite) TestFindOrder(c *gc.C) {
	customImageId := "custom1"
	customImageId2 := "custom2"
	customImageId3 := "custom3"
	publicImageId := "public1"

	s.state.findMetadata = func(f cloudimagemetadata.MetadataFilter) (map[string][]cloudimagemetadata.Metadata, error) {
		return map[string][]cloudimagemetadata.Metadata{
				"public": []cloudimagemetadata.Metadata{
					cloudimagemetadata.Metadata{ImageId: publicImageId, Priority: 15},
				},
				"custom": []cloudimagemetadata.Metadata{
					cloudimagemetadata.Metadata{ImageId: customImageId, Priority: 87},
					cloudimagemetadata.Metadata{ImageId: customImageId2, Priority: 20},
					cloudimagemetadata.Metadata{ImageId: customImageId3, Priority: 56},
				},
			},
			nil
	}

	found, err := s.api.List(params.ImageMetadataFilter{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Result, gc.HasLen, 4)

	c.Assert(found.Result, jc.SameContents, []params.CloudImageMetadata{
		params.CloudImageMetadata{ImageId: customImageId, Priority: 87},
		params.CloudImageMetadata{ImageId: customImageId3, Priority: 56},
		params.CloudImageMetadata{ImageId: customImageId2, Priority: 20},
		params.CloudImageMetadata{ImageId: publicImageId, Priority: 15},
	})
	s.assertCalls(c, findMetadata)
}

func (s *metadataSuite) TestSaveEmpty(c *gc.C) {
	errs, err := s.api.Save(params.MetadataSaveParams{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errs.Results, gc.HasLen, 0)
	// not expected to call state :D
	s.assertCalls(c, environConfig)
}

func (s *metadataSuite) TestSave(c *gc.C) {
	m := params.CloudImageMetadata{
		Source: "custom",
	}
	msg := "save error"

	saveCalls := 0
	s.state.saveMetadata = func(m []cloudimagemetadata.Metadata) error {
		saveCalls += 1
		c.Assert(m, gc.HasLen, saveCalls)
		if saveCalls == 1 {
			// don't err on first call
			return nil
		}
		return errors.New(msg)
	}

	errs, err := s.api.Save(params.MetadataSaveParams{
		Metadata: []params.CloudImageMetadataList{{
			Metadata: []params.CloudImageMetadata{m},
		}, {
			Metadata: []params.CloudImageMetadata{m, m},
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errs.Results, gc.HasLen, 2)
	c.Assert(errs.Results[0].Error, gc.IsNil)
	c.Assert(errs.Results[1].Error, gc.ErrorMatches, msg)
	s.assertCalls(c, environConfig, saveMetadata, saveMetadata)
}

func (s *metadataSuite) TestDeleteEmpty(c *gc.C) {
	errs, err := s.api.Delete(params.MetadataImageIds{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errs.Results, gc.HasLen, 0)
	// not expected to call state :D
	s.assertCalls(c)
}

func (s *metadataSuite) TestDelete(c *gc.C) {
	idOk := "ok"
	idFail := "fail"
	msg := "delete error"

	s.state.deleteMetadata = func(imageId string) error {
		if imageId == idFail {
			return errors.New(msg)
		}
		return nil
	}

	errs, err := s.api.Delete(params.MetadataImageIds{[]string{idOk, idFail}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errs.Results, gc.HasLen, 2)
	c.Assert(errs.Results[0].Error, gc.IsNil)
	c.Assert(errs.Results[1].Error, gc.ErrorMatches, msg)
	s.assertCalls(c, deleteMetadata, deleteMetadata)
}
