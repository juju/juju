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

	FakeInit  *fakeInit
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
	s.FakeInit = &fakeInit{}
	s.FakeFile = &fs.FakeFile{}
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

// TODO(ericsnow) Move fakeInit to service/testing.

type fakeInit struct {
	testing.Fake

	Names   []string
	Enabled bool
	SInfo   *initsystems.ServiceInfo
	SConf   *initsystems.Conf
	Data    []byte
}

func (fi *fakeInit) List(include ...string) ([]string, error) {
	fi.AddCall("List", testing.FakeCallArgs{
		"include": include,
	})
	return fi.Names, fi.Err()
}

func (fi *fakeInit) Start(name string) error {
	fi.AddCall("Start", testing.FakeCallArgs{
		"name": name,
	})
	return fi.Err()
}

func (fi *fakeInit) Stop(name string) error {
	fi.AddCall("Stop", testing.FakeCallArgs{
		"name": name,
	})
	return fi.Err()
}

func (fi *fakeInit) Enable(name, filename string) error {
	fi.AddCall("Enable", testing.FakeCallArgs{
		"name":     name,
		"filename": filename,
	})
	return fi.Err()
}

func (fi *fakeInit) Disable(name string) error {
	fi.AddCall("Disable", testing.FakeCallArgs{
		"name": name,
	})
	return fi.Err()
}

func (fi *fakeInit) IsEnabled(name string, filenames ...string) (bool, error) {
	fi.AddCall("IsEnabled", testing.FakeCallArgs{
		"name":      name,
		"filenames": filenames,
	})
	return fi.Enabled, fi.Err()
}

func (fi *fakeInit) Info(name string) (*initsystems.ServiceInfo, error) {
	fi.AddCall("Info", testing.FakeCallArgs{
		"name": name,
	})
	return fi.SInfo, fi.Err()
}

func (fi *fakeInit) Conf(name string) (*initsystems.Conf, error) {
	fi.AddCall("Conf", testing.FakeCallArgs{
		"name": name,
	})
	return fi.SConf, fi.Err()
}

func (fi *fakeInit) Serialize(conf *initsystems.Conf) ([]byte, error) {
	fi.AddCall("Serialize", testing.FakeCallArgs{
		"conf": conf,
	})
	return fi.Data, fi.Err()
}

func (fi *fakeInit) Deserialize(data []byte) (*initsystems.Conf, error) {
	fi.AddCall("Deserialize", testing.FakeCallArgs{
		"data": data,
	})
	return fi.SConf, fi.Err()
}
