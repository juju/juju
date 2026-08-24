// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce_test

import (
	"testing"

	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/canonical/gomock/gomock"
	"github.com/juju/tc"

	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/internal/provider/gce"
	coretools "github.com/juju/juju/internal/tools"
)

type imageSuite struct {
	gce.BaseSuite
}

func TestImageSuite(t *testing.T) {
	tc.Run(t, &imageSuite{})
}

func (s *imageSuite) TestConvertImageArchARM64(c *tc.C) {
	arch, err := gce.ConvertImageArch("ARM64")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(arch, tc.Equals, "arm64")
}

func (s *imageSuite) TestConvertImageArchX86_64(c *tc.C) {
	arch, err := gce.ConvertImageArch("X86_64")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(arch, tc.Equals, "amd64")
}

func (s *imageSuite) TestConvertImageArchX86_64LowerCase(c *tc.C) {
	arch, err := gce.ConvertImageArch("x86_64")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(arch, tc.Equals, "amd64")
}

func (s *imageSuite) TestConvertImageArchMixedCase(c *tc.C) {
	arch, err := gce.ConvertImageArch("Arm64")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(arch, tc.Equals, "arm64")
}

func (s *imageSuite) TestConvertImageArchUnsupported(c *tc.C) {
	_, err := gce.ConvertImageArch("ppc64")
	c.Assert(err, tc.NotNil)
	c.Assert(err, tc.ErrorMatches, "unsupported image architecture.*")
}

func (s *imageSuite) TestConvertImageArchEmpty(c *tc.C) {
	_, err := gce.ConvertImageArch("")
	c.Assert(err, tc.NotNil)
	c.Assert(err, tc.ErrorMatches, "unsupported image architecture.*")
}

func (s *imageSuite) TestSplitProjectImagePathWithFullPath(c *tc.C) {
	project, imageName, ok := gce.SplitProjectImagePath("projects/my-project/global/images/my-image")
	c.Assert(ok, tc.IsTrue)
	c.Assert(project, tc.Equals, "my-project")
	c.Assert(imageName, tc.Equals, "my-image")
}

func (s *imageSuite) TestSplitProjectImagePathWithLeadingSlash(c *tc.C) {
	project, imageName, ok := gce.SplitProjectImagePath("/projects/my-project/global/images/my-image")
	c.Assert(ok, tc.IsTrue)
	c.Assert(project, tc.Equals, "my-project")
	c.Assert(imageName, tc.Equals, "my-image")
}

func (s *imageSuite) TestSplitProjectImagePathWithURL(c *tc.C) {
	project, imageName, ok := gce.SplitProjectImagePath(
		"https://www.googleapis.com/compute/v1/projects/my-project/global/images/my-image",
	)
	c.Assert(ok, tc.IsTrue)
	c.Assert(project, tc.Equals, "my-project")
	c.Assert(imageName, tc.Equals, "my-image")
}

func (s *imageSuite) TestSplitProjectImagePathWithoutImageName(c *tc.C) {
	project, imageName, ok := gce.SplitProjectImagePath("projects/my-project/global/images/")
	c.Assert(ok, tc.IsTrue)
	c.Assert(project, tc.Equals, "my-project")
	c.Assert(imageName, tc.Equals, "")
}

func (s *imageSuite) TestSplitProjectImagePathInvalidFormat(c *tc.C) {
	_, _, ok := gce.SplitProjectImagePath("invalid/path")
	c.Assert(ok, tc.IsFalse)
}

func (s *imageSuite) TestSplitProjectImagePathEmpty(c *tc.C) {
	_, _, ok := gce.SplitProjectImagePath("")
	c.Assert(ok, tc.IsFalse)
}

func (s *imageSuite) TestSplitProjectImagePathNoProject(c *tc.C) {
	_, _, ok := gce.SplitProjectImagePath("projects//global/images/my-image")
	c.Assert(ok, tc.IsFalse)
}

func (s *imageSuite) TestResolveImageIDMetadata(c *tc.C) {
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
		Version: semversion.Binary{Arch: arch.AMD64, Release: "ubuntu"},
		URL:     "https://example.org",
	}}

	metadata, err := gce.ResolveImageIDMetadata(env, c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(metadata, tc.HasLen, 1)
	c.Assert(metadata[0].Id, tc.Equals, imageID)
	c.Assert(metadata[0].Arch, tc.Equals, arch.AMD64)
}

func (s *imageSuite) TestResolveImageIDMetadataArchMismatch(c *tc.C) {
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
		Version: semversion.Binary{Arch: arch.AMD64, Release: "ubuntu"},
		URL:     "https://example.org",
	}}

	_, err := gce.ResolveImageIDMetadata(env, c.Context(), args)
	c.Assert(err, tc.NotNil)
	c.Assert(err, tc.ErrorMatches, ".*has architecture.*requires.*")
}

func (s *imageSuite) TestParseImageIDReferenceWithFullPath(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	project, imageName, err := gce.ParseImageIDReference(env, "ubuntu", "projects/my-project/global/images/my-image")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(project, tc.Equals, "my-project")
	c.Assert(imageName, tc.Equals, "my-image")
}

func (s *imageSuite) TestImageURL(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	url, err := gce.ImageURL(env, "ubuntu", "ubuntu-2204-jammy-v20260803")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(url, tc.Equals, "projects/ubuntu-os-cloud/global/images/ubuntu-2204-jammy-v20260803")

	url, err = gce.ImageURL(env, "ubuntu", "projects/custom-images/global/images/custom-ubuntu")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(url, tc.Equals, "projects/custom-images/global/images/custom-ubuntu")
}
