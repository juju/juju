// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"os"

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

	Stub      *testing.Stub
	StubInit  *initsystems.Stub
	StubFile  *fs.StubFile
	StubFiles *fs.StubOps
}

func (s *BaseSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.DataDir = "/var/lib/juju"
	s.Conf = &Conf{Conf: initsystems.Conf{
		Desc: "a service",
		Cmd:  "spam",
	}}

	// Patch a few things.
	s.Stub = &testing.Stub{}
	s.StubInit = &initsystems.Stub{Stub: s.Stub}
	s.StubFile = &fs.StubFile{Stub: s.Stub}
	s.StubFiles = &fs.StubOps{Stub: s.Stub}
	s.StubFiles.Returns.File = s.StubFile

	s.PatchValue(&newFileOps, func() fs.Operations {
		return s.StubFiles
	})

	name := "jujud-machine-0"
	initDir := s.DataDir + "/init"
	s.Confdir = newConfDir(name, initDir, InitSystemUpstart, nil)
}

func newStubFile(name string, data []byte) os.FileInfo {
	return fs.NewFile(name, 0644, data)
}

func newStubDir(name string) os.FileInfo {
	return fs.NewDir(name, 0755)
}
