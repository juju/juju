// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for info.

package cachedimages_test

import (
	"time"

	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/cachedimages"
	"github.com/juju/juju/testing"
)

type listImagesCommandSuite struct {
	testing.FakeJujuHomeSuite
	mockAPI *fakeImagesListAPI
}

var _ = gc.Suite(&listImagesCommandSuite{})

type fakeImagesListAPI struct{}

func (*fakeImagesListAPI) Close() error {
	return nil
}

func (f *fakeImagesListAPI) ListImages(kind, series, arch string) ([]params.ImageMetadata, error) {
	if kind != "lxc" {
		return nil, nil
	}
	result := []params.ImageMetadata{
		{
			Kind:    kind,
			Series:  series,
			Arch:    arch,
			URL:     "http://image",
			Created: time.Date(2015, 1, 1, 0, 0, 0, 0, time.UTC),
		},
	}
	return result, nil
}

func (s *listImagesCommandSuite) SetUpTest(c *gc.C) {
	s.FakeJujuHomeSuite.SetUpTest(c)
	s.mockAPI = &fakeImagesListAPI{}
	s.PatchValue(cachedimages.GetListImagesAPI, func(c *cachedimages.ListCommand) (cachedimages.ListImagesAPI, error) {
		return s.mockAPI, nil
	})
}

func runListCommand(c *gc.C, args ...string) (*cmd.Context, error) {
	return testing.RunCommand(c, envcmd.Wrap(&cachedimages.ListCommand{}), args...)
}

func (*listImagesCommandSuite) TestListImagesNone(c *gc.C) {
	context, err := runListCommand(c, "--kind", "kvm")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(context), gc.Equals, "no matching images found\n")
}

func (*listImagesCommandSuite) TestListImagesFormatJson(c *gc.C) {
	context, err := runListCommand(c, "--format", "json", "--kind", "lxc", "--series", "trusty", "--arch", "amd64")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(context), gc.Equals, "Cached images:\n["+
		`{"kind":"lxc","series":"trusty","arch":"amd64","source-url":"http://image","created":"Thu, 01 Jan 2015 00:00:00 UTC"}`+
		"]\n")
}

func (*listImagesCommandSuite) TestListImagesFormatYaml(c *gc.C) {
	context, err := runListCommand(c, "--format", "yaml", "--kind", "lxc", "--series", "trusty", "--arch", "amd64")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(context), gc.Equals, "Cached images:\n"+
		"- kind: lxc\n"+
		"  series: trusty\n"+
		"  arch: amd64\n"+
		"  source-url: http://image\n"+
		"  created: Thu, 01 Jan 2015 00:00:00 UTC\n")
}

func (*listImagesCommandSuite) TestTooManyArgs(c *gc.C) {
	_, err := runListCommand(c, "bad")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["bad"\]`)
}
