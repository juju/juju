// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oci_test

import (
	"context"
	"time"

	ociCore "github.com/oracle/oci-go-sdk/v65/core"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/arch"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/provider/oci"
	ocitesting "github.com/juju/juju/provider/oci/testing"
	jujutesting "github.com/juju/juju/testing"
)

type imagesSuite struct {
	jujutesting.BaseSuite

	testImageID     string
	testCompartment string
}

var _ = gc.Suite(&imagesSuite{})

func (s *imagesSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	oci.SetImageCache(&oci.ImageCache{})

	s.testImageID = "ocid1.image.oc1.phx.aaaaaaaaa5mikpf5fktj4x47bx4p4ak4g5jyuyukkxpdg4nll36qzqwjzd2q"
	s.testCompartment = "ocid1.compartment.oc1..aaaaaaaaakr75vvb5yx4nkm7ag7ekvluap7afa2y4zprswuprcnehqecwqga"
}

func (s *imagesSuite) TestNewImageVersion(c *gc.C) {
	name := "Canonical-Ubuntu-22.04-2017.08.22-0"
	img := ociCore.Image{
		DisplayName: &name,
	}
	timeStamp, _ := time.Parse("2006.01.02", "2017.08.22")
	version, err := oci.NewImageVersion(img)
	c.Assert(err, gc.IsNil)
	c.Assert(version.TimeStamp, gc.Equals, timeStamp)
	c.Assert(version.Revision, gc.Equals, 0)
}

func (s *imagesSuite) TestNewImageVersionInvalidDate(c *gc.C) {
	name := "Canonical-Ubuntu-22.04-NotARealDate-0"
	img := ociCore.Image{
		DisplayName: &name,
	}
	_, err := oci.NewImageVersion(img)
	c.Assert(err, gc.ErrorMatches, "parsing time for.*")
}

func (s *imagesSuite) TestNewImageVersionInvalidRevision(c *gc.C) {
	name := "Canonical-Ubuntu-22.04-2017.08.22-IShouldBeNumeric"
	img := ociCore.Image{
		DisplayName: &name,
	}
	_, err := oci.NewImageVersion(img)
	c.Assert(err, gc.ErrorMatches, "parsing revision for.*")
}

