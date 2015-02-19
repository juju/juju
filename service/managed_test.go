// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"os"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/service/initsystems"
)

var _ = gc.Suite(&managedSuite{})

type managedSuite struct {
	BaseSuite

	configs *serviceConfigs
}

func (s *managedSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.configs = newConfigs(s.DataDir, InitSystemUpstart)
}

func (s *managedSuite) TestNewConfigs(c *gc.C) {
	configs := newConfigs(s.DataDir, InitSystemUpstart)

	c.Check(configs, jc.DeepEquals, &serviceConfigs{
		baseDir:    "/var/lib/juju/init",
		initSystem: InitSystemUpstart,
		prefixes:   []string{"juju-", "jujud-"},
		names:      nil,
		fops:       s.StubFiles,
	})
}

func (s *managedSuite) TestNewConfigsPrefixes(c *gc.C) {
	configs := newConfigs(s.DataDir, InitSystemUpstart, "spam-")

	c.Check(configs, jc.DeepEquals, &serviceConfigs{
		baseDir:    "/var/lib/juju/init",
		initSystem: InitSystemUpstart,
		prefixes:   []string{"spam-"},
		names:      nil,
		fops:       s.StubFiles,
	})
}

func (s *managedSuite) TestRefresh(c *gc.C) {
	s.StubFiles.Returns.Exists = true
	s.StubFiles.Returns.DirEntries = []os.FileInfo{
		newStubFile("an-errant-file", []byte("<data>")),
		newStubDir("jujud-machine-0"),
		newStubDir("an-errant-dir"),
		newStubDir("jujud-unit-wordpress-0"),
	}

	err := s.configs.refresh()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.configs.names, jc.SameContents, []string{
		"jujud-machine-0",
		"jujud-unit-wordpress-0",
	})
}

func (s *managedSuite) TestRefreshIncomplete(c *gc.C) {
	s.StubFiles.Returns.DirEntries = []os.FileInfo{
		newStubFile("an-errant-file", []byte("<data>")),
		newStubDir("jujud-machine-0"),
		newStubDir("an-errant-dir"),
		newStubDir("jujud-unit-wordpress-0"),
	}

	err := s.configs.refresh()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.configs.names, jc.SameContents, []string{})
}

func (s *managedSuite) TestRefreshEmpty(c *gc.C) {
	err := s.configs.refresh()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.configs.names, jc.SameContents, []string{})
}

func (s *managedSuite) TestList(c *gc.C) {
	s.StubFiles.Returns.Exists = true
	s.StubFiles.Returns.DirEntries = []os.FileInfo{
		newStubFile("an-errant-file", []byte("<data>")),
		newStubDir("jujud-machine-0"),
		newStubDir("an-errant-dir"),
		newStubDir("jujud-unit-wordpress-0"),
	}

	names, err := s.configs.list()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(names, jc.SameContents, []string{
		"jujud-machine-0",
		"jujud-unit-wordpress-0",
	})
}

func (s *managedSuite) TestLookup(c *gc.C) {
	name := "jujud-machine-0"
	s.configs.names = []string{name}

	dir := s.configs.lookup(name)

	expected := initsystems.NewConfDirInfo(name, s.DataDir+"/init", InitSystemUpstart)
	c.Check(dir, jc.DeepEquals, expected)
}

func (s *managedSuite) TestLookupNotFound(c *gc.C) {
	dir := s.configs.lookup("jujud-machine-0")

	c.Check(dir, gc.IsNil)
}

func (s *managedSuite) TestAddSuccess(c *gc.C) {
	s.StubInit.Returns.Data = []byte("<upstart conf>")

	err := s.configs.add("jujud-machine-0", s.Conf, s.StubInit)
	c.Assert(err, jc.ErrorIsNil)

	s.Stub.CheckCalls(c, []testing.StubCall{{
		FuncName: "Exists",
		Args: []interface{}{
			"/var/lib/juju/init/jujud-machine-0",
		},
	}, {
		FuncName: "MkdirAll",
		Args: []interface{}{
			"/var/lib/juju/init/jujud-machine-0",
			os.FileMode(0755),
		},
	}, {
		FuncName: "Serialize",
		Args: []interface{}{
			"jujud-machine-0",
			initsystems.Conf{
				Desc: "a service",
				Cmd:  "spam",
			},
		},
	}, {
		FuncName: "CreateFile",
		Args: []interface{}{
			"/var/lib/juju/init/jujud-machine-0/upstart.conf",
		},
	}, {
		FuncName: "Write",
		Args: []interface{}{
			[]byte("<upstart conf>"),
		},
	}, {
		FuncName: "Close",
	}, {
		FuncName: "Chmod",
		Args: []interface{}{
			"/var/lib/juju/init/jujud-machine-0/upstart.conf",
			os.FileMode(0644),
		},
	}})
}

