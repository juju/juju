// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oci_test

import (
	"context"
	"time"

	gomock "github.com/golang/mock/gomock"
	"github.com/juju/errors"
	ocitesting "github.com/juju/juju/provider/oci/testing"
	jujutesting "github.com/juju/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	ociCore "github.com/oracle/oci-go-sdk/core"

	"github.com/juju/juju/provider/oci"
)

type imagesSuite struct {
	jujutesting.BaseSuite

	testImageID     string
	testCompartment string
}

var _ = gc.Suite(&imagesSuite{})

func (i *imagesSuite) SetUpTest(c *gc.C) {
	i.BaseSuite.SetUpTest(c)
	oci.SetImageCache(&oci.ImageCache{})

	i.testImageID = "ocid1.image.oc1.phx.aaaaaaaaa5mikpf5fktj4x47bx4p4ak4g5jyuyukkxpdg4nll36qzqwjzd2q"
	i.testCompartment = "ocid1.compartment.oc1..aaaaaaaaakr75vvb5yx4nkm7ag7ekvluap7afa2y4zprswuprcnehqecwqga"
}

func (i *imagesSuite) TestNewImageVersion(c *gc.C) {
	name := "Canonical-Ubuntu-14.04-2017.08.22-0"
	img := ociCore.Image{
		DisplayName: &name,
	}
	timeStamp, _ := time.Parse("2006.01.02", "2017.08.22")
	version, err := oci.NewImageVersion(img)
	c.Assert(err, gc.IsNil)
	c.Assert(version.TimeStamp, gc.Equals, timeStamp)
	c.Assert(version.Revision, gc.Equals, 0)
}

func (i *imagesSuite) TestNewImageVersionInvalidDate(c *gc.C) {
	name := "Canonical-Ubuntu-14.04-NotARealDate-0"
	img := ociCore.Image{
		DisplayName: &name,
	}
	_, err := oci.NewImageVersion(img)
	c.Assert(err, gc.ErrorMatches, "parsing time for.*")
}

func (i *imagesSuite) TestNewImageVersionInvalidRevision(c *gc.C) {
	name := "Canonical-Ubuntu-14.04-2017.08.22-IShouldBeNumeric"
	img := ociCore.Image{
		DisplayName: &name,
	}
	_, err := oci.NewImageVersion(img)
	c.Assert(err, gc.ErrorMatches, "parsing revision for.*")
}

func (i *imagesSuite) TestNewImageVersionInvalidName(c *gc.C) {
	name := "fakeInvalidName"
	img := ociCore.Image{
		DisplayName: &name,
	}
	_, err := oci.NewImageVersion(img)
	c.Assert(err, gc.ErrorMatches, "invalid image display name.*")

	img = ociCore.Image{}
	_, err = oci.NewImageVersion(img)
	c.Assert(err, gc.ErrorMatches, "image does not have a display name")
}

func makeStringPointer(name string) *string {
	return &name
}

func makeIntPointer(name int) *int {
	return &name
}

func (i *imagesSuite) TestInstanceTypes(c *gc.C) {
	ctrl := gomock.NewController(c)
	compute := ocitesting.NewMockOCIComputeClient(ctrl)
	defer ctrl.Finish()

	request := ociCore.ListShapesRequest{
		CompartmentId: &i.testCompartment,
		ImageId:       &i.testImageID,
	}

	response := ociCore.ListShapesResponse{
		Items: []ociCore.Shape{
			{
				Shape: makeStringPointer("VM.Standard1.1"),
			},
			{
				Shape: makeStringPointer("VM.Standard2.1"),
			},
			{
				Shape: makeStringPointer("VM.Standard1.2"),
			},
		},
	}

	compute.EXPECT().ListShapes(context.Background(), request).Return(response, nil)

	types, err := oci.InstanceTypes(compute, &i.testCompartment, &i.testImageID)
	c.Assert(err, gc.IsNil)
	c.Check(types, gc.HasLen, 3)

	spec, ok := oci.ShapeSpecs["VM.Standard1.1"]
	c.Assert(ok, jc.IsTrue)
	c.Check(int(types[0].Mem), gc.Equals, spec.Memory)
	c.Check(int(types[0].CpuCores), gc.Equals, spec.Cpus)

	spec, ok = oci.ShapeSpecs["VM.Standard2.1"]
	c.Assert(ok, jc.IsTrue)
	c.Check(int(types[1].Mem), gc.Equals, spec.Memory)
	c.Check(int(types[1].CpuCores), gc.Equals, spec.Cpus)

	spec, ok = oci.ShapeSpecs["VM.Standard1.2"]
	c.Assert(ok, jc.IsTrue)
	c.Check(int(types[2].Mem), gc.Equals, spec.Memory)
	c.Check(int(types[2].CpuCores), gc.Equals, spec.Cpus)
}

