// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"os"
	"time"

	"github.com/juju/errors"
	"github.com/juju/testing"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/service/common"
	svctesting "github.com/juju/juju/service/common/testing"
	"github.com/juju/juju/version"
)

// Stub stubs out the external functions used in the service package.
type Stub struct {
	*testing.Stub

	Version version.Binary
	Service Service
}

// GetVersion stubs out .
func (s *Stub) GetVersion() version.Binary {
	s.AddCall("GetVersion")

	// Pop the next error off the queue, even though we don't use it.
	s.NextErr()
	return s.Version
}

// DiscoverService stubs out service.DiscoverService.
func (s *Stub) DiscoverService(name string) (Service, error) {
	s.AddCall("DiscoverService", name)

	return s.Service, s.NextErr()
}

// TODO(ericsnow) StubFileInfo belongs in utils/fs.

// StubFileInfo implements os.FileInfo.
type StubFileInfo struct{}

func (StubFileInfo) Name() string       { return "" }
func (StubFileInfo) Size() int64        { return 0 }
func (StubFileInfo) Mode() os.FileMode  { return 0 }
func (StubFileInfo) ModTime() time.Time { return time.Time{} }
func (StubFileInfo) IsDir() bool        { return false }
func (StubFileInfo) Sys() interface{}   { return nil }

// StubFileInfo implements os.FileInfo for symlinks.
type StubSymlinkInfo struct{ StubFileInfo }

func (StubSymlinkInfo) Mode() os.FileMode { return os.ModeSymlink }

// BaseSuite is the base test suite for the service package.
type BaseSuite struct {
	testing.IsolationSuite

	Dirname string
	Name    string
	Conf    common.Conf
	Failure error

	Stub    *testing.Stub
	Service *svctesting.FakeService
	Patched *Stub
}

func (s *BaseSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.Dirname = c.MkDir()
	s.Name = "juju-agent-machine-0"
	s.Conf = common.Conf{
		Desc:      "some service",
		ExecStart: "/bin/jujud machine 0",
	}
	s.Failure = errors.New("<failed>")

	s.Service = svctesting.NewFakeService(s.Name, s.Conf)
	s.Stub = &s.Service.Stub
	s.Patched = &Stub{Stub: s.Stub}
	s.PatchValue(&discoverService, s.Patched.DiscoverService)
}

func (s *BaseSuite) PatchAttempts(retries int) {
	s.PatchValue(&installStartRetryAttempts, utils.AttemptStrategy{
		Min: retries,
	})
}

func (s *BaseSuite) PatchVersion(vers version.Binary) {
	s.Patched.Version = vers
	s.PatchValue(&getVersion, s.Patched.GetVersion)
}

func NewDiscoveryCheck(name string, running bool, failure error) discoveryCheck {
	return discoveryCheck{
		name: name,
		isRunning: func() (bool, error) {
			return running, failure
		},
	}
}

func (s *BaseSuite) PatchLocalDiscovery(checks ...discoveryCheck) {
	s.PatchValue(&discoveryFuncs, checks)
}

func (s *BaseSuite) PatchLocalDiscoveryDisable() {
	s.PatchLocalDiscovery()
}

func (s *BaseSuite) PatchLocalDiscoveryNoMatch(expected string) {
	// TODO(ericsnow) Pull from a list of supported init systems.
	names := []string{
		InitSystemUpstart,
		InitSystemSystemd,
		InitSystemWindows,
	}
	var checks []discoveryCheck
	for _, name := range names {
		checks = append(checks, NewDiscoveryCheck(name, name == expected, nil))
	}
	s.PatchLocalDiscovery(checks...)
}

func (s *BaseSuite) CheckFailure(c *gc.C, err error) {
	c.Check(errors.Cause(err), gc.Equals, s.Failure)
}
