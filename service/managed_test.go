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
		fops:       s.FakeFiles,
	})
}

func (s *managedSuite) TestNewConfigsPrefixes(c *gc.C) {
	configs := newConfigs(s.DataDir, InitSystemUpstart, "spam-")

	c.Check(configs, jc.DeepEquals, &serviceConfigs{
		baseDir:    "/var/lib/juju/init",
		initSystem: InitSystemUpstart,
		prefixes:   []string{"spam-"},
		names:      nil,
		fops:       s.FakeFiles,
	})
}

func (s *managedSuite) TestNewDir(c *gc.C) {
	newConfDir := s.configs.newDir("jujud-machine-0")

	c.Check(newConfDir, jc.DeepEquals, &confDir{
		dirname:    "/var/lib/juju/init/jujud-machine-0",
		initSystem: InitSystemUpstart,
		fops:       s.FakeFiles,
	})
}

func (s *managedSuite) TestRefresh(c *gc.C) {
	s.FakeFiles.Returns.Exists = true
	s.FakeFiles.Returns.DirEntries = []os.FileInfo{
		newFakeFile("an-errant-file", []byte("<data>")),
		newFakeDir("jujud-machine-0"),
		newFakeDir("an-errant-dir"),
		newFakeDir("jujud-unit-wordpress-0"),
	}

	err := s.configs.refresh()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.configs.names, jc.SameContents, []string{
		"jujud-machine-0",
		"jujud-unit-wordpress-0",
	})
}