func (i *imagesSuite) TestInstanceTypesNilClient(c *gc.C) {
	_, err := oci.InstanceTypes(nil, &i.testCompartment, &i.testImageID)
	c.Assert(err, gc.ErrorMatches, "cannot use nil client")
}

func (i *imagesSuite) TestInstanceTypesImageWithUnknownShape(c *gc.C) {
	ctrl := gomock.NewController(c)
	compute := ocitesting.NewMockOCIComputeClient(ctrl)
	defer ctrl.Finish()

	request := ociCore.ListShapesRequest{
		CompartmentId: &i.testCompartment,
		ImageId:       &i.testImageID,
	}

	response := ociCore.ListShapesResponse{
		Items: []ociCore.Shape{
			{
				Shape: makeStringPointer("IDontExistInTheOCIProviderWasProbablyAddedLaterAndThatsWhyIHopeTheyWillAddResourceDetailsToShapesAPISoWeDontNeedToMaintainAMapping"),
			},
			{
				Shape: makeStringPointer("VM.Standard2.1"),
			},
			{
				Shape: makeStringPointer("VM.Standard1.2"),
			},
		},
	}

	compute.EXPECT().ListShapes(context.Background(), request).Return(response, nil)

	types, err := oci.InstanceTypes(compute, &i.testCompartment, &i.testImageID)
	c.Assert(err, gc.IsNil)
	c.Check(types, gc.HasLen, 2)
}

func (i *imagesSuite) TestNewInstanceImage(c *gc.C) {
	image := ociCore.Image{
		CompartmentId:          &i.testCompartment,
		Id:                     &i.testImageID,
		OperatingSystem:        makeStringPointer("Canonical Ubuntu"),
		OperatingSystemVersion: makeStringPointer("14.04"),
		DisplayName:            makeStringPointer("Canonical-Ubuntu-14.04-2018.01.11-0"),
	}

	imgType, err := oci.NewInstanceImage(image, &i.testCompartment)
	c.Assert(err, gc.IsNil)
	c.Check(imgType.ImageType, gc.Equals, oci.ImageTypeGeneric)
	c.Check(imgType.Series, gc.Equals, "trusty")
	c.Check(imgType.CompartmentId, gc.NotNil)
	c.Check(*imgType.CompartmentId, gc.Equals, i.testCompartment)
	c.Check(imgType.Id, gc.Equals, i.testImageID)
}

func (i *imagesSuite) TestNewInstanceImageUnknownOS(c *gc.C) {
	image := ociCore.Image{
		CompartmentId:          &i.testCompartment,
		Id:                     &i.testImageID,
		OperatingSystem:        makeStringPointer("NotKnownToJuju"),
		OperatingSystemVersion: makeStringPointer("14.04"),
		DisplayName:            makeStringPointer("Canonical-Ubuntu-14.04-2018.01.11-0"),
	}

	_, err := oci.NewInstanceImage(image, &i.testCompartment)
	c.Assert(err, gc.ErrorMatches, "os NotKnownToJuju not supported")
}

