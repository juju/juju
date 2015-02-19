// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package initsystems

import (
	"path"

	"github.com/juju/testing"
	"github.com/juju/utils/fs"
	gc "gopkg.in/check.v1"
)

type BaseSuite struct {
	testing.IsolationSuite

	DataDir string
	Conf    Conf

	Stub  *testing.Stub
	Init  *Stub
	File  *fs.StubFile
	Files *fs.StubOps
}

func (s *BaseSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.DataDir = "/var/lib/juju"
	s.Conf = Conf{
		Desc: "a service",
		Cmd:  "spam",
	}

	s.Stub = &testing.Stub{}
	s.Init = &Stub{Stub: s.Stub}
	s.File = &fs.StubFile{Stub: s.Stub}
	s.Files = &fs.StubOps{Stub: s.Stub}
	s.Files.Returns.File = s.File
}

func (s *BaseSuite) ConfDirInfo(name string) ConfDirInfo {
	return ConfDirInfo{confDirInfo{
		DirName:    path.Join(s.DataDir, "init", name),
		Name:       name,
		InitSystem: "upstart",
	}}
}

func (s *BaseSuite) ConfDir(name, data string) ConfDir {
	return ConfDir{
		confDirInfo: confDirInfo{
			DirName:    path.Join(s.DataDir, "init", name),
			Name:       name,
			InitSystem: "upstart",
		},
		ConfName: name + ".conf",
		Conf:     s.Conf,
		ConfFile: FileData{
			FileName: name + ".conf",
			Data:     []byte(data),
		},
	}
}
