// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package executable_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/workload/plugin/executable"
)

type pathsSuite struct {
	testing.IsolationSuite

	stub *testing.Stub
}

var _ = gc.Suite(&pathsSuite{})

func (s *pathsSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.stub = &testing.Stub{}
}

func (s *pathsSuite) TestNewPaths(c *gc.C) {
	p := executable.NewPaths("some-base-dir", "a-plugin")

	c.Check(p, jc.DeepEquals, executable.Paths{
		Plugin:             "a-plugin",
		DataDir:            filepath.Join("some-base-dir", "plugins", "a-plugin"),
		ExecutablePathFile: filepath.Join("some-base-dir", "plugins", "a-plugin", ".executable"),
		Fops:               p.Fops,
	})
}

func (s *pathsSuite) TestString(c *gc.C) {
	p := executable.NewPaths("some-base-dir", "a-plugin")
	str := p.String()
	quoted := fmt.Sprintf("%q", p)

	expected := filepath.Join("some-base-dir", "plugins", "a-plugin")
	c.Check(str, gc.Equals, expected)
	c.Check(quoted, gc.Equals, `"`+expected+`"`)
}

func (s *pathsSuite) TestExecutable(c *gc.C) {
	executablePathFile := filepath.Join("some-base-dir", "plugins", "a-plugin", ".executable")
	expected := filepath.Join("some", "dir", "juju-workload-a-plugin")
	fops := &stubFops{stub: s.stub}
	fops.dataOut = expected

	p := executable.NewPaths("some-base-dir", "a-plugin")
	p.Fops = fops
	exePath, err := p.Executable()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(exePath, gc.Equals, expected)
	s.stub.CheckCalls(c, []testing.StubCall{{
		FuncName: "ReadFile",
		Args: []interface{}{
			executablePathFile,
		},
	}})
}

func (s *pathsSuite) TestInit(c *gc.C) {
	executablePathFile := filepath.Join("some-base-dir", "plugins", "a-plugin", ".executable")
	dataDir := filepath.Join("some-base-dir", "plugins", "a-plugin")
	fops := &stubFops{stub: s.stub}

	p := executable.NewPaths("some-base-dir", "a-plugin")
	p.Fops = fops
	exePath := filepath.Join("some", "dir", "juju-workload-a-plugin")
	err := p.Init(exePath)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCalls(c, []testing.StubCall{{
		FuncName: "MkdirAll",
		Args: []interface{}{
			dataDir,
			os.FileMode(0755),
		},
	}, {
		FuncName: "WriteFile",
		Args: []interface{}{
			executablePathFile,
			[]byte(exePath),
			os.FileMode(0644),
		},
	}})
}

type stubFops struct {
	stub *testing.Stub

	dataOut string
	found   string
	abs     string
}

func (s *stubFops) MkdirAll(path string, perm os.FileMode) error {
	s.stub.AddCall("MkdirAll", path, perm)
	if err := s.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (s *stubFops) ReadFile(filename string) ([]byte, error) {
	s.stub.AddCall("ReadFile", filename)
	if err := s.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return []byte(s.dataOut), nil
}

func (s *stubFops) WriteFile(filename string, data []byte, perm os.FileMode) error {
	s.stub.AddCall("WriteFile", filename, data, perm)
	if err := s.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (s *stubFops) LookPath(name string) (string, error) {
	s.stub.AddCall("LookPath", name)
	if err := s.stub.NextErr(); err != nil {
		return "", errors.Trace(err)
	}

	if s.found == "" {
		return "", exec.ErrNotFound
	}
	return s.found, nil
}

func (s *stubFops) Abs(path string) (string, error) {
	s.stub.AddCall("Abs", path)
	if err := s.stub.NextErr(); err != nil {
		return "", errors.Trace(err)
	}

	if s.abs == "" {
		return path, nil
	}
	return s.abs, nil
}
