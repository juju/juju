// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package hostkeyreporter_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/juju/errors"
	"github.com/juju/tc"
	jujutesting "github.com/juju/testing"
	"github.com/juju/worker/v4/dependency"
	"github.com/juju/worker/v4/workertest"

	"github.com/juju/juju/internal/worker/hostkeyreporter"
)

type Suite struct {
	jujutesting.IsolationSuite

	dir    string
	stub   *jujutesting.Stub
	facade *stubFacade
	config hostkeyreporter.Config
}

var _ = tc.Suite(&Suite{})

func (s *Suite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)

	// Generate some dummy key files
	s.dir = c.MkDir()
	sshDir := filepath.Join(s.dir, "etc", "ssh")
	err := os.MkdirAll(sshDir, 0755)
	c.Assert(err, tc.ErrorIsNil)
	writeKey := func(keyType string) {
		baseName := fmt.Sprintf("ssh_host_%s_key.pub", keyType)
		fileName := filepath.Join(sshDir, baseName)
		err := os.WriteFile(fileName, []byte(keyType), 0644)
		c.Assert(err, tc.ErrorIsNil)
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

func (s *Suite) TestInvalidConfig(c *tc.C) {
	s.config.MachineId = ""
	_, err := hostkeyreporter.New(s.config)
	c.Check(err, tc.ErrorMatches, "empty MachineId .+")
	c.Check(s.stub.Calls(), tc.HasLen, 0)
}

func (s *Suite) TestNoSSHDir(c *tc.C) {
	// No /etc/ssh at all
	s.config.RootDir = c.MkDir()

	w, err := hostkeyreporter.New(s.config)
	c.Assert(err, tc.ErrorIsNil)
	err = workertest.CheckKilled(c, w)
	c.Check(errors.Cause(err), tc.Equals, dependency.ErrUninstall)
}

func (s *Suite) TestNoKeys(c *tc.C) {
	// Pass an empty /etc/ssh
	dir := c.MkDir()
	c.Assert(os.MkdirAll(filepath.Join(dir, "etc", "ssh"), 0777), tc.ErrorIsNil)
	s.config.RootDir = dir

	w, err := hostkeyreporter.New(s.config)
	c.Assert(err, tc.ErrorIsNil)
	err = workertest.CheckKilled(c, w)
	c.Check(err, tc.ErrorMatches, "no SSH host keys found")
}

func (s *Suite) TestReportKeysError(c *tc.C) {
	s.facade.reportErr = errors.New("blam")
	w, err := hostkeyreporter.New(s.config)
	c.Assert(err, tc.ErrorIsNil)
	err = workertest.CheckKilled(c, w)
	c.Check(err, tc.ErrorMatches, "blam")
}

func (s *Suite) TestSuccess(c *tc.C) {
	w, err := hostkeyreporter.New(s.config)
	c.Assert(err, tc.ErrorIsNil)
	err = workertest.CheckKilled(c, w)
	c.Check(err, tc.Equals, dependency.ErrUninstall)
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

func (c *stubFacade) ReportKeys(_ context.Context, machineId string, publicKeys []string) error {
	c.stub.AddCall("ReportKeys", machineId, publicKeys)
	return c.reportErr
}
