// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oci_test

import (
	"context"
	"time"

	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"
	ociCore "github.com/oracle/oci-go-sdk/v65/core"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/arch"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/internal/provider/oci"
	ocitesting "github.com/juju/juju/internal/provider/oci/testing"
	jujutesting "github.com/juju/juju/internal/testing"
)

type imagesSuite struct {
	jujutesting.BaseSuite

	testImageID     string
	testCompartment string
}

var _ = tc.Suite(&imagesSuite{})

func (s *imagesSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
	oci.SetImageCache(&oci.ImageCache{})

	s.testImageID = "ocid1.image.oc1.phx.aaaaaaaaa5mikpf5fktj4x47bx4p4ak4g5jyuyukkxpdg4nll36qzqwjzd2q"
	s.testCompartment = "ocid1.compartment.oc1..aaaaaaaaakr75vvb5yx4nkm7ag7ekvluap7afa2y4zprswuprcnehqecwqga"
}

func (s *imagesSuite) TestNewImageVersion(c *tc.C) {
	name := "Canonical-Ubuntu-22.04-2017.08.22-0"
	img := ociCore.Image{
		DisplayName: &name,
	}
	timeStamp, _ := time.Parse("2006.01.02", "2017.08.22")
	version, err := oci.NewImageVersion(img)
	c.Assert(err, tc.IsNil)
	c.Assert(version.TimeStamp, tc.Equals, timeStamp)
	c.Assert(version.Revision, tc.Equals, 0)
}

func (s *imagesSuite) TestNewImageVersionInvalidDate(c *tc.C) {
	name := "Canonical-Ubuntu-22.04-NotARealDate-0"
	img := ociCore.Image{
		DisplayName: &name,
	}
	_, err := oci.NewImageVersion(img)
	c.Assert(err, tc.ErrorMatches, "parsing time for.*")
}

func (s *imagesSuite) TestNewImageVersionInvalidRevision(c *tc.C) {
	name := "Canonical-Ubuntu-22.04-2017.08.22-IShouldBeNumeric"
	img := ociCore.Image{
		DisplayName: &name,
	}
	_, err := oci.NewImageVersion(img)
	c.Assert(err, tc.ErrorMatches, "parsing revision for.*")
}

func (s *imagesSuite) TestNewImageVersionInvalidName(c *tc.C) {
	name := "fakeInvalidName"
	img := ociCore.Image{
		DisplayName: &name,
	}
	_, err := oci.NewImageVersion(img)
	c.Assert(err, tc.ErrorMatches, "invalid image display name.*")

	img = ociCore.Image{}
	_, err = oci.NewImageVersion(img)
	c.Assert(err, tc.ErrorMatches, "image does not have a display name")
}

func makeStringPointer(name string) *string {
	return &name
}

func makeIntPointer(name int) *int {
	return &name
}

func makeUint64Pointer(name uint64) *uint64 {
	return &name
}

func makeBoolPointer(name bool) *bool {
	return &name
}
func makeFloat32Pointer(name float32) *float32 {
	return &name
}

func (s *imagesSuite) TestInstanceTypes(c *tc.C) {
	ctrl := gomock.NewController(c)
	compute := ocitesting.NewMockComputeClient(ctrl)
	defer ctrl.Finish()

	compute.EXPECT().ListShapes(context.Background(), &s.testCompartment, &s.testImageID).Return(listShapesResponse(), nil)

	types, err := oci.InstanceTypes(compute, &s.testCompartment, &s.testImageID)
	c.Assert(err, tc.IsNil)
	c.Check(types, tc.HasLen, 5)
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
			Name:        "VM.Standard.A1.Flex",
			Arch:        arch.ARM64,
			Mem:         6 * 1024,
			MaxCpuCores: makeUint64Pointer(80),
			MaxMem:      makeUint64Pointer(512 * 1024),
			CpuCores:    1,
			VirtType:    makeStringPointer("vm"),
		}, {
			Name:        "VM.Standard3.Flex",
			Arch:        arch.AMD64,
			Mem:         6 * 1024,
			MaxCpuCores: makeUint64Pointer(32),
			MaxMem:      makeUint64Pointer(512 * 1024),
			CpuCores:    1,
			VirtType:    makeStringPointer("vm"),
		},
	}
	c.Assert(types, tc.DeepEquals, expectedTypes)
}

