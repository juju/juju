// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for info.

package cachedimages_test

import (
	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/cachedimages"
	"github.com/juju/juju/testing"
)

type deleteImageCommandSuite struct {
	testing.FakeJujuHomeSuite
	mockAPI *fakeImageDeleteAPI
}

var _ = gc.Suite(&deleteImageCommandSuite{})

type fakeImageDeleteAPI struct {
	kind   string
	series string
	arch   string
}

func (*fakeImageDeleteAPI) Close() error {
	return nil
}

func (f *fakeImageDeleteAPI) DeleteImage(kind, series, arch string) error {
	f.kind = kind
	f.series = series
	f.arch = arch
	return nil
}

func (s *deleteImageCommandSuite) SetUpTest(c *gc.C) {
	s.FakeJujuHomeSuite.SetUpTest(c)
	s.mockAPI = &fakeImageDeleteAPI{}
	s.PatchValue(cachedimages.GetDeleteImageAPI, func(c *cachedimages.DeleteCommand) (cachedimages.DeleteImageAPI, error) {
		return s.mockAPI, nil
	})
}

func runDeleteCommand(c *gc.C, args ...string) (*cmd.Context, error) {
	return testing.RunCommand(c, envcmd.Wrap(&cachedimages.DeleteCommand{}), args...)
}

func (s *deleteImageCommandSuite) TestDeleteImage(c *gc.C) {
	_, err := runDeleteCommand(c, "--kind", "lxc", "--series", "trusty", "--arch", "amd64")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.mockAPI.kind, gc.Equals, "lxc")
	c.Assert(s.mockAPI.series, gc.Equals, "trusty")
	c.Assert(s.mockAPI.arch, gc.Equals, "amd64")
}

func (*deleteImageCommandSuite) TestTooManyArgs(c *gc.C) {
	_, err := runDeleteCommand(c, "--kind", "lxc", "--series", "trusty", "--arch", "amd64", "bad")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["bad"\]`)
}

func (*deleteImageCommandSuite) TestKindRequired(c *gc.C) {
	_, err := runDeleteCommand(c, "--series", "trusty", "--arch", "amd64", "bad")
	c.Assert(err, gc.ErrorMatches, `image kind must be specified`)
}

func (*deleteImageCommandSuite) TestSeriesRequired(c *gc.C) {
	_, err := runDeleteCommand(c, "--kind", "lxc", "--arch", "amd64", "bad")
	c.Assert(err, gc.ErrorMatches, `image series must be specified`)
}

func (*deleteImageCommandSuite) TestArchRequired(c *gc.C) {
	_, err := runDeleteCommand(c, "--kind", "lxc", "--series", "trusty", "bad")
	c.Assert(err, gc.ErrorMatches, `image architecture must be specified`)
}
