// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package looputil_test

import (
	"os"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/storage/looputil"
	"github.com/juju/juju/testing"
)

type LoopUtilSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&LoopUtilSuite{})

func (s *LoopUtilSuite) TestDetachLoopDevicesNone(c *gc.C) {
	commands := &mockRunCommand{c: c}
	defer commands.assertDrained()
	commands.expect("losetup", "-a")

	m := looputil.NewTestLoopDeviceManager(commands.run, nil, nil)
	err := m.DetachLoopDevices("", "")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *LoopUtilSuite) TestDetachLoopDevicesListError(c *gc.C) {
	commands := &mockRunCommand{c: c}
	defer commands.assertDrained()
	commands.expect("losetup", "-a").respond("", errors.New("badness"))

	m := looputil.NewTestLoopDeviceManager(commands.run, nil, nil)
	err := m.DetachLoopDevices("", "")
	c.Assert(err, gc.ErrorMatches, "listing loop devices: badness")
}

func (s *LoopUtilSuite) TestDetachLoopDevicesListBadOutput(c *gc.C) {
	commands := &mockRunCommand{c: c}
	defer commands.assertDrained()
	commands.expect("losetup", "-a").respond("bad output", nil)

	m := looputil.NewTestLoopDeviceManager(commands.run, nil, nil)
	err := m.DetachLoopDevices("", "")
	c.Assert(err, gc.ErrorMatches, `listing loop devices: cannot parse loop device info from "bad output"`)
}

func (s *LoopUtilSuite) TestDetachLoopDevicesListBadInode(c *gc.C) {
	commands := &mockRunCommand{c: c}
	defer commands.assertDrained()
	commands.expect("losetup", "-a").respond("/dev/loop0: [0]:99999999999999999999999 (woop)", nil)

	m := looputil.NewTestLoopDeviceManager(commands.run, nil, nil)
	err := m.DetachLoopDevices("", "")
	c.Assert(err, gc.ErrorMatches, `listing loop devices: parsing inode: strconv.ParseUint: parsing "99999999999999999999999": value out of range`)
}

func (s *LoopUtilSuite) TestDetachLoopDevicesNotFound(c *gc.C) {
	commands := &mockRunCommand{c: c}
	defer commands.assertDrained()
	commands.expect("losetup", "-a").respond("/dev/loop0: [0021]:7504142 (/tmp/test.dat)", nil)
	stat := func(string) (os.FileInfo, error) {
		return nil, os.ErrNotExist
	}
	m := looputil.NewTestLoopDeviceManager(commands.run, stat, nil)
	err := m.DetachLoopDevices("", "")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *LoopUtilSuite) TestDetachLoopDevicesStatError(c *gc.C) {
	commands := &mockRunCommand{c: c}
	defer commands.assertDrained()
	commands.expect("losetup", "-a").respond("/dev/loop0: [0021]:7504142 (/tmp/test.dat)", nil)
	stat := func(path string) (os.FileInfo, error) {
		return nil, errors.Errorf("stat fails for %q", path)
	}
	m := looputil.NewTestLoopDeviceManager(commands.run, stat, nil)
	err := m.DetachLoopDevices("", "")
	c.Assert(err, gc.ErrorMatches, `querying backing file: stat fails for "/tmp/test.dat"`)
}

func (s *LoopUtilSuite) TestDetachLoopDevicesInodeMismatch(c *gc.C) {
	commands := &mockRunCommand{c: c}
	defer commands.assertDrained()
	commands.expect("losetup", "-a").respond("/dev/loop0: [0021]:7504142 (/tmp/test.dat)", nil)
	stat := func(path string) (os.FileInfo, error) {
		return mockFileInfo{inode: 123}, nil
	}
	m := looputil.NewTestLoopDeviceManager(commands.run, stat, mockFileInfoInode)
	err := m.DetachLoopDevices("", "")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *LoopUtilSuite) TestDetachLoopDevicesInodeMatch(c *gc.C) {
	commands := &mockRunCommand{c: c}
	defer commands.assertDrained()
	commands.expect("losetup", "-a").respond("/dev/loop0: [0021]:7504142 (/tmp/test.dat)", nil)
	commands.expect("losetup", "-d", "/dev/loop0")
	stat := func(path string) (os.FileInfo, error) {
		return mockFileInfo{inode: 7504142}, nil
	}
	m := looputil.NewTestLoopDeviceManager(commands.run, stat, mockFileInfoInode)
	err := m.DetachLoopDevices("", "")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *LoopUtilSuite) TestDetachLoopDevicesDetachError(c *gc.C) {
	commands := &mockRunCommand{c: c}
	defer commands.assertDrained()
	commands.expect("losetup", "-a").respond("/dev/loop0: [0021]:7504142 (/tmp/test.dat)", nil)
	commands.expect("losetup", "-d", "/dev/loop0").respond("", errors.New("oh noes"))
	stat := func(path string) (os.FileInfo, error) {
		return mockFileInfo{inode: 7504142}, nil
	}
	m := looputil.NewTestLoopDeviceManager(commands.run, stat, mockFileInfoInode)
	err := m.DetachLoopDevices("", "")
	c.Assert(err, gc.ErrorMatches, `detaching loop device "/dev/loop0": oh noes`)
}