func (s *imagesSuite) TestInstanceTypesNilClient(c *tc.C) {
	_, err := oci.InstanceTypes(nil, &s.testCompartment, &s.testImageID)
	c.Assert(err, tc.ErrorMatches, "cannot use nil client")
}

func (s *imagesSuite) TestNewInstanceImageUbuntu(c *tc.C) {
	image := ociCore.Image{
		CompartmentId:          &s.testCompartment,
		Id:                     &s.testImageID,
		OperatingSystem:        makeStringPointer("Canonical Ubuntu"),
		OperatingSystemVersion: makeStringPointer("22.04"),
		DisplayName:            makeStringPointer("Canonical-Ubuntu-22.04-2018.01.11-0"),
	}

	imgType, a, err := oci.NewInstanceImage(image, &s.testCompartment)
	c.Assert(err, tc.IsNil)
	c.Check(imgType.ImageType, tc.Equals, oci.ImageTypeGeneric)
	c.Check(imgType.Base.DisplayString(), tc.Equals, "ubuntu@22.04")
	c.Check(imgType.CompartmentId, tc.NotNil)
	c.Check(*imgType.CompartmentId, tc.Equals, s.testCompartment)
	c.Check(imgType.Id, tc.Equals, s.testImageID)
	c.Check(a, tc.Equals, arch.AMD64)
}

// TestNewInstanceImageUbuntuMinimalNotSupported is testing that if an image
// passed to the parser is of type minimal we result in a not supported error.
func (s *imagesSuite) TestNewInstanceImageUbuntuMinimalNotSupported(c *tc.C) {
	tests := []struct {
		Name  string
		Image ociCore.Image
	}{
		{
			Name: "Test minimal image for amd64 in OperatingSystem",
			Image: ociCore.Image{
				CompartmentId:          &s.testCompartment,
				Id:                     &s.testImageID,
				OperatingSystem:        makeStringPointer("Canonical Ubuntu"),
				OperatingSystemVersion: makeStringPointer("22.04 Minimal"),
				DisplayName:            makeStringPointer("Canonical-Ubuntu-22.04-2018.01.11-0"),
			},
		},
		{
			Name: "Test minimal image for amd64 in DisplayName",
			Image: ociCore.Image{
				CompartmentId:          &s.testCompartment,
				Id:                     &s.testImageID,
				OperatingSystem:        makeStringPointer("Canonical Ubuntu"),
				OperatingSystemVersion: makeStringPointer("22.04"),
				DisplayName:            makeStringPointer("Canonical-Ubuntu-22.04-Minimal-2018.01.11-0"),
			},
		},
		{
			Name: "Test minimal image for amd64 in OperatingSystem",
			Image: ociCore.Image{
				CompartmentId:          &s.testCompartment,
				Id:                     &s.testImageID,
				OperatingSystem:        makeStringPointer("Canonical Ubuntu"),
				OperatingSystemVersion: makeStringPointer("22.04 Minimal aarch64"),
				DisplayName:            makeStringPointer("Canonical-Ubuntu-22.04-aarch64-2018.01.11-0"),
			},
		},
		{
			Name: "Test minimal image for amd64 in DisplayName",
			Image: ociCore.Image{
				CompartmentId:          &s.testCompartment,
				Id:                     &s.testImageID,
				OperatingSystem:        makeStringPointer("Canonical Ubuntu"),
				OperatingSystemVersion: makeStringPointer("22.04 aarch64"),
				DisplayName:            makeStringPointer("Canonical-Ubuntu-22.04-Minimal-aarch64-2018.01.11-0"),
			},
		},
	}

	for _, test := range tests {
		img, _, err := oci.NewInstanceImage(test.Image, &s.testCompartment)
		c.Assert(err, jc.ErrorIsNil)
		c.Check(img.IsMinimal, jc.IsTrue)
	}
}

func (s *imagesSuite) TestNewInstanceImageUbuntuAARCH64(c *tc.C) {
	image := ociCore.Image{
		CompartmentId:          &s.testCompartment,
		Id:                     &s.testImageID,
		OperatingSystem:        makeStringPointer("Canonical Ubuntu"),
		OperatingSystemVersion: makeStringPointer("22.04 aarch64"),
		DisplayName:            makeStringPointer("Canonical-Ubuntu-22.04-aarch64-2018.01.11-0"),
	}

	imgType, a, err := oci.NewInstanceImage(image, &s.testCompartment)
	c.Assert(err, tc.IsNil)
	c.Check(imgType.ImageType, tc.Equals, oci.ImageTypeGeneric)
	c.Check(imgType.Base.DisplayString(), tc.Equals, "ubuntu@22.04")
	c.Check(imgType.CompartmentId, tc.NotNil)
	c.Check(*imgType.CompartmentId, tc.Equals, s.testCompartment)
	c.Check(imgType.Id, tc.Equals, s.testImageID)
	c.Check(a, tc.Equals, arch.ARM64)
}

