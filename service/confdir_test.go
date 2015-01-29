// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"os"

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
	s.FakeFiles.Exists = true

	err := s.Confdir.validate()
	c.Assert(err, jc.ErrorIsNil)

	s.FakeFiles.CheckCalls(c, []FakeCall{{
		FuncName: "Exists",
		Args: FakeCallArgs{
			"name": "/var/lib/juju/init/jujud-machine-0/upstart.conf",
		},
	}})
}

func (s *confDirSuite) TestValidateMissingConf(c *gc.C) {
	s.FakeFiles.SetErrors(os.ErrNotExist)

	err := s.Confdir.validate()

	c.Check(err, gc.ErrorMatches, `.*missing conf file .*`)
}

func (s *confDirSuite) TestCreate(c *gc.C) {
	err := s.Confdir.create()
	c.Assert(err, jc.ErrorIsNil)

	s.FakeFiles.CheckCalls(c, []FakeCall{{
		FuncName: "Exists",
		Args: FakeCallArgs{
			"name": "/var/lib/juju/init/jujud-machine-0",
		},
	}, {
		FuncName: "MkdirAll",
		Args: FakeCallArgs{
			"dirname": "/var/lib/juju/init/jujud-machine-0",
			"mode":    os.FileMode(0755),
		},
	}})
}

func (s *confDirSuite) TestConf(c *gc.C) {
	s.FakeFiles.Data = []byte("<conf file contents>")

	content, err := s.Confdir.conf()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(string(content), gc.Equals, "<conf file contents>")
}

func (s *confDirSuite) TestScript(c *gc.C) {
	s.FakeFiles.Data = []byte("<script file contents>")

	content, err := s.Confdir.script()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(string(content), gc.Equals, "<script file contents>")
}

func (s *confDirSuite) TestWriteConf(c *gc.C) {
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
			"data": data,
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

func (s *confDirSuite) TestNormalizeConf(c *gc.C) {
	s.Conf.ExtraScript = "<preceding command>"

	conf, err := s.Confdir.normalizeConf(*s.Conf)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(conf, jc.DeepEquals, &common.Conf{
		Desc: "a service",
		Cmd:  "/var/lib/juju/init/jujud-machine-0/script.sh",
	})
	s.FakeFiles.CheckCalls(c, []FakeCall{{
		FuncName: "CreateFile",
		Args: FakeCallArgs{
			"filename": "/var/lib/juju/init/jujud-machine-0/script.sh",
		},
	}, {
		FuncName: "Write",
		Args: FakeCallArgs{
			"data": []byte("<preceding command>\nspam"),
		},
	}, {
		FuncName: "Close",
	}, {
		FuncName: "Chmod",
		Args: FakeCallArgs{
			"name": "/var/lib/juju/init/jujud-machine-0/script.sh",
			"mode": os.FileMode(0755),
		},
	}})
}

func (s *confDirSuite) TestNormalizeConfNop(c *gc.C) {
	conf, err := s.Confdir.normalizeConf(*s.Conf)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(conf, jc.DeepEquals, &common.Conf{
		Desc: "a service",
		Cmd:  "spam",
	})
}

func (s *confDirSuite) TestIsSimpleScript(c *gc.C) {
	simple := s.Confdir.isSimpleScript("<a script>")

	c.Check(simple, jc.IsTrue)
}

func (s *confDirSuite) TestIsSimpleScriptMultiline(c *gc.C) {
	simple := s.Confdir.isSimpleScript("<a script>\n<more script>")

	c.Check(simple, jc.IsFalse)
}

func (s *confDirSuite) TestWriteScript(c *gc.C) {
	script := "<command script>"
	filename, err := s.Confdir.writeScript(script)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(filename, gc.Equals, "/var/lib/juju/init/jujud-machine-0/script.sh")
	s.FakeFiles.CheckCalls(c, []FakeCall{{
		FuncName: "CreateFile",
		Args: FakeCallArgs{
			"filename": "/var/lib/juju/init/jujud-machine-0/script.sh",
		},
	}, {
		FuncName: "Write",
		Args: FakeCallArgs{
			"data": []byte(script),
		},
	}, {
		FuncName: "Close",
	}, {
		FuncName: "Chmod",
		Args: FakeCallArgs{
			"name": "/var/lib/juju/init/jujud-machine-0/script.sh",
			"mode": os.FileMode(0755),
		},
	}})
}

func (s *confDirSuite) TestRemove(c *gc.C) {
	err := s.Confdir.remove()
	c.Assert(err, jc.ErrorIsNil)

	s.FakeFiles.CheckCalls(c, []FakeCall{{
		FuncName: "RemoveAll",
		Args: FakeCallArgs{
			"name": "/var/lib/juju/init/jujud-machine-0",
		},
	}})
}
