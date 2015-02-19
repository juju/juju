// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package initsystems_test

import (
	"os"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/fs"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/service/initsystems"
)

var _ = gc.Suite(&confDirSuite{})

type confDirSuite struct {
	initsystems.BaseSuite
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
	info := s.ConfDirInfo(name)
	err := info.Remove(s.Files.RemoveAll)
	c.Assert(err, jc.ErrorIsNil)

	s.Stub.CheckCallNames(c, "RemoveAll")
}

func (s *confDirSuite) TestConfDirInfoRemoveNotExists(c *gc.C) {
	s.Files.SetErrors(os.ErrNotExist)

	name := "jujud-machine-0"
	info := s.ConfDirInfo(name)
	err := info.Remove(s.Files.RemoveAll)
	c.Assert(err, jc.ErrorIsNil)

	s.Stub.CheckCallNames(c, "RemoveAll")
}

func (s *confDirSuite) TestConfDirInfoRead(c *gc.C) {
	content := `
{
    "description": "a service",
    "startexec": "spam",
    "name": "jujud-machine-0",
    "confname": "jujud-machine-0.conf"
}`[1:]
	s.Files.Returns.Data = []byte(content)

	name := "jujud-machine-0"
	info := s.ConfDirInfo(name)
	confDir, err := info.Read(s.Files)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(confDir.DirName, gc.Equals, "/var/lib/juju/init/jujud-machine-0")
	c.Check(confDir.Name, gc.Equals, "jujud-machine-0")
	c.Check(confDir.InitSystem, gc.Equals, "upstart")
	c.Check(confDir.ConfName, gc.Equals, "jujud-machine-0.conf")
	c.Check(confDir.Conf, jc.DeepEquals, s.Conf)
	c.Check(confDir.ConfFile.FileName, gc.Equals, "jujud-machine-0.conf")
	c.Check(confDir.ConfFile.Mode, gc.Equals, os.FileMode(0))
	c.Check(string(confDir.ConfFile.Data), gc.Equals, content)
	c.Check(confDir.Files, gc.HasLen, 0)
	s.Stub.CheckCallNames(c, "ReadFile", "ReadFile", "ListDir")
}

func (s *confDirSuite) TestConfDirInfoReadScript(c *gc.C) {
	content := "{}"
	s.Files.Returns.Data = []byte("{}")
	s.Files.Returns.DirEntries = []os.FileInfo{
		fs.NewFile("script.sh", 0755, nil),
		fs.NewFile("extra.txt", 0, nil),
	}

	name := "jujud-machine-0"
	info := s.ConfDirInfo(name)
	confDir, err := info.Read(s.Files)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(string(confDir.ConfFile.Data), gc.Equals, content)
	c.Check(confDir.Files, jc.DeepEquals, []initsystems.FileData{{
		FileName: "script.sh",
		Mode:     0755,
		Data:     []byte("{}"),
	}, {
		FileName: "extra.txt",
		Data:     []byte("{}"),
	}})
	s.Stub.CheckCallNames(c, "ReadFile", "ReadFile", "ListDir", "ReadFile", "ReadFile")
}

func (s *confDirSuite) TestConfDirInfoPopulate(c *gc.C) {
	content := "<some data>"
	s.Init.Returns.ConfName = "jujud-machine-0.conf"
	s.Init.Returns.Data = []byte(content)
	s.Init.Returns.Name = "upstart"

	name := "jujud-machine-0"
	info := s.ConfDirInfo(name)
	confDir, err := info.Populate(s.Conf, s.Init)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(confDir.DirName, gc.Equals, "/var/lib/juju/init/jujud-machine-0")
	c.Check(confDir.Name, gc.Equals, "jujud-machine-0")
	c.Check(confDir.InitSystem, gc.Equals, "upstart")
	c.Check(confDir.ConfName, gc.Equals, "jujud-machine-0.conf")
	c.Check(confDir.Conf, jc.DeepEquals, s.Conf)
	c.Check(confDir.ConfFile.FileName, gc.Equals, "jujud-machine-0.conf")
	c.Check(confDir.ConfFile.Mode, gc.Equals, os.FileMode(0))
	c.Check(string(confDir.ConfFile.Data), gc.Equals, content)
	c.Check(confDir.Files, gc.HasLen, 0)
	s.Stub.CheckCallNames(c, "Name", "Validate", "Validate", "Serialize")
}

func (s *confDirSuite) TestConfDirInfoPopulateMultiline(c *gc.C) {
	content := "<some data>"
	s.Init.Returns.ConfName = "jujud-machine-0.conf"
	s.Init.Returns.Data = []byte(content)
	s.Init.Returns.Name = "upstart"
	s.Conf.Cmd = "spam\nham"

	name := "jujud-machine-0"
	info := s.ConfDirInfo(name)
	confDir, err := info.Populate(s.Conf, s.Init)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(confDir.Conf, jc.DeepEquals, initsystems.Conf{
		Desc: "a service",
		Cmd:  "/var/lib/juju/init/jujud-machine-0/exec-start.sh",
	})
	c.Check(confDir.Files, jc.DeepEquals, []initsystems.FileData{{
		FileName: "exec-start.sh",
		Mode:     0755,
		Data:     []byte("spam\nham"),
	}})
	s.Stub.CheckCallNames(c, "Name", "Validate", "Validate", "Serialize")
}

func (s *confDirSuite) TestConfDirInfoPopulateInitMismatch(c *gc.C) {
	s.Init.Returns.Name = "windows"

	name := "jujud-machine-0"
	info := s.ConfDirInfo(name)
	_, err := info.Populate(s.Conf, s.Init)

	c.Check(err, gc.ErrorMatches, `.*init system mismatch; .*`)
}

func (s *confDirSuite) TestConfDirWrite(c *gc.C) {
	name := "jujud-machine-0"
	confDir := s.ConfDir(name, "<data>")
	err := confDir.Write(s.Files)
	c.Assert(err, jc.ErrorIsNil)

	s.Stub.CheckCallNames(c,
		"MkdirAll",
		"CreateFile",
		"Write",
		"Close",
		"CreateFile",
		"Write",
		"Close",
	)
}

func (s *confDirSuite) TestConfDirWriteFiles(c *gc.C) {
	name := "jujud-machine-0"
	confDir := s.ConfDir(name, "<data>")
	confDir.Files = []initsystems.FileData{{
		FileName: "exec-start.sh",
		Mode:     0755,
		Data:     []byte("spam\nham"),
	}, {
		FileName: "extra.txt",
		Data:     []byte("{}"),
	}}
	err := confDir.Write(s.Files)
	c.Assert(err, jc.ErrorIsNil)

	s.Stub.CheckCallNames(c,
		"MkdirAll",
		"CreateFile",
		"Write",
		"Close",
		"CreateFile",
		"Write",
		"Close",
		"CreateFile",
		"Write",
		"Close",
		"CreateFile",
		"Write",
		"Close",
	)
}

/*
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
