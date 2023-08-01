// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadatamanager_test

import (
	"regexp"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	basemocks "github.com/juju/juju/api/base/mocks"
	"github.com/juju/juju/api/client/imagemetadatamanager"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/rpc/params"
)

type imagemetadataSuite struct {
}

var _ = gc.Suite(&imagemetadataSuite{})

func (s *imagemetadataSuite) TestList(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	// setup data for test
	imageId := "imageid"
	stream := "stream"
	region := "region"

	// This is used by filters to search function
	base := corebase.MustParseBaseFromString("ubuntu@22.04")
	version := base.Channel.Track

	arch := "arch"
	virtType := "virt-type"
	rootStorageType := "root-storage-type"
	rootStorageSize := uint64(1024)
	source := "source"

	instances := []params.CloudImageMetadata{
		{
			ImageId:         imageId,
			Stream:          stream,
			Region:          region,
			Version:         version,
			Arch:            arch,
			VirtType:        virtType,
			RootStorageType: rootStorageType,
			RootStorageSize: &rootStorageSize,
			Source:          source,
		},
	}

	args := params.ImageMetadataFilter{
		Arches:          []string{arch},
		Stream:          stream,
		VirtType:        virtType,
		RootStorageType: rootStorageType,
		Region:          region,
		Versions:        []string{"22.04"},
	}
	res := new(params.ListCloudImageMetadataResult)
	ress := params.ListCloudImageMetadataResult{
		Result: instances,
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("List", args, res).SetArg(2, ress).Return(nil)
	client := imagemetadatamanager.NewClientFromCaller(mockFacadeCaller)
	found, err := client.List(
		stream, region,
		[]corebase.Base{base}, []string{arch},
		virtType, rootStorageType,
	)
	c.Check(err, jc.ErrorIsNil)
	c.Assert(found, jc.DeepEquals, instances)
}

func (s *imagemetadataSuite) TestListFacadeCallError(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	msg := "facade failure"
	args := params.ImageMetadataFilter{
		Stream:          "",
		Region:          "",
		Arches:          nil,
		VirtType:        "",
		RootStorageType: "",
		Versions:        []string{},
	}
	res := new(params.ListCloudImageMetadataResult)
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("List", args, res).Return(errors.New(msg))
	client := imagemetadatamanager.NewClientFromCaller(mockFacadeCaller)
	found, err := client.List("", "", nil, nil, "", "")
	c.Assert(errors.Cause(err), gc.ErrorMatches, msg)
	c.Assert(found, gc.HasLen, 0)
}

func (s *imagemetadataSuite) TestSave(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	m := params.CloudImageMetadata{}
	args := params.MetadataSaveParams{
		Metadata: []params.CloudImageMetadataList{
			{[]params.CloudImageMetadata{m, m}},
		},
	}
	res := new(params.ErrorResults)
	ress := params.ErrorResults{
		Results: []params.ErrorResult{{}},
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("Save", args, res).SetArg(2, ress).Return(nil)
	client := imagemetadatamanager.NewClientFromCaller(mockFacadeCaller)

	err := client.Save([]params.CloudImageMetadata{m, m})
	c.Check(err, jc.ErrorIsNil)
}

func (s *imagemetadataSuite) TestSaveFacadeCallError(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	m := []params.CloudImageMetadata{{}}
	msg := "facade failure"
	args := params.MetadataSaveParams{
		Metadata: []params.CloudImageMetadataList{
			{m},
		},
	}
	res := new(params.ErrorResults)
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("Save", args, res).Return(errors.New(msg))
	client := imagemetadatamanager.NewClientFromCaller(mockFacadeCaller)

	err := client.Save(m)
	c.Assert(errors.Cause(err), gc.ErrorMatches, msg)
}

func (s *imagemetadataSuite) TestSaveFacadeCallErrorResult(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	m := []params.CloudImageMetadata{{}}
	msg := "facade failure"
	args := params.MetadataSaveParams{
		Metadata: []params.CloudImageMetadataList{
			{m},
		},
	}
	res := new(params.ErrorResults)
	ress := params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: &params.Error{Message: msg}},
		},
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("Save", args, res).SetArg(2, ress).Return(nil)
	client := imagemetadatamanager.NewClientFromCaller(mockFacadeCaller)

	err := client.Save(m)
	c.Assert(errors.Cause(err), gc.ErrorMatches, msg)
}

func (s *imagemetadataSuite) TestDelete(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	imageId := "tst12345"
	args := params.MetadataImageIds{
		Ids: []string{imageId},
	}
	res := new(params.ErrorResults)
	ress := params.ErrorResults{
		Results: []params.ErrorResult{{}},
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("Delete", args, res).SetArg(2, ress).Return(nil)
	client := imagemetadatamanager.NewClientFromCaller(mockFacadeCaller)

	err := client.Delete(imageId)
	c.Check(err, jc.ErrorIsNil)
}

func (s *imagemetadataSuite) TestDeleteMultipleResult(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	imageId := "tst12345"
	args := params.MetadataImageIds{
		Ids: []string{imageId},
	}
	res := new(params.ErrorResults)
	ress := params.ErrorResults{
		Results: []params.ErrorResult{{}, {}},
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("Delete", args, res).SetArg(2, ress).Return(nil)
	client := imagemetadatamanager.NewClientFromCaller(mockFacadeCaller)

	err := client.Delete(imageId)
	c.Assert(err, gc.ErrorMatches, regexp.QuoteMeta(`expected to find one result for image id "tst12345" but found 2`))
}

func (s *imagemetadataSuite) TestDeleteFailure(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	msg := "save failure"
	args := params.MetadataImageIds{
		Ids: []string{"tst12345"},
	}
	res := new(params.ErrorResults)
	ress := params.ErrorResults{
		Results: []params.ErrorResult{{&params.Error{Message: msg}}},
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("Delete", args, res).SetArg(2, ress).Return(nil)
	client := imagemetadatamanager.NewClientFromCaller(mockFacadeCaller)

	err := client.Delete("tst12345")
	c.Assert(err, gc.ErrorMatches, msg)
}

func (s *imagemetadataSuite) TestDeleteFacadeCallError(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	msg := "facade failure"
	args := params.MetadataImageIds{
		Ids: []string{"tst12345"},
	}
	res := new(params.ErrorResults)
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("Delete", args, res).Return(errors.New(msg))
	client := imagemetadatamanager.NewClientFromCaller(mockFacadeCaller)

	err := client.Delete("tst12345")
	c.Assert(err, gc.ErrorMatches, msg)
}