func (s *LoopUtilSuite) TestDetachLoopDevicesMultiple(c *gc.C) {
	commands := &mockRunCommand{c: c}
	defer commands.assertDrained()
	commands.expect("losetup", "-a").respond(
		"/dev/loop0: [0021]:7504142 (/tmp/test1.dat)\n"+
			"/dev/loop1: [002f]:7504143 (/tmp/test2.dat (deleted))\n"+
			"/dev/loop2: [002a]:7504144 (/tmp/test3.dat)",
		nil,
	)
	commands.expect("losetup", "-d", "/dev/loop0")
	commands.expect("losetup", "-d", "/dev/loop2")
	var statted []string
	stat := func(path string) (os.FileInfo, error) {
		statted = append(statted, path)
		switch path {
		case "/tmp/test1.dat":
			return mockFileInfo{inode: 7504142}, nil
		case "/tmp/test3.dat":
			return mockFileInfo{inode: 7504144}, nil
		}
		return nil, os.ErrNotExist
	}
	m := looputil.NewTestLoopDeviceManager(commands.run, stat, mockFileInfoInode)
	err := m.DetachLoopDevices("", "")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(statted, jc.DeepEquals, []string{"/tmp/test1.dat", "/tmp/test2.dat", "/tmp/test3.dat"})
}

func (s *LoopUtilSuite) TestDetachLoopDevicesAlternativeRoot(c *gc.C) {
	commands := &mockRunCommand{c: c}
	defer commands.assertDrained()
	commands.expect("losetup", "-a").respond("/dev/loop0: [0021]:7504142 (/tmp/test.dat)", nil)
	var statted string
	stat := func(path string) (os.FileInfo, error) {
		statted = path
		return nil, os.ErrNotExist
	}
	m := looputil.NewTestLoopDeviceManager(commands.run, stat, mockFileInfoInode)
	err := m.DetachLoopDevices("/var/lib/lxc/mycontainer/rootfs", "")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(statted, gc.Equals, "/var/lib/lxc/mycontainer/rootfs/tmp/test.dat")
}

func (s *LoopUtilSuite) TestDetachLoopDevicesAlternativeRootWithPrefix(c *gc.C) {
	commands := &mockRunCommand{c: c}
	defer commands.assertDrained()
	commands.expect("losetup", "-a").respond(
		"/dev/loop0: [0021]:7504142 (/var/lib/juju/storage/loop/volume-0-0)\n"+
			"/dev/loop1: [002f]:7504143 (/some/random/loop/device)",
		nil,
	)
	commands.expect("losetup", "-d", "/dev/loop0")
	var statted []string
	stat := func(path string) (os.FileInfo, error) {
		statted = append(statted, path)
		return mockFileInfo{inode: 7504142}, nil
	}
	m := looputil.NewTestLoopDeviceManager(commands.run, stat, mockFileInfoInode)
	err := m.DetachLoopDevices("/var/lib/lxc/mycontainer/rootfs", "/var/lib/juju")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(statted, jc.DeepEquals, []string{
		"/var/lib/lxc/mycontainer/rootfs/var/lib/juju/storage/loop/volume-0-0",
	})
}

type mockFileInfo struct {
	os.FileInfo
	inode uint64
}

func mockFileInfoInode(info os.FileInfo) uint64 {
	return info.(mockFileInfo).inode
}
