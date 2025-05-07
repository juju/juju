// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadatamanager_test

import (
	"context"
	"regexp"

	"github.com/juju/errors"
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"

	basemocks "github.com/juju/juju/api/base/mocks"
	"github.com/juju/juju/api/client/imagemetadatamanager"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/rpc/params"
)

type imagemetadataSuite struct {
}

var _ = tc.Suite(&imagemetadataSuite{})

func (s *imagemetadataSuite) TestList(c *tc.C) {
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
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "List", args, res).SetArg(3, ress).Return(nil)
	client := imagemetadatamanager.NewClientFromCaller(mockFacadeCaller)
	found, err := client.List(
		context.Background(),
		stream, region,
		[]corebase.Base{base}, []string{arch},
		virtType, rootStorageType,
	)
	c.Check(err, jc.ErrorIsNil)
	c.Assert(found, jc.DeepEquals, instances)
}

func (s *imagemetadataSuite) TestListFacadeCallError(c *tc.C) {
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
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "List", args, res).Return(errors.New(msg))
	client := imagemetadatamanager.NewClientFromCaller(mockFacadeCaller)
	found, err := client.List(context.Background(), "", "", nil, nil, "", "")
	c.Assert(errors.Cause(err), tc.ErrorMatches, msg)
	c.Assert(found, tc.HasLen, 0)
}

func (s *imagemetadataSuite) TestSave(c *tc.C) {
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
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "Save", args, res).SetArg(3, ress).Return(nil)
	client := imagemetadatamanager.NewClientFromCaller(mockFacadeCaller)

	err := client.Save(context.Background(), []params.CloudImageMetadata{m, m})
	c.Check(err, jc.ErrorIsNil)
}

func (s *imagemetadataSuite) TestSaveFacadeCallError(c *tc.C) {
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
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "Save", args, res).Return(errors.New(msg))
	client := imagemetadatamanager.NewClientFromCaller(mockFacadeCaller)

	err := client.Save(context.Background(), m)
	c.Assert(errors.Cause(err), tc.ErrorMatches, msg)
}

func (s *imagemetadataSuite) TestSaveFacadeCallErrorResult(c *tc.C) {
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
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "Save", args, res).SetArg(3, ress).Return(nil)
	client := imagemetadatamanager.NewClientFromCaller(mockFacadeCaller)

	err := client.Save(context.Background(), m)
	c.Assert(errors.Cause(err), tc.ErrorMatches, msg)
}

func (s *imagemetadataSuite) TestDelete(c *tc.C) {
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
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "Delete", args, res).SetArg(3, ress).Return(nil)
	client := imagemetadatamanager.NewClientFromCaller(mockFacadeCaller)

	err := client.Delete(context.Background(), imageId)
	c.Check(err, jc.ErrorIsNil)
}

func (s *imagemetadataSuite) TestDeleteMultipleResult(c *tc.C) {
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
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "Delete", args, res).SetArg(3, ress).Return(nil)
	client := imagemetadatamanager.NewClientFromCaller(mockFacadeCaller)

	err := client.Delete(context.Background(), imageId)
	c.Assert(err, tc.ErrorMatches, regexp.QuoteMeta(`expected to find one result for image id "tst12345" but found 2`))
}

func (s *imagemetadataSuite) TestDeleteFailure(c *tc.C) {
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
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "Delete", args, res).SetArg(3, ress).Return(nil)
	client := imagemetadatamanager.NewClientFromCaller(mockFacadeCaller)

	err := client.Delete(context.Background(), "tst12345")
	c.Assert(err, tc.ErrorMatches, msg)
}

func (s *imagemetadataSuite) TestDeleteFacadeCallError(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	msg := "facade failure"
	args := params.MetadataImageIds{
		Ids: []string{"tst12345"},
	}
	res := new(params.ErrorResults)
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "Delete", args, res).Return(errors.New(msg))
	client := imagemetadatamanager.NewClientFromCaller(mockFacadeCaller)

	err := client.Delete(context.Background(), "tst12345")
	c.Assert(err, tc.ErrorMatches, msg)
}