func (i *imagesSuite) TestRefreshImageCache(c *gc.C) {
	ctrl := gomock.NewController(c)
	compute := ocitesting.NewMockOCIComputeClient(ctrl)
	defer ctrl.Finish()

	fakeUbuntuID := "fakeUbuntu1"
	fakeUbuntuIDSecond := "fakeUbuntu2"

	listImageRequest, listImageResponse := makeListImageRequestResponse(
		[]ociCore.Image{
			{
				CompartmentId:          &i.testCompartment,
				Id:                     &fakeUbuntuID,
				OperatingSystem:        makeStringPointer("Canonical Ubuntu"),
				OperatingSystemVersion: makeStringPointer("14.04"),
				DisplayName:            makeStringPointer("Canonical-Ubuntu-14.04-2018.01.11-0"),
			},
			{
				CompartmentId:          &i.testCompartment,
				Id:                     &fakeUbuntuIDSecond,
				OperatingSystem:        makeStringPointer("Canonical Ubuntu"),
				OperatingSystemVersion: makeStringPointer("14.04"),
				DisplayName:            makeStringPointer("Canonical-Ubuntu-14.04-2018.01.12-0"),
			},
			{
				CompartmentId:          &i.testCompartment,
				Id:                     makeStringPointer("fakeCentOS"),
				OperatingSystem:        makeStringPointer("CentOS"),
				OperatingSystemVersion: makeStringPointer("7"),
				DisplayName:            makeStringPointer("CentOS-7-2017.10.19-0"),
			},
		},
	)
	shapesRequestUbuntu, shapesResponseUbuntu := makeShapesRequestResponse(
		i.testCompartment, fakeUbuntuID, []string{"VM.Standard2.1", "VM.Standard1.2"})

	shapesRequestUbuntuSecond := ociCore.ListShapesRequest{
		CompartmentId: &i.testCompartment,
		ImageId:       &fakeUbuntuIDSecond,
	}

	shapesRequestCentOS, shapesResponseCentOS := makeShapesRequestResponse(
		i.testCompartment, "fakeCentOS", []string{"VM.Standard1.2"})

	gomock.InOrder(
		compute.EXPECT().ListImages(context.Background(), listImageRequest).Return(listImageResponse, nil),
		compute.EXPECT().ListShapes(context.Background(), shapesRequestUbuntu).Return(shapesResponseUbuntu, nil),
		compute.EXPECT().ListShapes(context.Background(), shapesRequestUbuntuSecond).Return(shapesResponseUbuntu, nil),
		compute.EXPECT().ListShapes(context.Background(), shapesRequestCentOS).Return(shapesResponseCentOS, nil),
	)

	imgCache, err := oci.RefreshImageCache(compute, &i.testCompartment)
	c.Assert(err, gc.IsNil)
	c.Assert(imgCache, gc.NotNil)
	c.Check(imgCache.ImageMap(), gc.HasLen, 2)

	imageMap := imgCache.ImageMap()
	c.Check(imageMap["trusty"], gc.HasLen, 2)
	c.Check(imageMap["centos7"], gc.HasLen, 1)

	timeStamp, _ := time.Parse("2006.01.02", "2018.01.12")

	// Check that the first image in the array is the newest one
	c.Assert(imageMap["trusty"][0].Version.TimeStamp, gc.Equals, timeStamp)

	// Check that InstanceTypes are set
	c.Assert(imageMap["trusty"][0].InstanceTypes, gc.HasLen, 2)
	c.Assert(imageMap["centos7"][0].InstanceTypes, gc.HasLen, 1)
}

func (i *imagesSuite) TestRefreshImageCacheFetchFromCache(c *gc.C) {
	ctrl := gomock.NewController(c)
	compute := ocitesting.NewMockOCIComputeClient(ctrl)
	defer ctrl.Finish()

	compute.EXPECT().ListImages(gomock.Any(), gomock.Any()).Return(ociCore.ListImagesResponse{}, nil)

	imgCache, err := oci.RefreshImageCache(compute, &i.testCompartment)
	c.Assert(err, gc.IsNil)
	c.Assert(imgCache, gc.NotNil)

	fromCache, err := oci.RefreshImageCache(compute, &i.testCompartment)
	c.Assert(err, gc.IsNil)
	c.Check(imgCache, gc.DeepEquals, fromCache)
}

func (i *imagesSuite) TestRefreshImageCacheStaleCache(c *gc.C) {
	ctrl := gomock.NewController(c)
	compute := ocitesting.NewMockOCIComputeClient(ctrl)
	defer ctrl.Finish()

	compute.EXPECT().ListImages(gomock.Any(), gomock.Any()).Return(ociCore.ListImagesResponse{}, nil).Times(2)

	imgCache, err := oci.RefreshImageCache(compute, &i.testCompartment)
	c.Assert(err, gc.IsNil)
	c.Assert(imgCache, gc.NotNil)

	now := time.Now()

	// No need to check the value. gomock will assert if ListImages
	// is not called twice
	imgCache.SetLastRefresh(now.Add(-31 * time.Minute))
	_, err = oci.RefreshImageCache(compute, &i.testCompartment)
	c.Assert(err, gc.IsNil)
}

