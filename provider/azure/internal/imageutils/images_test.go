// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imageutils_test

import (
	"net/http"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v2"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/arch"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/provider/azure/internal/azuretesting"
	"github.com/juju/juju/provider/azure/internal/imageutils"
	"github.com/juju/juju/testing"
)

type imageutilsSuite struct {
	testing.BaseSuite

	mockSender *azuretesting.MockSender
	client     *armcompute.VirtualMachineImagesClient
	callCtx    *context.CloudCallContext
}

var _ = gc.Suite(&imageutilsSuite{})

func (s *imageutilsSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.mockSender = &azuretesting.MockSender{}
	opts := &arm.ClientOptions{
		ClientOptions: policy.ClientOptions{
			Transport: s.mockSender,
		},
	}
	var err error
	s.client, err = armcompute.NewVirtualMachineImagesClient("subscription-id", &azuretesting.FakeCredential{}, opts)
	c.Assert(err, jc.ErrorIsNil)
	s.callCtx = context.NewEmptyCloudCallContext()
}

func (s *imageutilsSuite) TestBaseImageOldStyle(c *gc.C) {
	s.mockSender.AppendResponse(azuretesting.NewResponseWithContent(
		`[{"name": "20_04"}, {"name": "20_04-LTS"}, {"name": "19_04"}]`,
	))
	image, err := imageutils.BaseImage(s.callCtx, corebase.MakeDefaultBase("ubuntu", "20.04"), "released", "westus", s.client)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(image, gc.NotNil)
	c.Assert(image, jc.DeepEquals, &instances.Image{
		Id:       "Canonical:0001-com-ubuntu-server-focal:20_04-LTS:latest",
		Arch:     arch.AMD64,
		VirtType: "Hyper-V",
	})
}

func (s *imageutilsSuite) TestBaseImageOldStyleInvalidSKU(c *gc.C) {
	s.mockSender.AppendResponse(azuretesting.NewResponseWithContent(
		`[{"name": "22_04_invalid"}, {"name": "22_04_5-LTS"}]`,
	))
	image, err := imageutils.BaseImage(s.callCtx, corebase.MakeDefaultBase("ubuntu", "22.04"), "released", "westus", s.client)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(image, gc.NotNil)
	c.Assert(image, jc.DeepEquals, &instances.Image{
		Id:       "Canonical:0001-com-ubuntu-server-jammy:22_04_5-LTS:latest",
		Arch:     arch.AMD64,
		VirtType: "Hyper-V",
	})
}

func (s *imageutilsSuite) TestBaseImage(c *gc.C) {
	s.mockSender.AppendResponse(azuretesting.NewResponseWithContent(
		`[{"name": "server"}, {"name": "server-gen1"}, {"name": "server-arm64"}]`,
	))
	image, err := imageutils.BaseImage(s.callCtx, corebase.MakeDefaultBase("ubuntu", "24.04"), "released", "westus", s.client)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(image, gc.NotNil)
	c.Assert(image, jc.DeepEquals, &instances.Image{
		Id:       "Canonical:ubuntu-24_04-lts:server-gen1:latest",
		Arch:     arch.AMD64,
		VirtType: "Hyper-V",
	})
}

func (s *imageutilsSuite) TestBaseImageNonLTS(c *gc.C) {
	s.mockSender.AppendResponse(azuretesting.NewResponseWithContent(
		`[{"name": "server"}, {"name": "server-gen1"}, {"name": "server-arm64"}]`,
	))
	image, err := imageutils.BaseImage(s.callCtx, corebase.MakeDefaultBase("ubuntu", "25.04"), "released", "westus", s.client)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(image, gc.NotNil)
	c.Assert(image, jc.DeepEquals, &instances.Image{
		Id:       "Canonical:ubuntu-25_04:server-gen1:latest",
		Arch:     arch.AMD64,
		VirtType: "Hyper-V",
	})
}

