// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	"io/ioutil"
	"path"
	"runtime"

	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/upgrades"
)

type ensureDotProfileSuite struct {
	testing.FakeJujuHomeSuite
	home string
	ctx  upgrades.Context
}

var _ = gc.Suite(&ensureDotProfileSuite{})

func (s *ensureDotProfileSuite) SetUpTest(c *gc.C) {
	//TODO(bogdanteleaga): Fix these on windows
	if runtime.GOOS == "windows" {
		c.Skip("bug 1403084: tests use bash scripts, will be fixed later on windows")
	}
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
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ensureDotProfileSuite) assertProfile(c *gc.C, content string) {
	dotProfile := path.Join(s.home, ".profile")
	data, err := ioutil.ReadFile(dotProfile)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(data), gc.Equals, content)
}

func (s *ensureDotProfileSuite) TestSourceAdded(c *gc.C) {
	s.writeDotProfile(c, "")
	err := upgrades.EnsureUbuntuDotProfileSourcesProxyFile(s.ctx)
	c.Assert(err, jc.ErrorIsNil)
	s.assertProfile(c, expectedLine)
}

func (s *ensureDotProfileSuite) TestIdempotent(c *gc.C) {
	s.writeDotProfile(c, "")
	err := upgrades.EnsureUbuntuDotProfileSourcesProxyFile(s.ctx)
	c.Assert(err, jc.ErrorIsNil)
	err = upgrades.EnsureUbuntuDotProfileSourcesProxyFile(s.ctx)
	c.Assert(err, jc.ErrorIsNil)
	s.assertProfile(c, expectedLine)
}

func (s *ensureDotProfileSuite) TestProfileUntouchedIfJujuProxyInSource(c *gc.C) {
	content := "source .juju-proxy\n"
	s.writeDotProfile(c, content)
	err := upgrades.EnsureUbuntuDotProfileSourcesProxyFile(s.ctx)
	c.Assert(err, jc.ErrorIsNil)
	s.assertProfile(c, content)
}

func (s *ensureDotProfileSuite) TestSkippedIfDotProfileDoesntExist(c *gc.C) {
	err := upgrades.EnsureUbuntuDotProfileSourcesProxyFile(s.ctx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(path.Join(s.home, ".profile"), jc.DoesNotExist)
}