func (s *imagesSuite) TestNewInstanceImageUbuntuAARCH64OnDisplayName(c *tc.C) {
	image := ociCore.Image{
		CompartmentId:          &s.testCompartment,
		Id:                     &s.testImageID,
		OperatingSystem:        makeStringPointer("Canonical Ubuntu"),
		OperatingSystemVersion: makeStringPointer("22.04"),
		DisplayName:            makeStringPointer("Canonical-Ubuntu-22.04-aarch64-2018.01.11-0"),
	}

	imgType, a, err := oci.NewInstanceImage(image, &s.testCompartment)
	c.Assert(err, tc.IsNil)
	c.Check(imgType.ImageType, tc.Equals, oci.ImageTypeGeneric)
	c.Check(imgType.Base.DisplayString(), tc.Equals, "ubuntu@22.04")
	c.Check(imgType.CompartmentId, tc.NotNil)
	c.Check(*imgType.CompartmentId, tc.Equals, s.testCompartment)
	c.Check(imgType.Id, tc.Equals, s.testImageID)
	c.Check(a, tc.Equals, arch.ARM64)
}

func (s *imagesSuite) TestNewInstanceImageUnknownOS(c *tc.C) {
	image := ociCore.Image{
		CompartmentId:          &s.testCompartment,
		Id:                     &s.testImageID,
		OperatingSystem:        makeStringPointer("NotKnownToJuju"),
		OperatingSystemVersion: makeStringPointer("22.04"),
		DisplayName:            makeStringPointer("Canonical-Ubuntu-22.04-2018.01.11-0"),
	}

	_, _, err := oci.NewInstanceImage(image, &s.testCompartment)
	c.Assert(err, tc.ErrorMatches, "os NotKnownToJuju not supported")
}

