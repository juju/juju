// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package hostkeyreporter_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/juju/errors"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2/dependency"
	"github.com/juju/worker/v2/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/worker/hostkeyreporter"
)

type Suite struct {
	jujutesting.IsolationSuite

	dir    string
	stub   *jujutesting.Stub
	facade *stubFacade
	config hostkeyreporter.Config
}

var _ = gc.Suite(&Suite{})

func (s *Suite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	// Generate some dummy key files
	s.dir = c.MkDir()
	sshDir := filepath.Join(s.dir, "etc", "ssh")
	err := os.MkdirAll(sshDir, 0755)
	c.Assert(err, jc.ErrorIsNil)
	writeKey := func(keyType string) {
		baseName := fmt.Sprintf("ssh_host_%s_key.pub", keyType)
		fileName := filepath.Join(sshDir, baseName)
		err := ioutil.WriteFile(fileName, []byte(keyType), 0644)
		c.Assert(err, jc.ErrorIsNil)
	}
	writeKey("dsa")
	writeKey("rsa")
	writeKey("ecdsa")

	s.stub = new(jujutesting.Stub)
	s.facade = newStubFacade(s.stub)
	s.config = hostkeyreporter.Config{
		Facade:    s.facade,
		MachineId: "42",
		RootDir:   s.dir,
	}
}

func (s *Suite) TestInvalidConfig(c *gc.C) {
	s.config.MachineId = ""
	_, err := hostkeyreporter.New(s.config)
	c.Check(err, gc.ErrorMatches, "empty MachineId .+")
	c.Check(s.stub.Calls(), gc.HasLen, 0)
}

func (s *Suite) TestNoSSHDir(c *gc.C) {
	// No /etc/ssh at all
	s.config.RootDir = c.MkDir()

	w, err := hostkeyreporter.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	err = workertest.CheckKilled(c, w)
	c.Check(errors.Cause(err), gc.Equals, dependency.ErrUninstall)
}

func (s *Suite) TestNoKeys(c *gc.C) {
	// Pass an empty /etc/ssh
	dir := c.MkDir()
	c.Assert(os.MkdirAll(filepath.Join(dir, "etc", "ssh"), 0777), jc.ErrorIsNil)
	s.config.RootDir = dir

	w, err := hostkeyreporter.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	err = workertest.CheckKilled(c, w)
	c.Check(err, gc.ErrorMatches, "no SSH host keys found")
}

func (s *Suite) TestReportKeysError(c *gc.C) {
	s.facade.reportErr = errors.New("blam")
	w, err := hostkeyreporter.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	err = workertest.CheckKilled(c, w)
	c.Check(err, gc.ErrorMatches, "blam")
}

func (s *Suite) TestSuccess(c *gc.C) {
	w, err := hostkeyreporter.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	err = workertest.CheckKilled(c, w)
	c.Check(err, gc.Equals, dependency.ErrUninstall)
	s.stub.CheckCalls(c, []jujutesting.StubCall{{
		"ReportKeys", []interface{}{"42", []string{"dsa", "ecdsa", "rsa"}},
	}})
}

func newStubFacade(stub *jujutesting.Stub) *stubFacade {
	return &stubFacade{
		stub: stub,
	}
}

type stubFacade struct {
	stub      *jujutesting.Stub
	reportErr error
}

func (c *stubFacade) ReportKeys(machineId string, publicKeys []string) error {
	c.stub.AddCall("ReportKeys", machineId, publicKeys)
	return c.reportErr
}
