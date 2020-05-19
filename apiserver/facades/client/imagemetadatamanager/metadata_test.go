// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadatamanager_test

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
	s.assertCalls(c, controllerTag, findMetadata)
}

func (s *metadataSuite) TestFindEmpty(c *gc.C) {
	s.state.findMetadata = func(f cloudimagemetadata.MetadataFilter) (map[string][]cloudimagemetadata.Metadata, error) {
		return map[string][]cloudimagemetadata.Metadata{}, nil
	}

	found, err := s.api.List(params.ImageMetadataFilter{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Result, gc.HasLen, 0)
	s.assertCalls(c, controllerTag, findMetadata)
}

func (s *metadataSuite) TestFindEmptyGroups(c *gc.C) {
	s.state.findMetadata = func(f cloudimagemetadata.MetadataFilter) (map[string][]cloudimagemetadata.Metadata, error) {
		return map[string][]cloudimagemetadata.Metadata{
			"public": {},
			"custom": {},
		}, nil
	}

	found, err := s.api.List(params.ImageMetadataFilter{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Result, gc.HasLen, 0)
	s.assertCalls(c, controllerTag, findMetadata)
}

func (s *metadataSuite) TestFindError(c *gc.C) {
	msg := "find error"
	s.state.findMetadata = func(f cloudimagemetadata.MetadataFilter) (map[string][]cloudimagemetadata.Metadata, error) {
		return nil, errors.New(msg)
	}

	found, err := s.api.List(params.ImageMetadataFilter{})
	c.Assert(err, gc.ErrorMatches, msg)
	c.Assert(found.Result, gc.HasLen, 0)
	s.assertCalls(c, controllerTag, findMetadata)
}

func (s *metadataSuite) TestFindOrder(c *gc.C) {
	customImageId := "custom1"
	customImageId2 := "custom2"
	customImageId3 := "custom3"
	publicImageId := "public1"

	s.state.findMetadata = func(f cloudimagemetadata.MetadataFilter) (map[string][]cloudimagemetadata.Metadata, error) {
		return map[string][]cloudimagemetadata.Metadata{
				"public": {
					{ImageId: publicImageId, Priority: 15},
				},
				"custom": {
					{ImageId: customImageId, Priority: 87},
					{ImageId: customImageId2, Priority: 20},
					{ImageId: customImageId3, Priority: 56},
				},
			},
			nil
	}

	found, err := s.api.List(params.ImageMetadataFilter{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Result, gc.HasLen, 4)

	c.Assert(found.Result, jc.SameContents, []params.CloudImageMetadata{
		{ImageId: customImageId, Priority: 87},
		{ImageId: customImageId3, Priority: 56},
		{ImageId: customImageId2, Priority: 20},
		{ImageId: publicImageId, Priority: 15},
	})
	s.assertCalls(c, controllerTag, findMetadata)
}

func (s *metadataSuite) TestSaveEmpty(c *gc.C) {
	errs, err := s.api.Save(params.MetadataSaveParams{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errs.Results, gc.HasLen, 0)
	s.assertCalls(c, controllerTag, model)
}

func (s *metadataSuite) TestSave(c *gc.C) {
	m1 := params.CloudImageMetadata{
		Source: "custom",
		Region: "east",
	}
	m2 := params.CloudImageMetadata{
		Source: "custom",
	}
	msg := "save error"

	saveCalls := 0
	s.state.saveMetadata = func(m []cloudimagemetadata.Metadata) error {
		saveCalls += 1
		if m[0].Region != "east" {
			c.Assert(m[0].Region, gc.Equals, "some-region")
		}
		// TODO (anastasiamac 2016-08-24) This is a check for a band-aid solution.
		// Once correct value is read from simplestreams, this needs to go.
		// Bug# 1616295
		// Ensure empty stream is changed to release
		c.Assert(m[0].Stream, gc.DeepEquals, "released")
		if saveCalls < 3 {
			// don't err on first or second call
			return nil
		}
		return errors.New(msg)
	}

	errs, err := s.api.Save(params.MetadataSaveParams{
		Metadata: []params.CloudImageMetadataList{{
			Metadata: []params.CloudImageMetadata{m1},
		}, {
			Metadata: []params.CloudImageMetadata{m2},
		}, {
			Metadata: []params.CloudImageMetadata{m1, m1},
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errs.Results, gc.HasLen, 3)
	c.Assert(errs.Results[0].Error, gc.IsNil)
	c.Assert(errs.Results[1].Error, gc.IsNil)
	c.Assert(errs.Results[2].Error, gc.ErrorMatches, msg)
	s.assertCalls(c, controllerTag, model, modelConfig, saveMetadata, saveMetadata, saveMetadata)
}

func (s *metadataSuite) TestDeleteEmpty(c *gc.C) {
	errs, err := s.api.Delete(params.MetadataImageIds{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errs.Results, gc.HasLen, 0)
	s.assertCalls(c, controllerTag)
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
	s.assertCalls(c, controllerTag, deleteMetadata, deleteMetadata)
}
