// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package local_test

import (
	"io/ioutil"
	gc "launchpad.net/gocheck"
	"path/filepath"

	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/provider/local"
	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/testing/testbase"
)

type lxcTest struct {
	testbase.LoggingSuite
}

var _ = gc.Suite(&lxcTest{})

func (*lxcTest) TestUseFastLXCForContainer(c *gc.C) {
	c.Assert(local.UseFastLXC(instance.ContainerType("")), jc.IsFalse)
	c.Assert(local.UseFastLXC(instance.KVM), jc.IsFalse)
}

func (t *lxcTest) TestUseFastLXC(c *gc.C) {
	for i, test := range []struct {
		message        string
		releaseContent string
		expected       bool
		overrideSlow   string
	}{{
		message: "missing release file",
	}, {
		message:        "missing prefix in file",
		releaseContent: "some junk\nand more junk",
	}, {
		message: "precise release",
		releaseContent: `
DISTRIB_ID=Ubuntu
DISTRIB_RELEASE=12.04
DISTRIB_CODENAME=precise
DISTRIB_DESCRIPTION="Ubuntu 12.04.3 LTS"
`,
	}, {
		message: "trusty release",
		releaseContent: `
DISTRIB_ID=Ubuntu
DISTRIB_RELEASE=14.04
DISTRIB_CODENAME=trusty
DISTRIB_DESCRIPTION="Ubuntu Trusty Tahr (development branch)"
`,
		expected: true,
	}, {
		message:        "minimal trusty release",
		releaseContent: `DISTRIB_RELEASE=14.04`,
		expected:       true,
	}, {
		message:        "minimal unstable unicorn",
		releaseContent: `DISTRIB_RELEASE=14.10`,
		expected:       true,
	}, {
		message:        "minimal jaunty",
		releaseContent: `DISTRIB_RELEASE=9.10`,
	}, {
		message:        "env override",
		releaseContent: `DISTRIB_RELEASE=14.04`,
		overrideSlow:   "value",
	}} {
		c.Logf("%v: %v", i, test.message)
		filename := filepath.Join(c.MkDir(), "lsbRelease")
		t.PatchValue(local.LSBReleaseFileVar, filename)
		if test.releaseContent != "" {
			err := ioutil.WriteFile(filename, []byte(test.releaseContent+"\n"), 0644)
			c.Assert(err, gc.IsNil)
		}
		t.PatchEnvironment(local.EnvKeyTestingForceSlow, test.overrideSlow)
		value := local.UseFastLXC(instance.LXC)
		c.Assert(value, gc.Equals, test.expected)
	}
}
