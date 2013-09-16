// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import (
	"os"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/filestorage"
	"launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/provider"
	jc "launchpad.net/juju-core/testing/checkers"
)

type bootstrapSuite struct {
	testing.JujuConnSuite
	env *localStorageEnviron
}

var _ = gc.Suite(&bootstrapSuite{})

type localStorageEnviron struct {
	environs.Environ
	storage           environs.Storage
	storageAddr       string
	storageDir        string
	sharedStorageAddr string
	sharedStorageDir  string
}

func (e *localStorageEnviron) BootstrapStorage() (environs.Storage, error) {
	return e.storage, nil
}

func (e *localStorageEnviron) StorageAddr() string {
	return e.storageAddr
}

func (e *localStorageEnviron) StorageDir() string {
	return e.storageDir
}

func (e *localStorageEnviron) SharedStorageAddr() string {
	return e.sharedStorageAddr
}

func (e *localStorageEnviron) SharedStorageDir() string {
	return e.sharedStorageDir
}

func (s *bootstrapSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.env = &localStorageEnviron{
		Environ:    s.Conn.Environ,
		storageDir: c.MkDir(),
	}
	storage, err := filestorage.NewFileStorageWriter(s.env.storageDir)
	c.Assert(err, gc.IsNil)
	s.env.storage = storage
}

func (s *bootstrapSuite) getArgs(c *gc.C) BootstrapArgs {
	hostname, err := os.Hostname()
	c.Assert(err, gc.IsNil)
	series := s.Conn.Environ.Config().DefaultSeries()
	toolsList, err := tools.FindBootstrapTools(s.Conn.Environ, nil, series, nil, false)
	c.Assert(err, gc.IsNil)
	return BootstrapArgs{
		Host:          hostname,
		Environ:       s.env,
		MachineId:     "0",
		PossibleTools: toolsList,
	}
}

func (s *bootstrapSuite) TestBootstrap(c *gc.C) {
	args := s.getArgs(c)
	args.Host = "ubuntu@" + args.Host

	defer fakeSSH{series: s.Conn.Environ.Config().DefaultSeries()}.install(c).Restore()
	err := Bootstrap(args)
	c.Assert(err, gc.IsNil)

	bootstrapState, err := provider.LoadState(s.env.storage)
	c.Assert(err, gc.IsNil)
	c.Assert(
		bootstrapState.StateInstances,
		gc.DeepEquals,
		[]instance.Id{BootstrapInstanceId},
	)

	// Do it all again; this should work, despite the fact that
	// there's a bootstrap state file. Existence for that is
	// checked in general bootstrap code (environs/bootstrap).
	defer fakeSSH{series: s.Conn.Environ.Config().DefaultSeries()}.install(c).Restore()
	err = Bootstrap(args)
	c.Assert(err, gc.IsNil)

	// We *do* check that the machine has no juju* upstart jobs, though.
	defer installFakeSSH(c, "", "/etc/init/jujud-machine-0.conf", 0).Restore()
	err = Bootstrap(args)
	c.Assert(err, gc.Equals, ErrProvisioned)
}

func (s *bootstrapSuite) TestBootstrapScriptFailure(c *gc.C) {
	args := s.getArgs(c)
	args.Host = "ubuntu@" + args.Host
	series := s.Conn.Environ.Config().DefaultSeries()
	defer fakeSSH{series: series, provisionAgentExitCode: 1}.install(c).Restore()
	err := Bootstrap(args)
	c.Assert(err, gc.NotNil)

	// Since the script failed, the state file should have been
	// removed from storage.
	_, err = provider.LoadState(s.env.storage)
	c.Assert(err, jc.Satisfies, os.IsNotExist)
}

func (s *bootstrapSuite) TestBootstrapEmptyHost(c *gc.C) {
	args := s.getArgs(c)
	args.Host = ""
	c.Assert(Bootstrap(args), gc.ErrorMatches, "host argument is empty")
}

func (s *bootstrapSuite) TestBootstrapNilEnviron(c *gc.C) {
	args := s.getArgs(c)
	args.Environ = nil
	c.Assert(Bootstrap(args), gc.ErrorMatches, "environ argument is nil")
}

func (s *bootstrapSuite) TestBootstrapInvalidMachineId(c *gc.C) {
	args := s.getArgs(c)
	args.MachineId = ""
	c.Assert(Bootstrap(args), gc.ErrorMatches, `"" is not a valid machine ID`)
	args.MachineId = "bahhumbug"
	c.Assert(Bootstrap(args), gc.ErrorMatches, `"bahhumbug" is not a valid machine ID`)
}

func (s *bootstrapSuite) TestBootstrapAlternativeMachineId(c *gc.C) {
	args := s.getArgs(c)
	args.MachineId = "1"
	defer fakeSSH{series: s.Conn.Environ.Config().DefaultSeries()}.install(c).Restore()
	c.Assert(Bootstrap(args), gc.IsNil)
}

func (s *bootstrapSuite) TestBootstrapNoMatchingTools(c *gc.C) {
	// Empty tools list.
	args := s.getArgs(c)
	args.PossibleTools = nil
	series := s.Conn.Environ.Config().DefaultSeries()
	defer fakeSSH{series: series, skipProvisionAgent: true}.install(c).Restore()
	c.Assert(Bootstrap(args), gc.ErrorMatches, "no matching tools available")

	// Non-empty list, but none that match the series/arch.
	defer fakeSSH{series: "edgy", skipProvisionAgent: true}.install(c).Restore()
	args = s.getArgs(c)
	c.Assert(Bootstrap(args), gc.ErrorMatches, "no matching tools available")
}
