// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce_test

import (
	"cloud.google.com/go/compute/apiv1/computepb"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/internal/provider/gce"
	coretools "github.com/juju/juju/tools"
)

type imageSuite struct {
	gce.BaseSuite
}

var _ = gc.Suite(&imageSuite{})

func (s *imageSuite) TestConvertImageArchARM64(c *gc.C) {
	arch, err := gce.ConvertImageArch("ARM64")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(arch, gc.Equals, "arm64")
}

func (s *imageSuite) TestConvertImageArchX86_64(c *gc.C) {
	arch, err := gce.ConvertImageArch("X86_64")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(arch, gc.Equals, "amd64")
}

func (s *imageSuite) TestConvertImageArchX86_64LowerCase(c *gc.C) {
	arch, err := gce.ConvertImageArch("x86_64")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(arch, gc.Equals, "amd64")
}

func (s *imageSuite) TestConvertImageArchMixedCase(c *gc.C) {
	arch, err := gce.ConvertImageArch("Arm64")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(arch, gc.Equals, "arm64")
}

func (s *imageSuite) TestConvertImageArchUnsupported(c *gc.C) {
	_, err := gce.ConvertImageArch("ppc64")
	c.Assert(err, gc.NotNil)
	c.Assert(err, gc.ErrorMatches, "unsupported image architecture.*")
}

func (s *imageSuite) TestConvertImageArchEmpty(c *gc.C) {
	_, err := gce.ConvertImageArch("")
	c.Assert(err, gc.NotNil)
	c.Assert(err, gc.ErrorMatches, "unsupported image architecture.*")
}

func (s *imageSuite) TestSplitProjectImagePathWithFullPath(c *gc.C) {
	project, imageName, ok := gce.SplitProjectImagePath("projects/my-project/global/images/my-image")
	c.Assert(ok, jc.IsTrue)
	c.Assert(project, gc.Equals, "my-project")
	c.Assert(imageName, gc.Equals, "my-image")
}

func (s *imageSuite) TestSplitProjectImagePathWithLeadingSlash(c *gc.C) {
	project, imageName, ok := gce.SplitProjectImagePath("/projects/my-project/global/images/my-image")
	c.Assert(ok, jc.IsTrue)
	c.Assert(project, gc.Equals, "my-project")
	c.Assert(imageName, gc.Equals, "my-image")
}

func (s *imageSuite) TestSplitProjectImagePathWithURL(c *gc.C) {
	project, imageName, ok := gce.SplitProjectImagePath(
		"https://www.googleapis.com/compute/v1/projects/my-project/global/images/my-image",
	)
	c.Assert(ok, jc.IsTrue)
	c.Assert(project, gc.Equals, "my-project")
	c.Assert(imageName, gc.Equals, "my-image")
}

func (s *imageSuite) TestSplitProjectImagePathWithoutImageName(c *gc.C) {
	project, imageName, ok := gce.SplitProjectImagePath("projects/my-project/global/images/")
	c.Assert(ok, jc.IsTrue)
	c.Assert(project, gc.Equals, "my-project")
	c.Assert(imageName, gc.Equals, "")
}

func (s *imageSuite) TestSplitProjectImagePathInvalidFormat(c *gc.C) {
	_, _, ok := gce.SplitProjectImagePath("invalid/path")
	c.Assert(ok, jc.IsFalse)
}

func (s *imageSuite) TestSplitProjectImagePathEmpty(c *gc.C) {
	_, _, ok := gce.SplitProjectImagePath("")
	c.Assert(ok, jc.IsFalse)
}

func (s *imageSuite) TestSplitProjectImagePathNoProject(c *gc.C) {
	_, _, ok := gce.SplitProjectImagePath("projects//global/images/my-image")
	c.Assert(ok, jc.IsFalse)
}

func (s *imageSuite) TestResolveImageIDMetadata(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	imageID := "projects/ubuntu-os-cloud/global/images/ubuntu-2204-lts"
	imageName := "ubuntu-2204-lts"
	project := "ubuntu-os-cloud"

	s.MockService.EXPECT().ImageByProject(gomock.Any(), project, imageName).Return(&computepb.Image{
		Architecture: new("X86_64"),
	}, nil)

	args := s.StartInstArgs
	args.Constraints = constraints.Value{ImageID: &imageID}
	args.Tools = []*coretools.Tools{{
		Version: version.Binary{Arch: arch.AMD64, Release: "ubuntu"},
		URL:     "https://example.org",
	}}

	metadata, err := gce.ResolveImageIDMetadata(env, s.CallCtx, args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metadata, gc.HasLen, 1)
	c.Assert(metadata[0].Id, gc.Equals, imageID)
	c.Assert(metadata[0].Arch, gc.Equals, arch.AMD64)
}

func (s *imageSuite) TestResolveImageIDMetadataArchMismatch(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	imageID := "projects/ubuntu-os-cloud/global/images/ubuntu-2204-arm64"
	imageName := "ubuntu-2204-arm64"
	project := "ubuntu-os-cloud"

	s.MockService.EXPECT().ImageByProject(gomock.Any(), project, imageName).Return(&computepb.Image{
		Architecture: new("ARM64"),
	}, nil)

	args := s.StartInstArgs
	args.Constraints = constraints.Value{ImageID: &imageID}
	args.Tools = []*coretools.Tools{{
		Version: version.Binary{Arch: arch.AMD64, Release: "ubuntu"},
		URL:     "https://example.org",
	}}

	_, err := gce.ResolveImageIDMetadata(env, s.CallCtx, args)
	c.Assert(err, gc.NotNil)
	c.Assert(err, gc.ErrorMatches, ".*has architecture.*requires.*")
}

func (s *imageSuite) TestParseImageIDReferenceWithFullPath(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	project, imageName, err := gce.ParseImageIDReference(env, "ubuntu", "projects/my-project/global/images/my-image")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(project, gc.Equals, "my-project")
	c.Assert(imageName, gc.Equals, "my-image")
}

func (s *imageSuite) TestImageURL(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	url, err := gce.ImageURL(env, "ubuntu", "ubuntu-2204-jammy-v20260803")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(url, gc.Equals, "projects/ubuntu-os-cloud/global/images/ubuntu-2204-jammy-v20260803")

	url, err = gce.ImageURL(env, "ubuntu", "projects/custom-images/global/images/custom-ubuntu")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(url, gc.Equals, "projects/custom-images/global/images/custom-ubuntu")
}
