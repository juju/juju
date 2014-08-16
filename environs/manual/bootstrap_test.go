// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual_test

import (
	"fmt"
	"io"
	"os"

	gc "launchpad.net/gocheck"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/cloudinit"
	"github.com/juju/juju/environs/filestorage"
	"github.com/juju/juju/environs/manual"
	"github.com/juju/juju/environs/storage"
	"github.com/juju/juju/environs/tools"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/network"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/version"
)

type bootstrapSuite struct {
	testing.JujuConnSuite
	env *localStorageEnviron
}

var _ = gc.Suite(&bootstrapSuite{})

type localStorageEnviron struct {
	environs.Environ
	storage     storage.Storage
	storageAddr string
	storageDir  string
}

func (e *localStorageEnviron) Storage() storage.Storage {
	return e.storage
}

func (e *localStorageEnviron) StorageAddr() string {
	return e.storageAddr
}

func (e *localStorageEnviron) StorageDir() string {
	return e.storageDir
}

func (s *bootstrapSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.env = &localStorageEnviron{
		Environ:    s.Environ,
		storageDir: c.MkDir(),
	}
	storage, err := filestorage.NewFileStorageWriter(s.env.storageDir)
	c.Assert(err, gc.IsNil)
	s.env.storage = storage
}

func (s *bootstrapSuite) getArgs(c *gc.C) manual.BootstrapArgs {
	hostname, err := os.Hostname()
	c.Assert(err, gc.IsNil)
	toolsList, err := tools.FindBootstrapTools(s.Environ, tools.BootstrapToolsParams{})
	c.Assert(err, gc.IsNil)
	arch := "amd64"
	return manual.BootstrapArgs{
		Host:          hostname,
		DataDir:       "/var/lib/juju",
		Environ:       s.env,
		PossibleTools: toolsList,
		Series:        "precise",
		HardwareCharacteristics: &instance.HardwareCharacteristics{
			Arch: &arch,
		},
		Context: coretesting.Context(c),
	}
}

func (s *bootstrapSuite) TestBootstrap(c *gc.C) {
	args := s.getArgs(c)
	args.Host = "ubuntu@" + args.Host

	defer fakeSSH{SkipDetection: true}.install(c).Restore()
	endpoints, err := manual.Bootstrap(args)
	c.Assert(endpoints, gc.DeepEquals, network.NewAddresses(args.Host))
	c.Assert(err, gc.IsNil)

	// If the machine has no juju* upstart jobs, then bootstrap
	// should fail with "machine is already provisioned".
	defer fakeSSH{
		Provisioned:        true,
		SkipDetection:      true,
		SkipProvisionAgent: true,
	}.install(c).Restore()
	_, err = manual.Bootstrap(args)
	c.Assert(err, gc.Equals, manual.ErrProvisioned)
}

func (s *bootstrapSuite) TestBootstrapScriptFailure(c *gc.C) {
	args := s.getArgs(c)
	args.Host = "ubuntu@" + args.Host
	defer fakeSSH{SkipDetection: true, ProvisionAgentExitCode: 1}.install(c).Restore()
	_, err := manual.Bootstrap(args)
	c.Assert(err, gc.NotNil)
}

func (s *bootstrapSuite) TestBootstrapEmptyDataDir(c *gc.C) {
	args := s.getArgs(c)
	args.DataDir = ""
	_, err := manual.Bootstrap(args)
	c.Assert(err, gc.ErrorMatches, "data-dir argument is empty")
}

func (s *bootstrapSuite) TestBootstrapEmptyHost(c *gc.C) {
	args := s.getArgs(c)
	args.Host = ""
	_, err := manual.Bootstrap(args)
	c.Assert(err, gc.ErrorMatches, "host argument is empty")
}

func (s *bootstrapSuite) TestBootstrapNilEnviron(c *gc.C) {
	args := s.getArgs(c)
	args.Environ = nil
	_, err := manual.Bootstrap(args)
	c.Assert(err, gc.ErrorMatches, "environ argument is nil")
}

func (s *bootstrapSuite) TestBootstrapNoMatchingTools(c *gc.C) {
	// Empty tools list.
	args := s.getArgs(c)
	args.PossibleTools = nil
	defer fakeSSH{SkipDetection: true, SkipProvisionAgent: true}.install(c).Restore()
	_, err := manual.Bootstrap(args)
	c.Assert(err, gc.ErrorMatches, "possible tools is empty")

	// Non-empty list, but none that match the series/arch.
	args = s.getArgs(c)
	args.Series = "edgy"
	defer fakeSSH{SkipDetection: true, SkipProvisionAgent: true}.install(c).Restore()
	_, err = manual.Bootstrap(args)
	c.Assert(err, gc.ErrorMatches, "no matching tools available")
}

func (s *bootstrapSuite) TestBootstrapToolsFileURL(c *gc.C) {
	storageName := tools.StorageName(version.Current)
	sftpURL, err := s.env.Storage().URL(storageName)
	c.Assert(err, gc.IsNil)
	fileURL := fmt.Sprintf("file://%s/%s", s.env.storageDir, storageName)
	s.testBootstrapToolsURL(c, sftpURL, fileURL)
}

func (s *bootstrapSuite) TestBootstrapToolsExternalURL(c *gc.C) {
	const externalURL = "http://test.invalid/tools.tgz"
	s.testBootstrapToolsURL(c, externalURL, externalURL)
}

func (s *bootstrapSuite) testBootstrapToolsURL(c *gc.C, toolsURL, expectedURL string) {
	s.PatchValue(manual.ProvisionMachineAgent, func(host string, mcfg *cloudinit.MachineConfig, w io.Writer) error {
		c.Assert(mcfg.Tools.URL, gc.Equals, expectedURL)
		return nil
	})
	args := s.getArgs(c)
	args.PossibleTools[0].URL = toolsURL
	defer fakeSSH{SkipDetection: true}.install(c).Restore()
	_, err := manual.Bootstrap(args)
	c.Assert(err, gc.IsNil)
}
