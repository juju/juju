// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	"io/ioutil"
	"path"

	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/upgrades"
)

type ensureDotProfileSuite struct {
	testing.FakeJujuHomeSuite
	home string
	ctx  upgrades.Context
}

var _ = gc.Suite(&ensureDotProfileSuite{})

func (s *ensureDotProfileSuite) SetUpTest(c *gc.C) {
	s.FakeJujuHomeSuite.SetUpTest(c)

	loggo.GetLogger("juju.upgrade").SetLogLevel(loggo.TRACE)

	s.home = c.MkDir()
	s.PatchValue(upgrades.UbuntuHome, s.home)
	s.ctx = &mockContext{}
}

const expectedLine = `
# Added by juju
[ -f "$HOME/.juju-proxy" ] && . "$HOME/.juju-proxy"
`

func (s *ensureDotProfileSuite) writeDotProfile(c *gc.C, content string) {
	dotProfile := path.Join(s.home, ".profile")
	err := ioutil.WriteFile(dotProfile, []byte(content), 0644)
	c.Assert(err, gc.IsNil)
}

func (s *ensureDotProfileSuite) assertProfile(c *gc.C, content string) {
	dotProfile := path.Join(s.home, ".profile")
	data, err := ioutil.ReadFile(dotProfile)
	c.Assert(err, gc.IsNil)
	c.Assert(string(data), gc.Equals, content)
}

func (s *ensureDotProfileSuite) TestSourceAdded(c *gc.C) {
	s.writeDotProfile(c, "")
	err := upgrades.EnsureUbuntuDotProfileSourcesProxyFile(s.ctx)
	c.Assert(err, gc.IsNil)
	s.assertProfile(c, expectedLine)
}

func (s *ensureDotProfileSuite) TestIdempotent(c *gc.C) {
	s.writeDotProfile(c, "")
	err := upgrades.EnsureUbuntuDotProfileSourcesProxyFile(s.ctx)
	c.Assert(err, gc.IsNil)
	err = upgrades.EnsureUbuntuDotProfileSourcesProxyFile(s.ctx)
	c.Assert(err, gc.IsNil)
	s.assertProfile(c, expectedLine)
}

func (s *ensureDotProfileSuite) TestProfileUntouchedIfJujuProxyInSource(c *gc.C) {
	content := "source .juju-proxy\n"
	s.writeDotProfile(c, content)
	err := upgrades.EnsureUbuntuDotProfileSourcesProxyFile(s.ctx)
	c.Assert(err, gc.IsNil)
	s.assertProfile(c, content)
}

func (s *ensureDotProfileSuite) TestSkippedIfDotProfileDoesntExist(c *gc.C) {
	err := upgrades.EnsureUbuntuDotProfileSourcesProxyFile(s.ctx)
	c.Assert(err, gc.IsNil)
	c.Assert(path.Join(s.home, ".profile"), jc.DoesNotExist)
}
