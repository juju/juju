// Copyright 2012-2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"context"
	"os"
	"path/filepath"

	"github.com/juju/tc"

	"github.com/juju/juju/core/blockdevice"
	"github.com/juju/juju/core/network"
	jujuversion "github.com/juju/juju/core/version"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/authenticationworker"
	"github.com/juju/juju/internal/worker/diskmanager"
	"github.com/juju/juju/internal/worker/machiner"
)

type commonMachineSuite struct {
	AgentSuite
	// FakeJujuXDGDataHomeSuite is needed only because the
	// authenticationworker writes to ~/.ssh.
	coretesting.FakeJujuXDGDataHomeSuite
}

func (s *commonMachineSuite) SetUpSuite(c *tc.C) {
	s.AgentSuite.SetUpSuite(c)
	// Set up FakeJujuXDGDataHomeSuite after AgentSuite since
	// AgentSuite clears all env vars.
	s.FakeJujuXDGDataHomeSuite.SetUpSuite(c)

	// Stub out executables etc used by workers.
	s.PatchValue(&jujuversion.Current, coretesting.FakeVersionNumber)
	s.PatchValue(&authenticationworker.SSHUser, "")
	s.PatchValue(&diskmanager.DefaultListBlockDevices, func(context.Context) ([]blockdevice.BlockDevice, error) {
		return nil, nil
	})
	s.PatchValue(&machiner.GetObservedNetworkConfig, func(_ network.ConfigSource) (network.InterfaceInfos, error) {
		return nil, nil
	})
}

func (s *commonMachineSuite) SetUpTest(c *tc.C) {
	s.AgentSuite.SetUpTest(c)
	// Set up FakeJujuXDGDataHomeSuite after AgentSuite since
	// AgentSuite clears all env vars.
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)

	testpath := c.MkDir()
	s.PatchEnvPathPrepend(testpath)
	// mock out the start method so we can fake install services without sudo
	fakeCmd(filepath.Join(testpath, "start"))
	fakeCmd(filepath.Join(testpath, "stop"))
}

func fakeCmd(path string) {
	err := os.WriteFile(path, []byte("#!/bin/bash --norc\nexit 0"), 0755)
	if err != nil {
		panic(err)
	}
}

func (s *commonMachineSuite) TearDownTest(c *tc.C) {
	s.FakeJujuXDGDataHomeSuite.TearDownTest(c)
	s.AgentSuite.TearDownTest(c)
}

func (s *commonMachineSuite) TearDownSuite(c *tc.C) {
	s.FakeJujuXDGDataHomeSuite.TearDownSuite(c)
	s.AgentSuite.TearDownSuite(c)
}