func (s *imagesSuite) TestNewImageVersionInvalidName(c *gc.C) {
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

func makeBoolPointer(name bool) *bool {
	return &name
}
func makeFloat32Pointer(name float32) *float32 {
	return &name
}

func (s *imagesSuite) TestInstanceTypes(c *gc.C) {
	ctrl := gomock.NewController(c)
	compute := ocitesting.NewMockComputeClient(ctrl)
	defer ctrl.Finish()

	compute.EXPECT().ListShapes(context.Background(), &s.testCompartment, &s.testImageID).Return(listShapesResponse(), nil)

	types, err := oci.InstanceTypes(compute, &s.testCompartment, &s.testImageID)
	c.Assert(err, gc.IsNil)
	c.Check(types, gc.HasLen, 4)
	expectedTypes := []instances.InstanceType{
		{
			Name:     "VM.Standard1.1",
			Arch:     arch.AMD64,
			Mem:      7 * 1024,
			CpuCores: 1,
			VirtType: makeStringPointer("vm"),
		}, {
			Name:     "VM.GPU.A10.1",
			Arch:     arch.AMD64,
			Mem:      240 * 1024,
			CpuCores: 15,
			VirtType: makeStringPointer("gpu"),
		}, {
			Name:     "BM.Standard.A1.160",
			Arch:     arch.ARM64,
			Mem:      1024 * 1024,
			CpuCores: 160,
			VirtType: makeStringPointer("metal"),
		}, {
			Name:     "VM.Standard.A1.Flex",
			Arch:     arch.ARM64,
			Mem:      6 * 1024,
			CpuCores: 1,
			VirtType: makeStringPointer("vm"),
		},
	}
	c.Assert(types, gc.DeepEquals, expectedTypes)
}

func (s *imagesSuite) TestInstanceTypesNilClient(c *gc.C) {
	_, err := oci.InstanceTypes(nil, &s.testCompartment, &s.testImageID)
	c.Assert(err, gc.ErrorMatches, "cannot use nil client")
}

func (s *imagesSuite) TestNewInstanceImage(c *gc.C) {
	image := ociCore.Image{
		CompartmentId:          &s.testCompartment,
		Id:                     &s.testImageID,
		OperatingSystem:        makeStringPointer("Canonical Ubuntu"),
		OperatingSystemVersion: makeStringPointer("22.04"),
		DisplayName:            makeStringPointer("Canonical-Ubuntu-22.04-2018.01.11-0"),
	}

	imgType, err := oci.NewInstanceImage(image, &s.testCompartment)
	c.Assert(err, gc.IsNil)
	c.Check(imgType.ImageType, gc.Equals, oci.ImageTypeGeneric)
	c.Check(imgType.Base.DisplayString(), gc.Equals, "ubuntu@22.04")
	c.Check(imgType.CompartmentId, gc.NotNil)
	c.Check(*imgType.CompartmentId, gc.Equals, s.testCompartment)
	c.Check(imgType.Id, gc.Equals, s.testImageID)
}

func (s *imagesSuite) TestNewInstanceImageUnknownOS(c *gc.C) {
	image := ociCore.Image{
		CompartmentId:          &s.testCompartment,
		Id:                     &s.testImageID,
		OperatingSystem:        makeStringPointer("NotKnownToJuju"),
		OperatingSystemVersion: makeStringPointer("22.04"),
		DisplayName:            makeStringPointer("Canonical-Ubuntu-22.04-2018.01.11-0"),
	}

	_, err := oci.NewInstanceImage(image, &s.testCompartment)
	c.Assert(err, gc.ErrorMatches, "os NotKnownToJuju not supported")
}

func (s *imagesSuite) TestRefreshImageCache(c *gc.C) {
	ctrl := gomock.NewController(c)
	compute := ocitesting.NewMockComputeClient(ctrl)
	defer ctrl.Finish()

	fakeUbuntuID := "fakeUbuntu1"
	fakeUbuntuIDSecond := "fakeUbuntu2"
	fakeCentOSID := "fakeCentOS"

	listImageResponse := []ociCore.Image{
		{
			CompartmentId:          &s.testCompartment,
			Id:                     &fakeUbuntuID,
			OperatingSystem:        makeStringPointer("Canonical Ubuntu"),
			OperatingSystemVersion: makeStringPointer("22.04"),
			DisplayName:            makeStringPointer("Canonical-Ubuntu-22.04-2018.01.11-0"),
		},
		{
			CompartmentId:          &s.testCompartment,
			Id:                     &fakeUbuntuIDSecond,
			OperatingSystem:        makeStringPointer("Canonical Ubuntu"),
			OperatingSystemVersion: makeStringPointer("22.04"),
			DisplayName:            makeStringPointer("Canonical-Ubuntu-22.04-2018.01.12-0"),
		},
		{
			CompartmentId:          &s.testCompartment,
			Id:                     makeStringPointer("fakeCentOS"),
			OperatingSystem:        makeStringPointer("CentOS"),
			OperatingSystemVersion: makeStringPointer("7"),
			DisplayName:            makeStringPointer("CentOS-7-2017.10.19-0"),
		},
	}

	compute.EXPECT().ListImages(context.Background(), &s.testCompartment).Return(listImageResponse, nil)
	compute.EXPECT().ListShapes(context.Background(), &s.testCompartment, &fakeUbuntuID).Return(listShapesResponse(), nil)
	compute.EXPECT().ListShapes(context.Background(), &s.testCompartment, &fakeUbuntuIDSecond).Return(listShapesResponse(), nil)
	compute.EXPECT().ListShapes(context.Background(), &s.testCompartment, &fakeCentOSID).Return(listShapesResponse()[:2], nil)

	imgCache, err := oci.RefreshImageCache(compute, &s.testCompartment)
	c.Assert(err, gc.IsNil)
	c.Assert(imgCache, gc.NotNil)
	c.Check(imgCache.ImageMap(), gc.HasLen, 2)

	imageMap := imgCache.ImageMap()
	jammy := corebase.MakeDefaultBase("ubuntu", "22.04")
	c.Check(imageMap[jammy], gc.HasLen, 2)
	c.Check(imageMap[corebase.MakeDefaultBase("centos", "7")], gc.HasLen, 1)

	timeStamp, _ := time.Parse("2006.01.02", "2018.01.12")

	// Check that the first image in the array is the newest one
	c.Assert(imageMap[jammy][0].Version.TimeStamp, gc.Equals, timeStamp)

	// Check that InstanceTypes are set
	c.Assert(imageMap[jammy][0].InstanceTypes, gc.HasLen, 4)
	c.Assert(imageMap[corebase.MakeDefaultBase("centos", "7")][0].InstanceTypes, gc.HasLen, 2)
}

func (s *imagesSuite) TestRefreshImageCacheFetchFromCache(c *gc.C) {
	ctrl := gomock.NewController(c)
	compute := ocitesting.NewMockComputeClient(ctrl)
	defer ctrl.Finish()

	compute.EXPECT().ListImages(gomock.Any(), gomock.Any()).Return([]ociCore.Image{}, nil)

	imgCache, err := oci.RefreshImageCache(compute, &s.testCompartment)
	c.Assert(err, gc.IsNil)
	c.Assert(imgCache, gc.NotNil)

	fromCache, err := oci.RefreshImageCache(compute, &s.testCompartment)
	c.Assert(err, gc.IsNil)
	c.Check(imgCache, gc.DeepEquals, fromCache)
}

func (s *imagesSuite) TestRefreshImageCacheStaleCache(c *gc.C) {
	ctrl := gomock.NewController(c)
	compute := ocitesting.NewMockComputeClient(ctrl)
	defer ctrl.Finish()

	compute.EXPECT().ListImages(gomock.Any(), gomock.Any()).Return([]ociCore.Image{}, nil).Times(2)

	imgCache, err := oci.RefreshImageCache(compute, &s.testCompartment)
	c.Assert(err, gc.IsNil)
	c.Assert(imgCache, gc.NotNil)

	now := time.Now()

	// No need to check the value. gomock will assert if ListImages
	// is not called twice
	imgCache.SetLastRefresh(now.Add(-31 * time.Minute))
	_, err = oci.RefreshImageCache(compute, &s.testCompartment)
	c.Assert(err, gc.IsNil)
}

func (s *imagesSuite) TestRefreshImageCacheWithInvalidImage(c *gc.C) {
	ctrl := gomock.NewController(c)
	compute := ocitesting.NewMockComputeClient(ctrl)
	defer ctrl.Finish()

	listImageResponse := []ociCore.Image{
		{
			CompartmentId:          &s.testCompartment,
			Id:                     makeStringPointer("fakeUbuntu1"),
			OperatingSystem:        makeStringPointer("Canonical Ubuntu"),
			OperatingSystemVersion: makeStringPointer("22.04"),
			DisplayName:            makeStringPointer("Canonical-Ubuntu-22.04-2018.01.11-0"),
		},
		{
			CompartmentId:          &s.testCompartment,
			Id:                     makeStringPointer("fake image id for bad image"),
			OperatingSystem:        makeStringPointer("CentOS"),
			OperatingSystemVersion: makeStringPointer("7"),
			DisplayName:            makeStringPointer("BadlyFormatedDisplayName_IshouldBeIgnored"),
		},
	}
	fakeUbuntuID := "fakeUbuntu1"
	fakeBadID := "fake image id for bad image"

	compute.EXPECT().ListImages(context.Background(), &s.testCompartment).Return(listImageResponse, nil)
	compute.EXPECT().ListShapes(context.Background(), &s.testCompartment, &fakeUbuntuID).Return(listShapesResponse(), nil)
	compute.EXPECT().ListShapes(context.Background(), &s.testCompartment, &fakeBadID).Return(listShapesResponse(), nil)

	imgCache, err := oci.RefreshImageCache(compute, &s.testCompartment)
	c.Assert(err, gc.IsNil)
	c.Assert(imgCache, gc.NotNil)
	c.Check(imgCache.ImageMap(), gc.HasLen, 1)
	imageMap := imgCache.ImageMap()

	jammy := corebase.MakeDefaultBase("ubuntu", "22.04")
	c.Check(imageMap[jammy][0].Id, gc.Equals, "fakeUbuntu1")
}

func (s *imagesSuite) TestImageMetadataFromCache(c *gc.C) {
	image := ociCore.Image{
		CompartmentId:          &s.testCompartment,
		Id:                     &s.testImageID,
		OperatingSystem:        makeStringPointer("Canonical Ubuntu"),
		OperatingSystemVersion: makeStringPointer("22.04"),
		DisplayName:            makeStringPointer("Canonical-Ubuntu-22.04-2018.01.11-0"),
	}

	imgType, err := oci.NewInstanceImage(image, &s.testCompartment)
	c.Assert(err, gc.IsNil)

	cache := &oci.ImageCache{}
	jammy := corebase.MakeDefaultBase("ubuntu", "22.04")
	images := map[corebase.Base][]oci.InstanceImage{
		jammy: {
			imgType,
		},
	}
	cache.SetImages(images)
	metadata := cache.ImageMetadata(jammy, "")
	c.Assert(metadata, gc.HasLen, 1)
	// generic images default to ImageTypeVM
	c.Assert(metadata[0].VirtType, gc.Equals, string(oci.ImageTypeVM))

	// explicitly set ImageTypeBM on generic images
	metadata = cache.ImageMetadata(jammy, string(oci.ImageTypeBM))
	c.Assert(metadata, gc.HasLen, 1)
	c.Assert(metadata[0].VirtType, gc.Equals, string(oci.ImageTypeBM))
}

func (s *imagesSuite) TestImageMetadataSpecificImageType(c *gc.C) {
	image := ociCore.Image{
		CompartmentId:          &s.testCompartment,
		Id:                     &s.testImageID,
		OperatingSystem:        makeStringPointer("Canonical Ubuntu"),
		OperatingSystemVersion: makeStringPointer("22.04"),
		DisplayName:            makeStringPointer("Canonical-Ubuntu-22.04-Gen2-GPU-2018.01.11-0"),
	}

	imgType, err := oci.NewInstanceImage(image, &s.testCompartment)
	c.Assert(err, gc.IsNil)

	cache := &oci.ImageCache{}
	jammy := corebase.MakeDefaultBase("ubuntu", "22.04")
	images := map[corebase.Base][]oci.InstanceImage{
		jammy: {
			imgType,
		},
	}
	cache.SetImages(images)
	metadata := cache.ImageMetadata(jammy, "")
	c.Assert(metadata, gc.HasLen, 1)
	// generic images default to ImageTypeVM
	c.Assert(metadata[0].VirtType, gc.Equals, string(oci.ImageTypeGPU))

	// explicitly set ImageTypeBM on generic images
	metadata = cache.ImageMetadata(jammy, string(oci.ImageTypeBM))
	c.Assert(metadata, gc.HasLen, 1)
	c.Assert(metadata[0].VirtType, gc.Equals, string(oci.ImageTypeGPU))
}
