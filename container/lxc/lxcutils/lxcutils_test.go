// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxcutils_test

import (
	"path/filepath"
	"runtime"
	stdtesting "testing"

	jc "github.com/juju/testing/checkers"
	ft "github.com/juju/testing/filetesting"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/container/lxc/lxcutils"
	"github.com/juju/juju/testing"
)

func Test(t *stdtesting.T) {
	if runtime.GOOS != "linux" {
		t.Skip("LXC is a Linux thing")
	}
	gc.TestingT(t)
}

type LxcUtilsSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&LxcUtilsSuite{})

var lxcCgroupContents = `11:hugetlb:/lxc/juju-machine-1-lxc-0
10:perf_event:/lxc/juju-machine-1-lxc-0
9:blkio:/lxc/juju-machine-1-lxc-0
8:freezer:/lxc/juju-machine-1-lxc-0
7:devices:/lxc/juju-machine-1-lxc-0
6:memory:/lxc/juju-machine-1-lxc-0
5:cpuacct:/lxc/juju-machine-1-lxc-0
4:cpu:/lxc/juju-machine-1-lxc-0
3:cpuset:/lxc/juju-machine-1-lxc-0
2:name=systemd:/lxc/juju-machine-1-lxc-0
`

var hostCgroupContents = `11:hugetlb:/
10:perf_event:/
9:blkio:/
8:freezer:/
7:devices:/
6:memory:/
5:cpuacct:/
4:cpu:/
3:cpuset:/
2:name=systemd:/
`

var malformedCgroupFile = `some bogus content
more bogus content`

func (s *LxcUtilsSuite) TestRunningInsideLXCOnHost(c *gc.C) {
	baseDir := c.MkDir()
	cgroup := filepath.Join(baseDir, "cgroup")

	ft.File{"cgroup", hostCgroupContents, 0400}.Create(c, baseDir)

	s.PatchValue(lxcutils.InitProcessCgroupFile, cgroup)
	runningInLXC, err := lxcutils.RunningInsideLXC()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(runningInLXC, jc.IsFalse)
}

func (s *LxcUtilsSuite) TestRunningInsideLXCOnLXCContainer(c *gc.C) {
	baseDir := c.MkDir()
	cgroup := filepath.Join(baseDir, "cgroup")

	ft.File{"cgroup", lxcCgroupContents, 0400}.Create(c, baseDir)

	s.PatchValue(lxcutils.InitProcessCgroupFile, cgroup)
	runningInLXC, err := lxcutils.RunningInsideLXC()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(runningInLXC, jc.IsTrue)
}

func (s *LxcUtilsSuite) TestRunningInsideLXCMissingCgroupFile(c *gc.C) {
	s.PatchValue(lxcutils.InitProcessCgroupFile, "")
	_, err := lxcutils.RunningInsideLXC()
	c.Assert(err.Error(), gc.Matches, "open : no such file or directory")
}

func (s *LxcUtilsSuite) TestRunningInsideLXCMalformedCgroupFile(c *gc.C) {
	baseDir := c.MkDir()
	cgroup := filepath.Join(baseDir, "cgroup")

	ft.File{"cgroup", malformedCgroupFile, 0400}.Create(c, baseDir)

	s.PatchValue(lxcutils.InitProcessCgroupFile, cgroup)
	_, err := lxcutils.RunningInsideLXC()
	c.Assert(err.Error(), gc.Equals, "malformed cgroup file")
}
