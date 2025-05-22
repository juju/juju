// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadatamanager

import (
	"context"
	"testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/domain/cloudimagemetadata"
	"github.com/juju/juju/environs/config"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type metadataSuite struct {
	baseImageMetadataSuite
}

func TestMetadataSuite(t *testing.T) {
	tc.Run(t, &metadataSuite{})
}

func (s *metadataSuite) TestFindNil(c *tc.C) {
	defer s.setupAPI(c).Finish()

	s.metadataService.EXPECT().FindMetadata(gomock.Any(), cloudimagemetadata.MetadataFilter{}).Return(nil, nil)

	found, err := s.api.List(c.Context(), params.ImageMetadataFilter{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(found.Result, tc.HasLen, 0)
}

func (s *metadataSuite) TestFindEmpty(c *tc.C) {
	defer s.setupAPI(c).Finish()

	s.metadataService.EXPECT().FindMetadata(gomock.Any(), gomock.Any()).Return(map[string][]cloudimagemetadata.Metadata{}, nil)

	found, err := s.api.List(c.Context(), params.ImageMetadataFilter{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(found.Result, tc.HasLen, 0)
}

func (s *metadataSuite) TestFindEmptyGroups(c *tc.C) {
	defer s.setupAPI(c).Finish()

	s.metadataService.EXPECT().FindMetadata(gomock.Any(), gomock.Any()).Return(map[string][]cloudimagemetadata.Metadata{
		"public": {},
		"custom": {},
	}, nil)

	found, err := s.api.List(c.Context(), params.ImageMetadataFilter{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(found.Result, tc.HasLen, 0)
}

func (s *metadataSuite) TestFindError(c *tc.C) {
	defer s.setupAPI(c).Finish()

	expectedError := errors.New("find error")
	s.metadataService.EXPECT().FindMetadata(gomock.Any(), gomock.Any()).Return(nil, expectedError)

	found, err := s.api.List(c.Context(), params.ImageMetadataFilter{})
	c.Assert(err, tc.ErrorMatches, expectedError.Error())
	c.Assert(found.Result, tc.HasLen, 0)
}

func (s *metadataSuite) TestFindOrder(c *tc.C) {
	defer s.setupAPI(c).Finish()

	customImageId := "custom1"
	customImageId2 := "custom2"
	customImageId3 := "custom3"
	publicImageId := "public1"

	s.metadataService.EXPECT().FindMetadata(gomock.Any(), gomock.Any()).Return(map[string][]cloudimagemetadata.Metadata{
		"public": {
			{ImageID: publicImageId, Priority: 15},
		},
		"custom": {
			{ImageID: customImageId, Priority: 87},
			{ImageID: customImageId2, Priority: 20},
			{ImageID: customImageId3, Priority: 56},
		},
	}, nil)

	found, err := s.api.List(c.Context(), params.ImageMetadataFilter{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(found.Result, tc.HasLen, 4)

	c.Assert(found.Result, tc.SameContents, []params.CloudImageMetadata{
		{ImageId: customImageId, Priority: 87},
		{ImageId: customImageId3, Priority: 56},
		{ImageId: customImageId2, Priority: 20},
		{ImageId: publicImageId, Priority: 15},
	})
}

func (s *metadataSuite) TestSaveEmpty(c *tc.C) {
	defer s.setupAPI(c).Finish()

	s.modelInfoService.EXPECT().GetModelInfo(gomock.Any()).Return(
		coremodel.ModelInfo{
			CloudRegion: "region1",
		}, nil,
	)

	errs, err := s.api.Save(c.Context(), params.MetadataSaveParams{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(errs.Results, tc.HasLen, 0)
}

func (s *metadataSuite) TestSave(c *tc.C) {
	defer s.setupAPI(c).Finish()

	s.modelConfigService.EXPECT().ModelConfig(gomock.Any()).DoAndReturn(func(v any) (*config.Config, error) {
		return config.New(config.UseDefaults, coretesting.FakeConfig())
	})
	s.modelInfoService.EXPECT().GetModelInfo(gomock.Any()).Return(
		coremodel.ModelInfo{
			CloudRegion: "east",
		}, nil,
	)

	m1 := params.CloudImageMetadata{
		Source: "custom",
		Region: "east",
	}
	m2 := params.CloudImageMetadata{
		Source: "custom",
	}
	msg := "save error"

	saveCalls := 0
	s.metadataService.EXPECT().SaveMetadata(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, m []cloudimagemetadata.Metadata) error {
		saveCalls += 1
		if m[0].Region != "east" {
			c.Assert(m[0].Region, tc.Equals, "some-region")
		}
		// TODO (anastasiamac 2016-08-24) This is a check for a band-aid solution.
		// Once correct value is read from simplestreams, this needs to go.
		// Bug# 1616295
		// Ensure empty stream is changed to release
		c.Assert(m[0].Stream, tc.DeepEquals, "released")
		if saveCalls < 3 {
			// don't err on first or second call
			return nil
		}
		return errors.New(msg)
	}).Times(3)

	errs, err := s.api.Save(c.Context(), params.MetadataSaveParams{
		Metadata: []params.CloudImageMetadataList{{
			Metadata: []params.CloudImageMetadata{m1},
		}, {
			Metadata: []params.CloudImageMetadata{m2},
		}, {
			Metadata: []params.CloudImageMetadata{m1, m1},
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(errs.Results, tc.HasLen, 3)
	c.Assert(errs.Results[0].Error, tc.IsNil)
	c.Assert(errs.Results[1].Error, tc.IsNil)
	c.Assert(errs.Results[2].Error, tc.ErrorMatches, msg)
}

func (s *metadataSuite) TestSaveModelCfgFailed(c *tc.C) {
	defer s.setupAPI(c).Finish()

	m := params.CloudImageMetadata{
		Source: "custom",
	}
	ms := params.MetadataSaveParams{
		Metadata: []params.CloudImageMetadataList{{
			Metadata: []params.CloudImageMetadata{m},
		}},
	}

	msg := "save error"
	s.modelConfigService.EXPECT().ModelConfig(gomock.Any()).Return(nil, errors.New(msg))
	s.modelInfoService.EXPECT().GetModelInfo(gomock.Any()).Return(
		coremodel.ModelInfo{
			CloudRegion: "region1",
		}, nil,
	)

	errs, err := s.api.Save(c.Context(), ms)
	c.Assert(errors.Cause(err), tc.ErrorMatches, msg)
	c.Assert(errs.Results, tc.HasLen, 0)
}

func (s *metadataSuite) TestDeleteEmpty(c *tc.C) {
	defer s.setupAPI(c).Finish()

	errs, err := s.api.Delete(c.Context(), params.MetadataImageIds{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(errs.Results, tc.HasLen, 0)
}

func (s *metadataSuite) TestDelete(c *tc.C) {
	defer s.setupAPI(c).Finish()

	idOk := "ok"
	idFail := "fail"
	msg := "delete error"

	s.metadataService.EXPECT().DeleteMetadataWithImageID(gomock.Any(), idFail).Return(errors.New(msg))
	s.metadataService.EXPECT().DeleteMetadataWithImageID(gomock.Any(), idOk).Return(nil)

	errs, err := s.api.Delete(c.Context(), params.MetadataImageIds{[]string{idOk, idFail}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(errs.Results, tc.HasLen, 2)
	c.Assert(errs.Results[0].Error, tc.IsNil)
	c.Assert(errs.Results[1].Error, tc.ErrorMatches, msg)
}
