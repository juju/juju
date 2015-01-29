// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

var _ = gc.Suite(&managedSuite{})

type managedSuite struct {
	BaseSuite
}

func (s *managedSuite) TestNewConfigs(c *gc.C) {
	configs := newConfigs(s.DataDir, InitSystemUpstart)

	// TODO(ericsnow) Fix the fragile test order of prefixes.
	c.Check(configs, jc.DeepEquals, &serviceConfigs{
		baseDir:    "/var/lib/juju/init",
		initSystem: InitSystemUpstart,
		prefixes:   []string{"juju-", "jujud-"},
		names:      nil,
	})
}

func (s *managedSuite) TestNewDir(c *gc.C) {
}

func (s *managedSuite) TestRefresh(c *gc.C) {
}

func (s *managedSuite) TestList(c *gc.C) {
}

func (s *managedSuite) TestLookup(c *gc.C) {
}

func (s *managedSuite) TestAdd(c *gc.C) {
}

func (s *managedSuite) TestRemove(c *gc.C) {
}

/*
func (s *managedSuite) TestWriteConf(c *gc.C) {
	s.FakeFiles.File = s.FakeFiles

	data := []byte("<upstart conf>")
	err := s.Confdir.writeConf(data)
	c.Assert(err, jc.ErrorIsNil)

	s.FakeFiles.CheckCalls(c, []FakeCall{{
		FuncName: "CreateFile",
		Args: FakeCallArgs{
			"filename": "/var/lib/juju/init/jujud-machine-0/upstart.conf",
		},
	}, {
		FuncName: "Write",
		Args: FakeCallArgs{
			"data": []byte("<upstart conf>"),
		},
	}, {
		FuncName: "Close",
	}, {
		FuncName: "Chmod",
		Args: FakeCallArgs{
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

	s.FakeFiles.CheckCalls(c, []FakeCall{{
		FuncName: "Stat",
		Args: FakeCallArgs{
			"filename": "/var/lib/juju/init/jujud-machine-0",
		},
	}, {
		FuncName: "Create",
		Args: FakeCallArgs{
			"filename": "/var/lib/juju/init/jujud-machine-0/upstart.conf",
		},
	}, {
		FuncName: "Write",
		Args: FakeCallArgs{
			"data": []byte("<upstart conf>"),
		},
	}, {
		FuncName: "Close",
	}})
	s.FakeInit.CheckCalls(c, []FakeCall{{
		FuncName: "Serialize",
		Args: FakeCallArgs{
			"conf": &common.Conf{
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

	s.FakeFiles.CheckCalls(c, []FakeCall{{
		FuncName: "Stat",
		Args: FakeCallArgs{
			"filename": "/var/lib/juju/init/jujud-machine-0",
		},
	}, {
		FuncName: "Create",
		Args: FakeCallArgs{
			"filename": "/var/lib/juju/init/jujud-machine-0/script.sh",
		},
	}, {
		FuncName: "Write",
		Args: FakeCallArgs{
			"data": []byte("spam\neggs"),
		},
	}, {
		FuncName: "Close",
	}, {
		FuncName: "Create",
		Args: FakeCallArgs{
			"filename": "/var/lib/juju/init/jujud-machine-0/upstart.conf",
		},
	}, {
		FuncName: "Write",
		Args: FakeCallArgs{
			"data": []byte("<upstart conf>"),
		},
	}, {
		FuncName: "Close",
	}})
	s.FakeInit.CheckCalls(c, []FakeCall{{
		FuncName: "Serialize",
		Args: FakeCallArgs{
			"conf": &common.Conf{
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

	s.FakeFiles.CheckCalls(c, []FakeCall{{
		FuncName: "Stat",
		Args: FakeCallArgs{
			"filename": "/var/lib/juju/init/jujud-machine-0",
		},
	}, {
		FuncName: "Create",
		Args: FakeCallArgs{
			"filename": "/var/lib/juju/init/jujud-machine-0/script.sh",
		},
	}, {
		FuncName: "Write",
		Args: FakeCallArgs{
			"data": []byte("eggs\nspam"),
		},
	}, {
		FuncName: "Close",
	}, {
		FuncName: "Create",
		Args: FakeCallArgs{
			"filename": "/var/lib/juju/init/jujud-machine-0/upstart.conf",
		},
	}, {
		FuncName: "Write",
		Args: FakeCallArgs{
			"data": []byte("<upstart conf>"),
		},
	}, {
		FuncName: "Close",
	}})
	s.FakeInit.CheckCalls(c, []FakeCall{{
		FuncName: "Serialize",
		Args: FakeCallArgs{
			"conf": &common.Conf{
				Desc: "a service",
				Cmd:  "/var/lib/juju/init/jujud-machine-0/script.sh",
			},
		},
	}})
}
*/