func (i *imagesSuite) TestRefreshImageCacheWithInvalidImage(c *gc.C) {
	ctrl := gomock.NewController(c)
	compute := ocitesting.NewMockOCIComputeClient(ctrl)
	defer ctrl.Finish()

	listImageRequest, listImageResponse := makeListImageRequestResponse(
		[]ociCore.Image{
			{
				CompartmentId:          &i.testCompartment,
				Id:                     makeStringPointer("fakeUbuntu1"),
				OperatingSystem:        makeStringPointer("Canonical Ubuntu"),
				OperatingSystemVersion: makeStringPointer("14.04"),
				DisplayName:            makeStringPointer("Canonical-Ubuntu-14.04-2018.01.11-0"),
			},
			{
				CompartmentId:          &i.testCompartment,
				Id:                     makeStringPointer("fake image id for bad image"),
				OperatingSystem:        makeStringPointer("CentOS"),
				OperatingSystemVersion: makeStringPointer("7"),
				DisplayName:            makeStringPointer("BadlyFormatedDisplayName_IshouldBeIgnored"),
			},
		},
	)

	shapesRequestUbuntu, shapesResponseUbuntu := makeShapesRequestResponse(
		i.testCompartment, "fakeUbuntu1", []string{"VM.Standard2.1", "VM.Standard1.2"})

	shapesRequestBadImage, shapesResponseBadImage := makeShapesRequestResponse(
		i.testCompartment, "fake image id for bad image", []string{"VM.Standard1.2"})

	gomock.InOrder(
		compute.EXPECT().ListImages(context.Background(), listImageRequest).Return(listImageResponse, nil),
		compute.EXPECT().ListShapes(context.Background(), shapesRequestUbuntu).Return(shapesResponseUbuntu, nil),
		compute.EXPECT().ListShapes(context.Background(), shapesRequestBadImage).Return(shapesResponseBadImage, nil),
	)

	imgCache, err := oci.RefreshImageCache(compute, &i.testCompartment)
	c.Assert(err, gc.IsNil)
	c.Assert(imgCache, gc.NotNil)
	c.Check(imgCache.ImageMap(), gc.HasLen, 1)
	imageMap := imgCache.ImageMap()

	c.Check(imageMap["trusty"][0].Id, gc.Equals, "fakeUbuntu1")
}

func (i *imagesSuite) TestRefreshImageCacheWithAPIError(c *gc.C) {
	ctrl := gomock.NewController(c)
	compute := ocitesting.NewMockOCIComputeClient(ctrl)
	defer ctrl.Finish()

	compute.EXPECT().ListImages(gomock.Any(), gomock.Any()).Return(ociCore.ListImagesResponse{}, errors.Errorf("awww snap!"))
	_, err := oci.RefreshImageCache(compute, &i.testCompartment)
	c.Assert(err, gc.ErrorMatches, "listing provider images.*awww snap!")
}

func (i *imagesSuite) TestImageMetadataFromCache(c *gc.C) {
	image := ociCore.Image{
		CompartmentId:          &i.testCompartment,
		Id:                     &i.testImageID,
		OperatingSystem:        makeStringPointer("Canonical Ubuntu"),
		OperatingSystemVersion: makeStringPointer("14.04"),
		DisplayName:            makeStringPointer("Canonical-Ubuntu-14.04-2018.01.11-0"),
	}

	imgType, err := oci.NewInstanceImage(image, &i.testCompartment)
	c.Assert(err, gc.IsNil)

	cache := &oci.ImageCache{}
	images := map[string][]oci.InstanceImage{
		"trusty": {
			imgType,
		},
	}
	cache.SetImages(images)
	metadata := cache.ImageMetadata("trusty", "")
	c.Assert(metadata, gc.HasLen, 1)
	// generic images default to ImageTypeVM
	c.Assert(metadata[0].VirtType, gc.Equals, string(oci.ImageTypeVM))

	// explicitly set ImageTypeBM on generic images
	metadata = cache.ImageMetadata("trusty", string(oci.ImageTypeBM))
	c.Assert(metadata, gc.HasLen, 1)
	c.Assert(metadata[0].VirtType, gc.Equals, string(oci.ImageTypeBM))
}

func (i *imagesSuite) TestImageMetadataSpecificImageType(c *gc.C) {
	image := ociCore.Image{
		CompartmentId:          &i.testCompartment,
		Id:                     &i.testImageID,
		OperatingSystem:        makeStringPointer("Canonical Ubuntu"),
		OperatingSystemVersion: makeStringPointer("14.04"),
		DisplayName:            makeStringPointer("Canonical-Ubuntu-14.04-Gen2-GPU-2018.01.11-0"),
	}

	imgType, err := oci.NewInstanceImage(image, &i.testCompartment)
	c.Assert(err, gc.IsNil)

	cache := &oci.ImageCache{}
	images := map[string][]oci.InstanceImage{
		"trusty": {
			imgType,
		},
	}
	cache.SetImages(images)
	metadata := cache.ImageMetadata("trusty", "")
	c.Assert(metadata, gc.HasLen, 1)
	// generic images default to ImageTypeVM
	c.Assert(metadata[0].VirtType, gc.Equals, string(oci.ImageTypeGPU))

	// explicitly set ImageTypeBM on generic images
	metadata = cache.ImageMetadata("trusty", string(oci.ImageTypeBM))
	c.Assert(metadata, gc.HasLen, 1)
	c.Assert(metadata[0].VirtType, gc.Equals, string(oci.ImageTypeGPU))
}