func (s *managedSuite) TestRefreshIncomplete(c *gc.C) {
	s.FakeFiles.Returns.DirEntries = []os.FileInfo{
		newFakeFile("an-errant-file", []byte("<data>")),
		newFakeDir("jujud-machine-0"),
		newFakeDir("an-errant-dir"),
		newFakeDir("jujud-unit-wordpress-0"),
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
	s.FakeFiles.Returns.Exists = true
	s.FakeFiles.Returns.DirEntries = []os.FileInfo{
		newFakeFile("an-errant-file", []byte("<data>")),
		newFakeDir("jujud-machine-0"),
		newFakeDir("an-errant-dir"),
		newFakeDir("jujud-unit-wordpress-0"),
	}

	names, err := s.configs.list()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(names, jc.SameContents, []string{
		"jujud-machine-0",
		"jujud-unit-wordpress-0",
	})
}

func (s *managedSuite) TestLookup(c *gc.C) {
	s.configs.names = []string{"jujud-machine-0"}

	dir := s.configs.lookup("jujud-machine-0")

	c.Check(dir, jc.DeepEquals, &confDir{
		dirname:    "/var/lib/juju/init/jujud-machine-0",
		initSystem: InitSystemUpstart,
		fops:       s.FakeFiles,
	})
}

func (s *managedSuite) TestLookupNotFound(c *gc.C) {
	dir := s.configs.lookup("jujud-machine-0")

	c.Check(dir, gc.IsNil)
}

func (s *managedSuite) TestAddSuccess(c *gc.C) {
	s.FakeInit.Returns.Data = []byte("<upstart conf>")

	err := s.configs.add("jujud-machine-0", *s.Conf, s.FakeInit)
	c.Assert(err, jc.ErrorIsNil)

	s.Fake.CheckCalls(c, []testing.FakeCall{{
		FuncName: "Exists",
		Args: testing.FakeCallArgs{
			"name": "/var/lib/juju/init/jujud-machine-0",
		},
	}, {
		FuncName: "MkdirAll",
		Args: testing.FakeCallArgs{
			"dirname": "/var/lib/juju/init/jujud-machine-0",
			"perm":    os.FileMode(0755),
		},
	}, {
		FuncName: "Serialize",
		Args: testing.FakeCallArgs{
			"name": "jujud-machine-0",
			"conf": initsystems.Conf{
				Desc: "a service",
				Cmd:  "spam",
			},
		},
	}, {
		FuncName: "CreateFile",
		Args: testing.FakeCallArgs{
			"filename": "/var/lib/juju/init/jujud-machine-0/upstart.conf",
		},
	}, {
		FuncName: "Write",
		Args: testing.FakeCallArgs{
			"data": []byte("<upstart conf>"),
		},
	}, {
		FuncName: "Close",
	}, {
		FuncName: "Chmod",
		Args: testing.FakeCallArgs{
			"name": "/var/lib/juju/init/jujud-machine-0/upstart.conf",
			"perm": os.FileMode(0644),
		},
	}})
}

func (s *managedSuite) TestAddExists(c *gc.C) {
	s.configs.names = append(s.configs.names, "jujud-machine-0")

	err := s.configs.add("jujud-machine-0", *s.Conf, s.FakeInit)

	c.Check(err, jc.Satisfies, errors.IsAlreadyExists)
}

func (s *managedSuite) TestAddMultiline(c *gc.C) {
	s.Conf.Cmd = "spam\neggs"
	s.FakeInit.Returns.Data = []byte("<upstart conf>")

	err := s.configs.add("jujud-machine-0", *s.Conf, s.FakeInit)
	c.Assert(err, jc.ErrorIsNil)

	s.FakeFiles.CheckCalls(c, []testing.FakeCall{{
		FuncName: "Exists",
		Args: testing.FakeCallArgs{
			"name": "/var/lib/juju/init/jujud-machine-0",
		},
	}, {
		FuncName: "MkdirAll",
		Args: testing.FakeCallArgs{
			"dirname": "/var/lib/juju/init/jujud-machine-0",
			"perm":    os.FileMode(0755),
		},
	}, {
		FuncName: "CreateFile",
		Args: testing.FakeCallArgs{
			"filename": "/var/lib/juju/init/jujud-machine-0/script.sh",
		},
	}, {
		FuncName: "Write",
		Args: testing.FakeCallArgs{
			"data": []byte("spam\neggs"),
		},
	}, {
		FuncName: "Close",
	}, {
		FuncName: "Chmod",
		Args: testing.FakeCallArgs{
			"name": "/var/lib/juju/init/jujud-machine-0/script.sh",
			"perm": os.FileMode(0755),
		},
	}, {
		FuncName: "Serialize",
		Args: testing.FakeCallArgs{
			"name": "jujud-machine-0",
			"conf": initsystems.Conf{
				Desc: "a service",
				Cmd:  "/var/lib/juju/init/jujud-machine-0/script.sh",
			},
		},
	}, {
		FuncName: "CreateFile",
		Args: testing.FakeCallArgs{
			"filename": "/var/lib/juju/init/jujud-machine-0/upstart.conf",
		},
	}, {
		FuncName: "Write",
		Args: testing.FakeCallArgs{
			"data": []byte("<upstart conf>"),
		},
	}, {
		FuncName: "Close",
	}, {
		FuncName: "Chmod",
		Args: testing.FakeCallArgs{
			"name": "/var/lib/juju/init/jujud-machine-0/upstart.conf",
			"perm": os.FileMode(0644),
		},
	}})
}

func (s *managedSuite) TestAddExtra(c *gc.C) {
	s.Conf.ExtraScript = "eggs"
	s.FakeInit.Returns.Data = []byte("<upstart conf>")

	err := s.configs.add("jujud-machine-0", *s.Conf, s.FakeInit)
	c.Assert(err, jc.ErrorIsNil)

	s.FakeFiles.CheckCalls(c, []testing.FakeCall{{
		FuncName: "Exists",
		Args: testing.FakeCallArgs{
			"name": "/var/lib/juju/init/jujud-machine-0",
		},
	}, {
		FuncName: "MkdirAll",
		Args: testing.FakeCallArgs{
			"dirname": "/var/lib/juju/init/jujud-machine-0",
			"perm":    os.FileMode(0755),
		},
	}, {
		FuncName: "CreateFile",
		Args: testing.FakeCallArgs{
			"filename": "/var/lib/juju/init/jujud-machine-0/script.sh",
		},
	}, {
		FuncName: "Write",
		Args: testing.FakeCallArgs{
			"data": []byte("eggs\nspam"),
		},
	}, {
		FuncName: "Close",
	}, {
		FuncName: "Chmod",
		Args: testing.FakeCallArgs{
			"name": "/var/lib/juju/init/jujud-machine-0/script.sh",
			"perm": os.FileMode(0755),
		},
	}, {
		FuncName: "Serialize",
		Args: testing.FakeCallArgs{
			"name": "jujud-machine-0",
			"conf": initsystems.Conf{
				Desc: "a service",
				Cmd:  "/var/lib/juju/init/jujud-machine-0/script.sh",
			},
		},
	}, {
		FuncName: "CreateFile",
		Args: testing.FakeCallArgs{
			"filename": "/var/lib/juju/init/jujud-machine-0/upstart.conf",
		},
	}, {
		FuncName: "Write",
		Args: testing.FakeCallArgs{
			"data": []byte("<upstart conf>"),
		},
	}, {
		FuncName: "Close",
	}, {
		FuncName: "Chmod",
		Args: testing.FakeCallArgs{
			"name": "/var/lib/juju/init/jujud-machine-0/upstart.conf",
			"perm": os.FileMode(0644),
		},
	}})
}

func (s *managedSuite) TestRemove(c *gc.C) {
	s.configs.names = append(s.configs.names, "jujud-machine-0")

	err := s.configs.remove("jujud-machine-0")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.configs.names, gc.HasLen, 0)

	s.Fake.CheckCalls(c, []testing.FakeCall{{
		FuncName: "RemoveAll",
		Args: testing.FakeCallArgs{
			"name": "/var/lib/juju/init/jujud-machine-0",
		},
	}})
}

func (s *managedSuite) TestRemoveNotFound(c *gc.C) {
	err := s.configs.remove("jujud-machine-0")

	c.Check(err, jc.Satisfies, errors.IsNotFound)
}
