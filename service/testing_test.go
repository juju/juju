// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"github.com/juju/testing"
	"github.com/juju/utils/fs"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/service/initsystems"
)

type BaseSuite struct {
	testing.IsolationSuite

	DataDir string
	Conf    *Conf
	Confdir *confDir

	FakeInit  *initsystems.Fake
	FakeFile  *fs.FakeFile
	FakeFiles *fs.FakeOps
}

func (s *BaseSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.DataDir = "/var/lib/juju"
	s.Conf = &Conf{Conf: initsystems.Conf{
		Desc: "a service",
		Cmd:  "spam",
	}}

	// Patch a few things.
	s.FakeInit = &initsystems.Fake{}
	s.FakeFile = fs.NewFakeFile()
	s.FakeFiles = fs.NewFakeOps()
	s.FakeFiles.Returns.File = s.FakeFile

	s.PatchValue(&newFileOps, func() fs.Operations {
		return s.FakeFiles
	})

	name := "jujud-machine-0"
	initDir := s.DataDir + "/init"
	s.Confdir = newConfDir(name, initDir, InitSystemUpstart, nil)
}

func newFakeFile(name string, data []byte) *fs.File {
	return fs.NewFile(name, 0644, data)
}

func newFakeDir(name string) *fs.File {
	return fs.NewDir(name, 0755)
}