func (s *imagesSuite) TestRefreshImageCache(c *tc.C) {
	ctrl := gomock.NewController(c)
	compute := ocitesting.NewMockComputeClient(ctrl)
	defer ctrl.Finish()

	fakeUbuntu1 := "fakeUbuntu1"
	fakeUbuntu2 := "fakeUbuntu2"
	fakeUbuntu3 := "fakeUbuntu3"
	fakeUbuntu4 := "fakeUbuntu4"
	fakeUbuntuMinimal0 := "fakeUbuntuMinimal0"
	fakeUbuntuMinimal1 := "fakeUbuntuMinimal1"

	listImageResponse := []ociCore.Image{
		{
			CompartmentId:          &s.testCompartment,
			Id:                     &fakeUbuntu1,
			OperatingSystem:        makeStringPointer("Canonical Ubuntu"),
			OperatingSystemVersion: makeStringPointer("22.04"),
			DisplayName:            makeStringPointer("Canonical-Ubuntu-22.04-2018.01.11-0"),
		},
		{
			CompartmentId:          &s.testCompartment,
			Id:                     &fakeUbuntu2,
			OperatingSystem:        makeStringPointer("Canonical Ubuntu"),
			OperatingSystemVersion: makeStringPointer("22.04"),
			DisplayName:            makeStringPointer("Canonical-Ubuntu-22.04-2018.01.12-0"),
		},
		{
			CompartmentId:          &s.testCompartment,
			Id:                     &fakeUbuntuMinimal0,
			OperatingSystem:        makeStringPointer("Canonical Ubuntu"),
			OperatingSystemVersion: makeStringPointer("22.04 Minimal"),
			DisplayName:            makeStringPointer("Canonical-Ubuntu-22.04-2018.01.12-0"),
		},
		{
			CompartmentId:          &s.testCompartment,
			Id:                     &fakeUbuntuMinimal1,
			OperatingSystem:        makeStringPointer("Canonical Ubuntu"),
			OperatingSystemVersion: makeStringPointer("22.04"),
			DisplayName:            makeStringPointer("Canonical-Ubuntu-22.04-Minimal-2018.01.12-0"),
		},
		{
			CompartmentId:          &s.testCompartment,
			Id:                     &fakeUbuntu3,
			OperatingSystem:        makeStringPointer("Canonical Ubuntu"),
			OperatingSystemVersion: makeStringPointer("22.04 aarch64"),
			DisplayName:            makeStringPointer("Canonical-Ubuntu-22.04-2018.01.11-0"),
		},
		{
			CompartmentId:          &s.testCompartment,
			Id:                     &fakeUbuntu4,
			OperatingSystem:        makeStringPointer("Canonical Ubuntu"),
			OperatingSystemVersion: makeStringPointer("22.04"),
			DisplayName:            makeStringPointer("Canonical-Ubuntu-22.04-aarch64-2018.01.12-0"),
		},
	}

	compute.EXPECT().ListImages(context.Background(), &s.testCompartment).Return(listImageResponse, nil)
	compute.EXPECT().ListShapes(context.Background(), &s.testCompartment, &fakeUbuntu1).Return(listShapesResponse(), nil)
	compute.EXPECT().ListShapes(context.Background(), &s.testCompartment, &fakeUbuntu2).Return(listShapesResponse(), nil)
	compute.EXPECT().ListShapes(context.Background(), &s.testCompartment, &fakeUbuntu3).Return(listShapesResponse(), nil)
	compute.EXPECT().ListShapes(context.Background(), &s.testCompartment, &fakeUbuntu4).Return(listShapesResponse(), nil)

	imgCache, err := oci.RefreshImageCache(context.Background(), compute, &s.testCompartment)
	c.Assert(err, tc.IsNil)
	c.Assert(imgCache, tc.NotNil)
	c.Check(imgCache.ImageMap(), tc.HasLen, 1)

	imageMap := imgCache.ImageMap()
	jammy := corebase.MakeDefaultBase("ubuntu", "22.04")
	// Both archs AMD64 and ARM64 should be on the base jammy and minimal
	// ubuntu should be ignored.
	c.Check(imageMap[jammy], tc.HasLen, 2)
	// Two images on each arch
	c.Check(imageMap[jammy][arch.AMD64], tc.HasLen, 2)
	c.Check(imageMap[jammy][arch.ARM64], tc.HasLen, 2)

	timeStamp, _ := time.Parse("2006.01.02", "2018.01.12")

	// Check that the first image in the array is the newest one
	c.Assert(imageMap[jammy][arch.AMD64][0].Version.TimeStamp, tc.Equals, timeStamp)
	c.Assert(imageMap[jammy][arch.ARM64][0].Version.TimeStamp, tc.Equals, timeStamp)

	// Check that InstanceTypes are set
	c.Assert(imageMap[jammy][arch.AMD64][0].InstanceTypes, tc.HasLen, 5)
	c.Assert(imageMap[jammy][arch.ARM64][0].InstanceTypes, tc.HasLen, 5)
}

func (s *imagesSuite) TestRefreshImageCacheFetchFromCache(c *tc.C) {
	ctrl := gomock.NewController(c)
	compute := ocitesting.NewMockComputeClient(ctrl)
	defer ctrl.Finish()

	compute.EXPECT().ListImages(gomock.Any(), gomock.Any()).Return([]ociCore.Image{}, nil)

	imgCache, err := oci.RefreshImageCache(context.Background(), compute, &s.testCompartment)
	c.Assert(err, tc.IsNil)
	c.Assert(imgCache, tc.NotNil)

	fromCache, err := oci.RefreshImageCache(context.Background(), compute, &s.testCompartment)
	c.Assert(err, tc.IsNil)
	c.Check(imgCache, tc.DeepEquals, fromCache)
}

func (s *imagesSuite) TestRefreshImageCacheStaleCache(c *tc.C) {
	ctrl := gomock.NewController(c)
	compute := ocitesting.NewMockComputeClient(ctrl)
	defer ctrl.Finish()

	compute.EXPECT().ListImages(gomock.Any(), gomock.Any()).Return([]ociCore.Image{}, nil).Times(2)

	imgCache, err := oci.RefreshImageCache(context.Background(), compute, &s.testCompartment)
	c.Assert(err, tc.IsNil)
	c.Assert(imgCache, tc.NotNil)

	now := time.Now()

	// No need to check the value. gomock will assert if ListImages
	// is not called twice
	imgCache.SetLastRefresh(now.Add(-31 * time.Minute))
	_, err = oci.RefreshImageCache(context.Background(), compute, &s.testCompartment)
	c.Assert(err, tc.IsNil)
}

