// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"os"

	"github.com/juju/testing"
	"github.com/juju/utils/fs"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/service/initsystems"
)

type BaseSuite struct {
	testing.IsolationSuite

	DataDir string
	LogDir  string
	Paths   AgentPaths
	Conf    Conf
	ConfDir initsystems.ConfDirInfo

	Stub  *testing.Stub
	Init  *initsystems.Stub
	File  *fs.StubFile
	Files *fs.StubOps
}

func (s *BaseSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.DataDir = "/var/lib/juju"
	s.LogDir = "/var/log/juju"
	s.Paths = NewAgentPaths(s.DataDir, s.LogDir)
	s.Conf = Conf{Conf: initsystems.Conf{
		Desc: "a service",
		Cmd:  "spam",
	}}

	// In the context of the `service` package, the particular init
	// system is not significant. Nothing in the package should rely on
	// any specific init system. So here we simply picked one.
	initName := InitSystemUpstart

	// Patch a few things.
	s.Stub = &testing.Stub{}
	s.Init = &initsystems.Stub{Stub: s.Stub}
	s.Init.Returns.Name = initName
	s.File = &fs.StubFile{Stub: s.Stub}
	s.Files = &fs.StubOps{Stub: s.Stub}
	s.Files.Returns.File = s.File

	s.PatchValue(&newFileOps, func() fs.Operations {
		return s.Files
	})

	name := "jujud-machine-0"
	initDir := s.DataDir + "/init"
	s.ConfDir = initsystems.NewConfDirInfo(name, initDir, initName)
}

func (s *BaseSuite) SetManaged(name string, services *Services) {
	services.configs.names = append(services.configs.names, name)
}

const (
	ConfDirReadCalls       = "<ConfDir.Read>"
	ConfDirPopulateCalls   = "<ConfDir.Populate>"
	ConfDirWriteCalls      = "<ConfDir.Write>"
	ConfDirCreateFileCalls = "<ConfDir.CreateFile>"
	ManageCalls            = "<serviceConfigs.add>"
)

func (s *BaseSuite) appendCallName(names []string, name string) []string {
	switch name {
	case ConfDirReadCalls:
		names = append(names, []string{
			"ReadFile",
			"ReadFile",
			"ListDir",
		}...)
	case ConfDirPopulateCalls:
		names = append(names, []string{
			"Name",
			"Validate",
			"Validate",
			"Serialize",
		}...)
	case ConfDirCreateFileCalls:
		names = append(names, []string{
			"CreateFile",
			"Write",
			"Close",
		}...)
	case ConfDirWriteCalls:
		names = append(names, "MkdirAll")
		names = s.appendCallName(names, ConfDirCreateFileCalls)
		names = s.appendCallName(names, ConfDirCreateFileCalls)
	case ManageCalls:
		names = s.appendCallName(names, ConfDirPopulateCalls)
		names = s.appendCallName(names, ConfDirWriteCalls)
	default:
		names = append(names, name)
	}
	return names
}

func (s *BaseSuite) CheckCallNames(c *gc.C, names ...string) {
	var expanded []string
	for _, name := range names {
		expanded = s.appendCallName(expanded, name)
	}
	s.Stub.CheckCallNames(c, expanded...)
}

func newStubFile(name string, data []byte) os.FileInfo {
	return fs.NewFile(name, 0644, data)
}

func newStubDir(name string) os.FileInfo {
	return fs.NewDir(name, 0755)
}

type agentPaths struct {
	dataDir string
	logDir  string
}

// NewAgentPaths returns a new AgentPaths based on the given info.
func NewAgentPaths(dataDir, logDir string) AgentPaths {
	return &agentPaths{
		dataDir: dataDir,
		logDir:  logDir,
	}
}

// DataDir implements AgentPaths.
func (ap agentPaths) DataDir() string {
	return ap.dataDir
}

// LogDir implements AgentPaths.
func (ap agentPaths) LogDir() string {
	return ap.logDir
}
