// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxc_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/container/lxc"
	coretesting "github.com/juju/juju/testing"
)

type cloneSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&cloneSuite{})

func (s *cloneSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.PatchValue(&lxc.TemplateLockDir, c.MkDir())
}

func (s *cloneSuite) createFiles(c *gc.C) {
	dirs := []string{
		"juju-precise-template",
		"juju-trusty-template",
		"other",
	}
	files := []string{
		"other-file",
	}
	for _, d := range dirs {
		err := os.MkdirAll(filepath.Join(lxc.TemplateLockDir, d), 0755)
		c.Assert(err, gc.IsNil)
	}
	for _, f := range files {
		err := ioutil.WriteFile(filepath.Join(lxc.TemplateLockDir, f), []byte{}, 0755)
		c.Assert(err, gc.IsNil)
	}
}

func (s *cloneSuite) TestRemoveTemplateLockFiles(c *gc.C) {
	s.createFiles(c)
	err := lxc.RemoveTemplateLockFiles()
	c.Assert(err, gc.IsNil)
	fileInfo, err := ioutil.ReadDir(lxc.TemplateLockDir)
	c.Assert(err, gc.IsNil)
	var remainingNames []string
	for _, f := range fileInfo {
		remainingNames = append(remainingNames, f.Name())
	}
	c.Assert(remainingNames, jc.SameContents, []string{"other", "other-file"})
}

func (s *cloneSuite) TestRemoveTemplateLockFilesSomeFail(c *gc.C) {
	s.createFiles(c)
	s.PatchValue(lxc.RemoveLockFile, func(path string) error {
		if path == filepath.Join(lxc.TemplateLockDir, "juju-precise-template") {
			return fmt.Errorf("nope")
		}
		return os.RemoveAll(path)
	})
	err := lxc.RemoveTemplateLockFiles()
	c.Assert(err, gc.ErrorMatches, "failed to remove all 2 lock files, only 1 removed")
	fileInfo, err := ioutil.ReadDir(lxc.TemplateLockDir)
	c.Assert(err, gc.IsNil)
	var remainingNames []string
	for _, f := range fileInfo {
		remainingNames = append(remainingNames, f.Name())
	}
	c.Assert(remainingNames, jc.SameContents, []string{"other", "other-file", "juju-precise-template"})
}