func (s *imagesSuite) TestRefreshImageCacheWithInvalidImage(c *tc.C) {
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

	compute.EXPECT().ListImages(context.Background(), &s.testCompartment).Return(listImageResponse, nil)
	// Only list shapes from "fakeUbuntu1" image, because the other one
	// is invalid.
	compute.EXPECT().ListShapes(context.Background(), &s.testCompartment, &fakeUbuntuID).Return(listShapesResponse(), nil)

	imgCache, err := oci.RefreshImageCache(context.Background(), compute, &s.testCompartment)
	c.Assert(err, tc.IsNil)
	c.Assert(imgCache, tc.NotNil)
	c.Check(imgCache.ImageMap(), tc.HasLen, 1)
	imageMap := imgCache.ImageMap()

	jammy := corebase.MakeDefaultBase("ubuntu", "22.04")
	c.Check(imageMap[jammy][arch.AMD64][0].Id, tc.Equals, "fakeUbuntu1")
}

func (s *imagesSuite) TestImageMetadataFromCache(c *tc.C) {
	image := ociCore.Image{
		CompartmentId:          &s.testCompartment,
		Id:                     &s.testImageID,
		OperatingSystem:        makeStringPointer("Canonical Ubuntu"),
		OperatingSystemVersion: makeStringPointer("22.04"),
		DisplayName:            makeStringPointer("Canonical-Ubuntu-22.04-2018.01.11-0"),
	}

	imgType, a, err := oci.NewInstanceImage(image, &s.testCompartment)
	c.Assert(err, tc.IsNil)
	instanceTypes := []instances.InstanceType{
		{
			Arch: "amd64",
		},
	}
	imgType.SetInstanceTypes(instanceTypes)

	cache := &oci.ImageCache{}
	jammy := corebase.MakeDefaultBase("ubuntu", "22.04")
	images := map[corebase.Base]map[string][]oci.InstanceImage{
		jammy: {
			arch.AMD64: {
				imgType,
			},
		},
	}
	cache.SetImages(images)
	metadata := cache.ImageMetadata(jammy, a, "")
	c.Assert(metadata, tc.HasLen, 1)
	// generic images default to ImageTypeVM
	c.Assert(metadata[0].VirtType, tc.Equals, string(oci.ImageTypeVM))

	// explicitly set ImageTypeBM on generic images
	metadata = cache.ImageMetadata(jammy, a, string(oci.ImageTypeBM))
	c.Assert(metadata, tc.HasLen, 1)
	c.Assert(metadata[0].VirtType, tc.Equals, string(oci.ImageTypeBM))
}

func (s *imagesSuite) TestImageMetadataSpecificImageType(c *tc.C) {
	image := ociCore.Image{
		CompartmentId:          &s.testCompartment,
		Id:                     &s.testImageID,
		OperatingSystem:        makeStringPointer("Canonical Ubuntu"),
		OperatingSystemVersion: makeStringPointer("22.04"),
		DisplayName:            makeStringPointer("Canonical-Ubuntu-22.04-Gen2-GPU-2018.01.11-0"),
	}

	imgType, a, err := oci.NewInstanceImage(image, &s.testCompartment)
	c.Assert(err, tc.IsNil)
	instanceTypes := []instances.InstanceType{
		{
			Arch: "amd64",
		},
	}
	imgType.SetInstanceTypes(instanceTypes)

	cache := &oci.ImageCache{}
	jammy := corebase.MakeDefaultBase("ubuntu", "22.04")
	images := map[corebase.Base]map[string][]oci.InstanceImage{
		jammy: {
			arch.AMD64: {
				imgType,
			},
		},
	}
	cache.SetImages(images)
	metadata := cache.ImageMetadata(jammy, a, "")
	c.Assert(metadata, tc.HasLen, 1)
	// generic images default to ImageTypeVM
	c.Assert(metadata[0].VirtType, tc.Equals, string(oci.ImageTypeGPU))

	// explicitly set ImageTypeBM on generic images
	metadata = cache.ImageMetadata(jammy, a, string(oci.ImageTypeBM))
	c.Assert(metadata, tc.HasLen, 1)
	c.Assert(metadata[0].VirtType, tc.Equals, string(oci.ImageTypeGPU))
}
