// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"os"

	//"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
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

	// TODO(ericsnow) Fix the fragile test order of prefixes.
	c.Check(configs, jc.DeepEquals, &serviceConfigs{
		baseDir:    "/var/lib/juju/init",
		initSystem: InitSystemUpstart,
		prefixes:   []string{"juju-", "jujud-"},
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

func (s *managedSuite) TestAdd(c *gc.C) {
	// TODO(ericsnow) Finish!
}

func (s *managedSuite) TestRemove(c *gc.C) {
	// TODO(ericsnow) Finish!
}

/*
func (s *managedSuite) TestWriteConf(c *gc.C) {
	s.FakeFiles.File = s.FakeFiles

	data := []byte("<upstart conf>")
	err := s.Confdir.writeConf(data)
	c.Assert(err, jc.ErrorIsNil)

	s.FakeFiles.CheckCalls(c, []testing.FakeCall{{
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
			"mode": os.FileMode(0644),
		},
	}})
}

func (s *managedSuite) TestWriteConf(c *gc.C) {
	s.FakeInit.Data = []byte("<upstart conf>")
	s.FakeFiles.File = s.FakeFiles
	s.FakeFiles.SetErrors(os.ErrNotExist)

	data := []byte("<upstart conf>")
	c.Assert(err, jc.ErrorIsNil)
	err = s.Confdir.writeConf(data)
	c.Assert(err, jc.ErrorIsNil)

	s.FakeFiles.CheckCalls(c, []testing.FakeCall{{
		FuncName: "Stat",
		Args: testing.FakeCallArgs{
			"filename": "/var/lib/juju/init/jujud-machine-0",
		},
	}, {
		FuncName: "Create",
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
	}})
	s.FakeInit.CheckCalls(c, []testing.FakeCall{{
		FuncName: "Serialize",
		Args: testing.FakeCallArgs{
			"conf": &initsystems.Conf{
				Desc: "a service",
				Cmd:  "spam",
			},
		},
	}})
}

func (s *managedSuite) TestWriteConfExists(c *gc.C) {
	data, err := s.FakeInit.Serialize(s.Conf)
	c.Assert(err, jc.ErrorIsNil)
	err = s.Confdir.writeConf(data)

	c.Check(err, jc.Satisfies, errors.IsAlreadyExists)
}

func (s *managedSuite) TestWriteConfMultiline(c *gc.C) {
	s.Conf.Cmd = "spam\neggs"
	s.FakeInit.Data = []byte("<upstart conf>")
	s.FakeFiles.File = s.FakeFiles
	//s.FakeFiles.SetErrors(os.ErrNotExist)

	data, err := s.FakeInit.Serialize(s.Conf)
	c.Assert(err, jc.ErrorIsNil)
	err = s.Confdir.writeConf(data)
	c.Assert(err, jc.ErrorIsNil)

	s.FakeFiles.CheckCalls(c, []testing.FakeCall{{
		FuncName: "Stat",
		Args: testing.FakeCallArgs{
			"filename": "/var/lib/juju/init/jujud-machine-0",
		},
	}, {
		FuncName: "Create",
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
		FuncName: "Create",
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
	}})
	s.FakeInit.CheckCalls(c, []testing.FakeCall{{
		FuncName: "Serialize",
		Args: testing.FakeCallArgs{
			"conf": &initsystems.Conf{
				Desc: "a service",
				Cmd:  "/var/lib/juju/init/jujud-machine-0/script.sh",
			},
		},
	}})
}

func (s *managedSuite) TestWriteConfExtra(c *gc.C) {
	s.Conf.ExtraScript = "eggs"
	s.FakeInit.Data = []byte("<upstart conf>")
	s.FakeFiles.File = s.FakeFiles
	s.FakeFiles.SetErrors(os.ErrNotExist)

	data, err := s.FakeInit.Serialize(s.Conf)
	c.Assert(err, jc.ErrorIsNil)
	err = s.Confdir.writeConf(data)
	c.Assert(err, jc.ErrorIsNil)

	s.FakeFiles.CheckCalls(c, []testing.FakeCall{{
		FuncName: "Stat",
		Args: testing.FakeCallArgs{
			"filename": "/var/lib/juju/init/jujud-machine-0",
		},
	}, {
		FuncName: "Create",
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
		FuncName: "Create",
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
	}})
	s.FakeInit.CheckCalls(c, []testing.FakeCall{{
		FuncName: "Serialize",
		Args: testing.FakeCallArgs{
			"conf": &initsystems.Conf{
				Desc: "a service",
				Cmd:  "/var/lib/juju/init/jujud-machine-0/script.sh",
			},
		},
	}})
}
*/
