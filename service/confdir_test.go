// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"os"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/service/common"
)

var _ = gc.Suite(&confDirSuite{})

type confDirSuite struct {
	BaseSuite
}

func (s *confDirSuite) TestName(c *gc.C) {
	name := s.Confdir.name()

	c.Check(name, gc.Equals, "jujud-machine-0")
}

func (s *confDirSuite) TestConfName(c *gc.C) {
	confname := s.Confdir.confname()

	c.Check(confname, gc.Equals, "upstart.conf")
}

func (s *confDirSuite) TestFilename(c *gc.C) {
	filename := s.Confdir.filename()

	c.Check(filename, gc.Equals, "/var/lib/juju/init/jujud-machine-0/upstart.conf")
}

func (s *confDirSuite) TestValidate(c *gc.C) {
	err := s.Confdir.validate()
	c.Assert(err, jc.ErrorIsNil)

	s.FakeFiles.CheckCalls(c, []FakeCall{{
		FuncName: "Stat",
		Args: FakeCallArgs{
			"filename": "/var/lib/juju/init/jujud-machine-0/upstart.conf",
		},
	}})
}

func (s *confDirSuite) TestValidateMissingConf(c *gc.C) {
	s.FakeFiles.SetErrors(os.ErrNotExist)

	err := s.Confdir.validate()

	c.Check(err, gc.ErrorMatches, `.*missing conf file .*`)
}

func (s *confDirSuite) TestConf(c *gc.C) {
	s.FakeFiles.Data = []byte("<conf file contents>")

	content, err := s.Confdir.conf()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(content, gc.Equals, "<conf file contents>")
}

func (s *confDirSuite) TestScript(c *gc.C) {
	s.FakeFiles.Data = []byte("<script file contents>")

	content, err := s.Confdir.script()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(content, gc.Equals, "<script file contents>")
	c.Check(content, gc.Equals, "<script file contents>")
}

func (s *confDirSuite) TestWriteConf(c *gc.C) {
	s.FakeInit.Data = []byte("<upstart conf>")
	s.FakeFiles.File = s.FakeFiles
	s.FakeFiles.SetErrors(os.ErrNotExist)

	err := s.Confdir.writeConf(s.Conf, s.FakeInit)
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

func (s *confDirSuite) TestWriteConfExists(c *gc.C) {
	err := s.Confdir.writeConf(s.Conf, s.FakeInit)

	c.Check(err, jc.Satisfies, errors.IsAlreadyExists)
}

func (s *confDirSuite) TestWriteConfMultiline(c *gc.C) {
	s.Conf.Cmd = "spam\neggs"
	s.FakeInit.Data = []byte("<upstart conf>")
	s.FakeFiles.File = s.FakeFiles
	s.FakeFiles.SetErrors(os.ErrNotExist)

	err := s.Confdir.writeConf(s.Conf, s.FakeInit)
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

func (s *confDirSuite) TestWriteConfExtra(c *gc.C) {
	s.Conf.ExtraScript = "eggs"
	s.FakeInit.Data = []byte("<upstart conf>")
	s.FakeFiles.File = s.FakeFiles
	s.FakeFiles.SetErrors(os.ErrNotExist)

	err := s.Confdir.writeConf(s.Conf, s.FakeInit)
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

func (s *confDirSuite) TestRemove(c *gc.C) {
	err := s.Confdir.remove()
	c.Assert(err, jc.ErrorIsNil)

	s.FakeFiles.CheckCalls(c, []FakeCall{{
		FuncName: "RemoveAll",
		Args: FakeCallArgs{
			"dirname": "/var/lib/juju/init/jujud-machine-0",
		},
	}})
}