func (s *imageutilsSuite) TestBaseImageCentOS(c *gc.C) {
	for _, cseries := range []string{"7", "8"} {
		base := corebase.MakeDefaultBase("centos", cseries)
		s.assertImageId(c, base, "released", "OpenLogic:CentOS:7.3:latest")
	}
}

func (s *imageutilsSuite) TestBaseImageStream(c *gc.C) {
	s.mockSender.AppendAndRepeatResponse(azuretesting.NewResponseWithContent(
		`[{"name": "22_04_2"}, {"name": "22_04_3-DAILY"}, {"name": "22_04_1-LTS"}]`), 2)
	base := corebase.MakeDefaultBase("ubuntu", "22.04")
	s.assertImageId(c, base, "daily", "Canonical:0001-com-ubuntu-server-jammy:22_04_3-DAILY:latest")
	s.assertImageId(c, base, "released", "Canonical:0001-com-ubuntu-server-jammy:22_04_2:latest")
}

func (s *imageutilsSuite) TestBaseImageOldStyleNotFound(c *gc.C) {
	s.mockSender.AppendResponse(azuretesting.NewResponseWithContent(`[]`))
	image, err := imageutils.BaseImage(s.callCtx, corebase.MakeDefaultBase("ubuntu", "22.04"), "released", "westus", s.client)
	c.Assert(err, gc.ErrorMatches, `selecting SKU for ubuntu@22.04: legacy ubuntu "jammy" SKUs for released stream not found`)
	c.Assert(image, gc.IsNil)
}

func (s *imageutilsSuite) TestBaseImageNotFound(c *gc.C) {
	s.mockSender.AppendResponse(azuretesting.NewResponseWithContent(`[]`))
	image, err := imageutils.BaseImage(s.callCtx, corebase.MakeDefaultBase("ubuntu", "24.04"), "released", "westus", s.client)
	c.Assert(err, gc.ErrorMatches, `selecting SKU for ubuntu@24.04: ubuntu "ubuntu@24.04/stable" SKUs for released stream not found`)
	c.Assert(image, gc.IsNil)
}

func (s *imageutilsSuite) TestBaseImageStreamNotFound(c *gc.C) {
	s.mockSender.AppendResponse(azuretesting.NewResponseWithContent(`[{"name": "22_04-beta1"}]`))
	_, err := imageutils.BaseImage(s.callCtx, corebase.MakeDefaultBase("ubuntu", "22.04"), "whatever", "westus", s.client)
	c.Assert(err, gc.ErrorMatches, `selecting SKU for ubuntu@22.04: legacy ubuntu "jammy" SKUs for whatever stream not found`)
}

func (s *imageutilsSuite) TestBaseImageStreamThrewCredentialError(c *gc.C) {
	s.mockSender.AppendResponse(azuretesting.NewResponseWithStatus("401 Unauthorized", http.StatusUnauthorized))
	called := false
	s.callCtx.InvalidateCredentialFunc = func(string) error {
		called = true
		return nil
	}

	_, err := imageutils.BaseImage(s.callCtx, corebase.MakeDefaultBase("ubuntu", "22.04"), "whatever", "westus", s.client)
	c.Assert(err.Error(), jc.Contains, "RESPONSE 401")
	c.Assert(called, jc.IsTrue)
}

func (s *imageutilsSuite) TestBaseImageStreamThrewNonCredentialError(c *gc.C) {
	s.mockSender.AppendResponse(azuretesting.NewResponseWithStatus("308 Permanent Redirect", http.StatusPermanentRedirect))
	called := false
	s.callCtx.InvalidateCredentialFunc = func(string) error {
		called = true
		return nil
	}

	_, err := imageutils.BaseImage(s.callCtx, corebase.MakeDefaultBase("ubuntu", "22.04"), "whatever", "westus", s.client)
	c.Assert(err.Error(), jc.Contains, "RESPONSE 308")
	c.Assert(called, jc.IsFalse)
}

func (s *imageutilsSuite) assertImageId(c *gc.C, base corebase.Base, stream, id string) {
	image, err := imageutils.BaseImage(s.callCtx, base, stream, "westus", s.client)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(image.Id, gc.Equals, id)
}
