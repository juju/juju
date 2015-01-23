// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"io"
	"os"

	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/service/common"
)

type BaseSuite struct {
	gitjujutesting.IsolationSuite

	Conf    *common.Conf
	Confdir *confDir

	FakeInit  *fakeInit
	FakeFiles *fakeFiles
}

func (s *BaseSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.Conf = &common.Conf{
		Desc: "a service",
		Cmd:  "spam",
	}

	s.Confdir = &confDir{
		dirname:    "/var/lib/juju/init/jujud-machine-0",
		initSystem: initSystemUpstart,
	}

	// Patch a few things.
	s.FakeInit = &fakeInit{}
	s.FakeFiles = &fakeFiles{}
	s.PatchValue(&stat, s.FakeFiles.Stat)
	s.PatchValue(&readfile, s.FakeFiles.ReadFile)
	s.PatchValue(&create, s.FakeFiles.Create)
	s.PatchValue(&remove, s.FakeFiles.RemoveAll)
	s.PatchValue(&mkdirs, s.FakeFiles.MakedirAll)
}

// TODO(ericsnow) Use the fake in the testing repo as soon as it lands.

type FakeCallArgs map[string]interface{}

type FakeCall struct {
	FuncName string
	Args     FakeCallArgs
}

type fake struct {
	calls []FakeCall

	Errors []error
}

func (f *fake) err() error {
	if len(f.Errors) == 0 {
		return nil
	}
	err := f.Errors[0]
	f.Errors = f.Errors[1:]
	return err
}

func (f *fake) addCall(funcName string, args FakeCallArgs) {
	f.calls = append(f.calls, FakeCall{
		FuncName: funcName,
		Args:     args,
	})
}

func (f *fake) SetErrors(errors ...error) {
	f.Errors = errors
}

func (f *fake) CheckCalls(c *gc.C, expected []FakeCall) {
	c.Check(f.calls, jc.DeepEquals, expected)
}

type fakeFiles struct {
	fake

	FileInfo os.FileInfo
	Data     []byte
	File     io.WriteCloser
	NWritten int
}

func (ff *fakeFiles) Stat(filename string) (os.FileInfo, error) {
	ff.addCall("Stat", FakeCallArgs{
		"filename": filename,
	})
	return ff.FileInfo, ff.err()
}

func (ff *fakeFiles) ReadFile(filename string) ([]byte, error) {
	ff.addCall("ReadFile", FakeCallArgs{
		"filename": filename,
	})
	return ff.Data, ff.err()
}

func (ff *fakeFiles) RemoveAll(dirname string) error {
	ff.addCall("RemoveAll", FakeCallArgs{
		"dirname": dirname,
	})
	return ff.err()
}

func (ff *fakeFiles) Create(filename string) (io.WriteCloser, error) {
	ff.addCall("Create", FakeCallArgs{
		"filename": filename,
	})
	return ff.File, ff.err()
}

func (ff *fakeFiles) Write(data []byte) (int, error) {
	ff.addCall("Write", FakeCallArgs{
		"data": data,
	})
	return ff.NWritten, ff.err()
}

func (ff *fakeFiles) Close() error {
	ff.addCall("Close", nil)
	return ff.err()
}

type fakeInit struct {
	fake

	Names   []string
	Enabled bool
	SInfo   *common.ServiceInfo
	SConf   *common.Conf
	Data    []byte
}

func (fi *fakeInit) List(include ...string) ([]string, error) {
	fi.addCall("List", FakeCallArgs{
		"include": include,
	})
	return fi.Names, fi.err()
}

func (fi *fakeInit) Start(name string) error {
	fi.addCall("Start", FakeCallArgs{
		"name": name,
	})
	return fi.err()
}

func (fi *fakeInit) Stop(name string) error {
	fi.addCall("Stop", FakeCallArgs{
		"name": name,
	})
	return fi.err()
}

func (fi *fakeInit) Enable(name, filename string) error {
	fi.addCall("Enable", FakeCallArgs{
		"name":     name,
		"filename": filename,
	})
	return fi.err()
}

func (fi *fakeInit) Disable(name string) error {
	fi.addCall("Disable", FakeCallArgs{
		"name": name,
	})
	return fi.err()
}

func (fi *fakeInit) IsEnabled(name string, filenames ...string) (bool, error) {
	fi.addCall("IsEnabled", FakeCallArgs{
		"name":      name,
		"filenames": filenames,
	})
	return fi.Enabled, fi.err()
}

func (fi *fakeInit) Info(name string) (*common.ServiceInfo, error) {
	fi.addCall("Info", FakeCallArgs{
		"name": name,
	})
	return fi.SInfo, fi.err()
}

func (fi *fakeInit) Conf(name string) (*common.Conf, error) {
	fi.addCall("Conf", FakeCallArgs{
		"name": name,
	})
	return fi.SConf, fi.err()
}

func (fi *fakeInit) Serialize(conf *common.Conf) ([]byte, error) {
	fi.addCall("Serialize", FakeCallArgs{
		"conf": conf,
	})
	return fi.Data, fi.err()
}

func (fi *fakeInit) Deserialize(data []byte) (*common.Conf, error) {
	fi.addCall("Deserialize", FakeCallArgs{
		"data": data,
	})
	return fi.SConf, fi.err()
}
