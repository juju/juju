// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package initsystems_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/service/initsystems"
)

var _ = gc.Suite(&confSuite{})

type confSuite struct {
	testing.IsolationSuite

	conf *initsystems.Conf
}

func (s *confSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.conf = &initsystems.Conf{
		Desc: "an important service",
		Cmd:  "<do something>",
	}
}

func (s *confSuite) TestRepairNoop(c *gc.C) {
	origErr := errors.New("<unknown>")
	err := s.conf.Repair(origErr)

	c.Check(err, gc.Equals, origErr)
}

func (s *confSuite) TestRepairNotSupported(c *gc.C) {
	origErr := errors.NotSupportedf("<unknown>")
	err := s.conf.Repair(origErr)

	c.Check(err, gc.Equals, origErr)
}

func (s *confSuite) TestRepairUnknownField(c *gc.C) {
	errs := []error{
		initsystems.NewUnsupportedField("<unknown>"),
		initsystems.NewUnsupportedItem("<unknown>", "<a key>"),
	}

	for _, origErr := range errs {
		c.Logf("checking %v", origErr)
		err := s.conf.Repair(origErr)

		c.Check(err, gc.ErrorMatches, `reported unknown field "<unknown>" as unsupported: .*`)
		c.Check(errors.Cause(err), gc.Equals, initsystems.ErrBadInitSystemFailure)
	}
}

func (s *confSuite) TestRepairRequiredFields(c *gc.C) {
	for _, field := range []string{"Desc", "Cmd"} {
		c.Logf("checking %q", field)
		origErr := initsystems.NewUnsupportedField(field)
		err := s.conf.Repair(origErr)

		c.Check(err, gc.ErrorMatches, `reported required field "`+field+`" as unsupported: .*`)
		c.Check(errors.Cause(err), gc.Equals, initsystems.ErrBadInitSystemFailure)
	}
}

func (s *confSuite) TestRepairOut(c *gc.C) {
	s.conf.Out = "<some command>"

	origErr := initsystems.NewUnsupportedField("Out")
	err := s.conf.Repair(origErr)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.conf.Out, gc.Equals, "")
}

func (s *confSuite) TestRepairNotMap(c *gc.C) {
	for _, field := range []string{"Desc", "Cmd", "Out"} {
		c.Logf("checking %q", field)
		origErr := initsystems.NewUnsupportedItem(field, "spam")
		err := s.conf.Repair(origErr)

		c.Check(errors.Cause(err), gc.Equals, initsystems.ErrBadInitSystemFailure)
	}
}

func (s *confSuite) TestRepairEnv(c *gc.C) {
	s.conf.Env = map[string]string{
		"x": "y",
		"w": "z",
	}

	origErr := initsystems.NewUnsupportedItem("Env", "w")
	err := s.conf.Repair(origErr)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.conf.Env, jc.DeepEquals, map[string]string{"x": "y"})
}

func (s *confSuite) TestRepairEnvField(c *gc.C) {
	s.conf.Env = map[string]string{"x": "y"}

	origErr := initsystems.NewUnsupportedField("Env")
	err := s.conf.Repair(origErr)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.conf.Env, gc.IsNil)
}

func (s *confSuite) TestRepairLimit(c *gc.C) {
	s.conf.Limit = map[string]string{
		"x": "y",
		"w": "z",
	}

	origErr := initsystems.NewUnsupportedItem("Limit", "w")
	err := s.conf.Repair(origErr)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.conf.Limit, jc.DeepEquals, map[string]string{"x": "y"})
}

func (s *confSuite) TestRepairLimitField(c *gc.C) {
	s.conf.Limit = map[string]string{"x": "y"}

	origErr := initsystems.NewUnsupportedField("Limit")
	err := s.conf.Repair(origErr)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.conf.Limit, gc.IsNil)
}

func (s *confSuite) TestValidateValid(c *gc.C) {
	err := s.conf.Validate("jujud-machine-0")

	c.Check(err, jc.ErrorIsNil)
}

func (s *confSuite) TestValidateMissingName(c *gc.C) {
	err := s.conf.Validate("")

	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `missing name.*`)
}

func (s *confSuite) TestValidateMissingDesc(c *gc.C) {
	s.conf.Desc = ""
	err := s.conf.Validate("jujud-machine-0")

	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `missing Desc.*`)
}

func (s *confSuite) TestValidateMissingCmd(c *gc.C) {
	s.conf.Cmd = ""
	err := s.conf.Validate("jujud-machine-0")

	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `missing Cmd.*`)
}

/*
import (
	"os"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/fs"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/service/initsystems"
)

var _ = gc.Suite(&confDirSuite{})

type confDirSuite struct {
	initsystems.BaseSuite
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

func (s *confDirSuite) TestNewConfDirInfo(c *gc.C) {
	name := "jujud-machine-0"
	dirname := "/var/lib/juju/init"
	info := initsystems.NewConfDirInfo(name, dirname, "upstart")

	c.Check(info.DirName, gc.Equals, "/var/lib/juju/init/jujud-machine-0")
	c.Check(info.Name, gc.Equals, "jujud-machine-0")
	c.Check(info.InitSystem, gc.Equals, "upstart")
}

func (s *confDirSuite) TestConfDirInfoRemove(c *gc.C) {
	name := "jujud-machine-0"
	confDir := s.ConfDir(name)
	err := confDir.Remove(s.files.RemoveAll)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "RemoveAll")
}

func (s *confDirSuite) TestConfDirInfoRemoveNotExists(c *gc.C) {
	s.Files.SetErrors(os.NotExist)

	name := "jujud-machine-0"
	confDir := s.ConfDir(name)
	err := confDir.Remove(s.files.RemoveAll)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "RemoveAll")
}

func (s *confDirSuite) TestConfDirInfoRead(c *gc.C) {
}

func (s *confDirSuite) TestConfDirInfoPopulate(c *gc.C) {
}

func (s *confDirSuite) TestConfDirWrite(c *gc.C) {
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
*/
