// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for info.

package cachedimages_test

import (
	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/cachedimages"
	"github.com/juju/juju/testing"
)

type removeImageCommandSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	mockAPI *fakeImageRemoveAPI
}

var _ = gc.Suite(&removeImageCommandSuite{})

type fakeImageRemoveAPI struct {
	kind   string
	series string
	arch   string
}

func (*fakeImageRemoveAPI) Close() error {
	return nil
}

func (f *fakeImageRemoveAPI) DeleteImage(kind, series, arch string) error {
	f.kind = kind
	f.series = series
	f.arch = arch
	return nil
}

func (s *removeImageCommandSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.mockAPI = &fakeImageRemoveAPI{}
	s.PatchValue(cachedimages.GetRemoveImageAPI, func(_ *cachedimages.CachedImagesCommandBase) (cachedimages.RemoveImageAPI, error) {
		return s.mockAPI, nil
	})
}

func runRemoveCommand(c *gc.C, args ...string) (*cmd.Context, error) {
	return testing.RunCommand(c, cachedimages.NewRemoveCommandForTest(), args...)
}

func (s *removeImageCommandSuite) TestRemoveImage(c *gc.C) {
	_, err := runRemoveCommand(c, "--kind", "lxd", "--series", "trusty", "--arch", "amd64")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.mockAPI.kind, gc.Equals, "lxd")
	c.Assert(s.mockAPI.series, gc.Equals, "trusty")
	c.Assert(s.mockAPI.arch, gc.Equals, "amd64")
}

func (*removeImageCommandSuite) TestTooManyArgs(c *gc.C) {
	_, err := runRemoveCommand(c, "--kind", "lxd", "--series", "trusty", "--arch", "amd64", "bad")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["bad"\]`)
}

func (*removeImageCommandSuite) TestKindRequired(c *gc.C) {
	_, err := runRemoveCommand(c, "--series", "trusty", "--arch", "amd64", "bad")
	c.Assert(err, gc.ErrorMatches, `image kind must be specified`)
}

func (*removeImageCommandSuite) TestSeriesRequired(c *gc.C) {
	_, err := runRemoveCommand(c, "--kind", "lxd", "--arch", "amd64", "bad")
	c.Assert(err, gc.ErrorMatches, `image series must be specified`)
}

func (*removeImageCommandSuite) TestArchRequired(c *gc.C) {
	_, err := runRemoveCommand(c, "--kind", "lxd", "--series", "trusty", "bad")
	c.Assert(err, gc.ErrorMatches, `image architecture must be specified`)
}
