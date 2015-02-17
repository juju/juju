// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"os"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/service/initsystems"
)

var _ = gc.Suite(&confDirSuite{})

type confDirSuite struct {
	BaseSuite
}

func (s *confDirSuite) checkWritten(c *gc.C, filename, content string, perm os.FileMode) {
	s.StubFiles.CheckCalls(c, []testing.StubCall{{
		FuncName: "CreateFile",
		Args: []interface{}{
			filename,
		},
	}, {
		FuncName: "Write",
		Args: []interface{}{
			[]byte(content),
		},
	}, {
		FuncName: "Close",
	}, {
		FuncName: "Chmod",
		Args: []interface{}{
			filename,
			perm,
		},
	}})
}

func (s *confDirSuite) TestName(c *gc.C) {
	name := s.Confdir.name()

	c.Check(name, gc.Equals, "jujud-machine-0")
}

func (s *confDirSuite) TestConfName(c *gc.C) {
	confName := s.Confdir.confName()

	c.Check(confName, gc.Equals, "upstart.conf")
}

func (s *confDirSuite) TestFilename(c *gc.C) {
	filename := s.Confdir.filename()

	c.Check(filename, gc.Equals, "/var/lib/juju/init/jujud-machine-0/upstart.conf")
}

func (s *confDirSuite) TestValidate(c *gc.C) {
	s.StubFiles.Returns.Exists = true

	err := s.Confdir.validate()
	c.Assert(err, jc.ErrorIsNil)

	s.StubFiles.CheckCalls(c, []testing.StubCall{{
		FuncName: "Exists",
		Args: []interface{}{
			"/var/lib/juju/init/jujud-machine-0/upstart.conf",
		},
	}})
}

func (s *confDirSuite) TestValidateMissingConf(c *gc.C) {
	s.StubFiles.SetErrors(os.ErrNotExist)

	err := s.Confdir.validate()

	c.Check(err, gc.ErrorMatches, `.*missing conf file .*`)
}

func (s *confDirSuite) TestCreate(c *gc.C) {
	err := s.Confdir.create()
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
	}})
}

func (s *confDirSuite) TestConf(c *gc.C) {
	s.StubFiles.Returns.Data = []byte("<conf file contents>")

	content, err := s.Confdir.conf()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(string(content), gc.Equals, "<conf file contents>")
}

func (s *confDirSuite) TestScript(c *gc.C) {
	s.StubFiles.Returns.Data = []byte("<script file contents>")

	content, err := s.Confdir.script()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(string(content), gc.Equals, "<script file contents>")
}

func (s *confDirSuite) TestWriteConf(c *gc.C) {
	content := "<upstart conf>"
	err := s.Confdir.writeConf([]byte(content))
	c.Assert(err, jc.ErrorIsNil)

	expected := "/var/lib/juju/init/jujud-machine-0/upstart.conf"
	s.checkWritten(c, expected, content, 0644)
}

func (s *confDirSuite) TestNormalizeConf(c *gc.C) {
	s.Conf.ExtraScript = "<preceding command>"

	conf, err := s.Confdir.normalizeConf(*s.Conf)
	c.Assert(err, jc.ErrorIsNil)

	expected := "/var/lib/juju/init/jujud-machine-0/script.sh"
	c.Check(conf, jc.DeepEquals, &initsystems.Conf{
		Desc: "a service",
		Cmd:  expected,
	})
	script := "<preceding command>\nspam"
	s.checkWritten(c, expected, script, 0755)
}

func (s *confDirSuite) TestNormalizeConfNop(c *gc.C) {
	conf, err := s.Confdir.normalizeConf(*s.Conf)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(conf, jc.DeepEquals, &initsystems.Conf{
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

	expected := "/var/lib/juju/init/jujud-machine-0/script.sh"
	c.Check(filename, gc.Equals, expected)
	s.checkWritten(c, expected, script, 0755)
}

func (s *confDirSuite) TestRemove(c *gc.C) {
	err := s.Confdir.remove()
	c.Assert(err, jc.ErrorIsNil)

	s.StubFiles.CheckCalls(c, []testing.StubCall{{
		FuncName: "RemoveAll",
		Args: []interface{}{
			"/var/lib/juju/init/jujud-machine-0",
		},
	}})
}