func (s *managedSuite) TestAddExists(c *gc.C) {
	s.configs.names = append(s.configs.names, "jujud-machine-0")

	err := s.configs.add("jujud-machine-0", s.Conf, s.StubInit)

	c.Check(err, jc.Satisfies, errors.IsAlreadyExists)
}

func (s *managedSuite) TestAddMultiline(c *gc.C) {
	s.Conf.Cmd = "spam\neggs"
	s.StubInit.Returns.Data = []byte("<upstart conf>")

	err := s.configs.add("jujud-machine-0", s.Conf, s.StubInit)
	c.Assert(err, jc.ErrorIsNil)

	s.StubFiles.CheckCalls(c, []testing.StubCall{{
		FuncName: "Exists",
		Args: []interface{}{
			"/var/lib/juju/init/jujud-machine-0",
		},
	}, {
		FuncName: "MkdirAll",
		Args: []interface{}{
			"/var/lib/juju/init/jujud-machine-0",
			os.FileMode(0755),
		},
	}, {
		FuncName: "CreateFile",
		Args: []interface{}{
			"/var/lib/juju/init/jujud-machine-0/script.sh",
		},
	}, {
		FuncName: "Write",
		Args: []interface{}{
			[]byte("spam\neggs"),
		},
	}, {
		FuncName: "Close",
	}, {
		FuncName: "Chmod",
		Args: []interface{}{
			"/var/lib/juju/init/jujud-machine-0/script.sh",
			os.FileMode(0755),
		},
	}, {
		FuncName: "Serialize",
		Args: []interface{}{
			"jujud-machine-0",
			initsystems.Conf{
				Desc: "a service",
				Cmd:  "/var/lib/juju/init/jujud-machine-0/script.sh",
			},
		},
	}, {
		FuncName: "CreateFile",
		Args: []interface{}{
			"/var/lib/juju/init/jujud-machine-0/upstart.conf",
		},
	}, {
		FuncName: "Write",
		Args: []interface{}{
			[]byte("<upstart conf>"),
		},
	}, {
		FuncName: "Close",
	}, {
		FuncName: "Chmod",
		Args: []interface{}{
			"/var/lib/juju/init/jujud-machine-0/upstart.conf",
			os.FileMode(0644),
		},
	}})
}

func (s *managedSuite) TestAddExtra(c *gc.C) {
	s.Conf.ExtraScript = "eggs"
	s.StubInit.Returns.Data = []byte("<upstart conf>")

	err := s.configs.add("jujud-machine-0", s.Conf, s.StubInit)
	c.Assert(err, jc.ErrorIsNil)

	s.StubFiles.CheckCalls(c, []testing.StubCall{{
		FuncName: "Exists",
		Args: []interface{}{
			"/var/lib/juju/init/jujud-machine-0",
		},
	}, {
		FuncName: "MkdirAll",
		Args: []interface{}{
			"/var/lib/juju/init/jujud-machine-0",
			os.FileMode(0755),
		},
	}, {
		FuncName: "CreateFile",
		Args: []interface{}{
			"/var/lib/juju/init/jujud-machine-0/script.sh",
		},
	}, {
		FuncName: "Write",
		Args: []interface{}{
			[]byte("eggs\nspam"),
		},
	}, {
		FuncName: "Close",
	}, {
		FuncName: "Chmod",
		Args: []interface{}{
			"/var/lib/juju/init/jujud-machine-0/script.sh",
			os.FileMode(0755),
		},
	}, {
		FuncName: "Serialize",
		Args: []interface{}{
			"jujud-machine-0",
			initsystems.Conf{
				Desc: "a service",
				Cmd:  "/var/lib/juju/init/jujud-machine-0/script.sh",
			},
		},
	}, {
		FuncName: "CreateFile",
		Args: []interface{}{
			"/var/lib/juju/init/jujud-machine-0/upstart.conf",
		},
	}, {
		FuncName: "Write",
		Args: []interface{}{
			[]byte("<upstart conf>"),
		},
	}, {
		FuncName: "Close",
	}, {
		FuncName: "Chmod",
		Args: []interface{}{
			"/var/lib/juju/init/jujud-machine-0/upstart.conf",
			os.FileMode(0644),
		},
	}})
}

func (s *managedSuite) TestRemove(c *gc.C) {
	s.configs.names = append(s.configs.names, "jujud-machine-0")

	err := s.configs.remove("jujud-machine-0")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.configs.names, gc.HasLen, 0)

	s.Stub.CheckCalls(c, []testing.StubCall{{
		FuncName: "RemoveAll",
		Args: []interface{}{
			"/var/lib/juju/init/jujud-machine-0",
		},
	}})
}

func (s *managedSuite) TestRemoveNotFound(c *gc.C) {
	err := s.configs.remove("jujud-machine-0")

	c.Check(err, jc.Satisfies, errors.IsNotFound)
}
