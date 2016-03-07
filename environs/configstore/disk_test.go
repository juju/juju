// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package configstore_test

import (
	"os"
	"path/filepath"
	"runtime"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/testing"
)

var _ = gc.Suite(&diskInterfaceSuite{})

type diskInterfaceSuite struct {
	interfaceSuite
	dir string
}

func (s *diskInterfaceSuite) SetUpTest(c *gc.C) {
	s.interfaceSuite.SetUpTest(c)
	s.dir = c.MkDir()
	s.NewStore = func(c *gc.C) configstore.Storage {
		store, err := configstore.NewDisk(s.dir)
		c.Assert(err, jc.ErrorIsNil)
		return store
	}
}

func (s *diskInterfaceSuite) TearDownTest(c *gc.C) {
	s.NewStore = nil
	s.interfaceSuite.TearDownTest(c)
}

var _ = gc.Suite(&diskStoreSuite{})

type diskStoreSuite struct {
	testing.BaseSuite
}

func (*diskStoreSuite) TestNewDisk(c *gc.C) {
	dir := c.MkDir()
	store, err := configstore.NewDisk(filepath.Join(dir, "foo"))
	c.Assert(err, jc.Satisfies, os.IsNotExist)
	c.Assert(store, gc.IsNil)

	store, err = configstore.NewDisk(filepath.Join(dir))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(store, gc.NotNil)
}

func (*diskStoreSuite) TestReadNotFound(c *gc.C) {
	dir := c.MkDir()
	store, err := configstore.NewDisk(dir)
	c.Assert(err, jc.ErrorIsNil)
	info, err := store.ReadInfo("somemodel")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(info, gc.IsNil)
}

func (*diskStoreSuite) TestWriteFails(c *gc.C) {
	dir := c.MkDir()
	store, err := configstore.NewDisk(dir)
	c.Assert(err, jc.ErrorIsNil)

	info := store.CreateInfo("uuid", "somemodel")

	// Make the directory non-writable
	err = os.Chmod(dir, 0555)
	c.Assert(err, jc.ErrorIsNil)

	// Cannot use permissions properly on windows for now
	if runtime.GOOS != "windows" {
		err = info.Write()
		c.Assert(err, gc.ErrorMatches, ".* permission denied")
	}

	// Make the directory writable again so that gocheck can clean it up.
	err = os.Chmod(dir, 0777)
	c.Assert(err, jc.ErrorIsNil)
}

func (*diskStoreSuite) TestConcurrentAccessBreaksIfTimeExceeded(c *gc.C) {
	var tw loggo.TestWriter
	c.Assert(loggo.RegisterWriter("test-log", &tw, loggo.DEBUG), gc.IsNil)

	dir := c.MkDir()
	store, err := configstore.NewDisk(dir)
	c.Assert(err, jc.ErrorIsNil)

	_, err = configstore.AcquireEnvironmentLock(dir, "blocking-op")
	c.Assert(err, jc.ErrorIsNil)

	_, err = store.ReadInfo("somemodel")
	c.Check(err, jc.Satisfies, errors.IsNotFound)

	// Using . between environments and env.lock so we don't have to care
	// about forward vs. backwards slash separator.
	messages := []jc.SimpleMessage{
		{loggo.WARNING, `breaking configstore lock, lock dir: .*env\.lock`},
		{loggo.WARNING, `lock holder message: pid: \d+, operation: blocking-op`},
	}

	c.Check(tw.Log(), jc.LogMatches, messages)
}
