// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upstart_test

import (
	"fmt"
	"os"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/utils/fs"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/service/initsystems"
	"github.com/juju/juju/service/initsystems/upstart"
	coretesting "github.com/juju/juju/testing"
)

const confStr = `
description "juju agent for %s"
author "Juju Team <juju@lists.ubuntu.com>"
start on runlevel [2345]
stop on runlevel [!2345]
respawn
normal exit 0
%s
%s
script
%s
  exec /var/lib/juju/init/%s/script.sh%s
end script
`

const outStr = `
  # Ensure log files are properly protected
  touch %s
  chown syslog:syslog %s
  chmod 0600 %s
`

type initSystemSuite struct {
	coretesting.BaseSuite

	initDir string
	conf    initsystems.Conf
	confStr string

	fake  *testing.Fake
	files *fs.FakeOps
	cmd   *initsystems.FakeShell
	init  initsystems.InitSystem
}

var _ = gc.Suite(&initSystemSuite{})

func (s *initSystemSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.initDir = c.MkDir()
	s.conf = initsystems.Conf{
		Desc: `juju agent for jujud-machine-0`,
		Cmd:  "/var/lib/juju/init/jujud-machine-0/script.sh",
	}
	s.confStr = s.newConfStr("jujud-machine-0", "", nil, nil)

	s.fake = &testing.Fake{}
	s.files = &fs.FakeOps{Fake: s.fake}
	s.cmd = &initsystems.FakeShell{Fake: s.fake}
	s.init = upstart.NewUpstart(s.initDir, s.files, s.cmd)

	s.PatchValue(&upstart.ConfDir, s.initDir)
	s.PatchValue(&initsystems.RetryAttempts, utils.AttemptStrategy{})
}

func (s *initSystemSuite) newConfStr(name, out string, env, limit map[string]string) string {
	var outScript, envStr, limitStr string
	if out != "" {
		outScript = fmt.Sprintf(outStr, out, out, out)
		out = " >> " + out + " 2>&1"
	}
	if env != nil {
		for key, value := range env {
			envStr += "env " + key + "=\"" + value + "\"\n"
		}
	}
	if limit != nil {
		for key, value := range limit {
			limitStr += "limit " + key + " " + value + "\n"
		}
	}
	return fmt.Sprintf(confStr[1:], name, envStr, limitStr, outScript, name, out)
}

func (s *initSystemSuite) TestInitSystemName(c *gc.C) {
	name := s.init.Name()

	c.Check(name, gc.Equals, "upstart")
}

func (s *initSystemSuite) TestInitSystemList(c *gc.C) {
	s.files.Returns.DirEntries = []os.FileInfo{
		fs.NewFile("jujud-machine-0.conf", 0644, nil),
		fs.NewFile("something-else.conf", 0644, nil),
		fs.NewFile("jujud-unit-wordpress-0.conf", 0644, nil),
		fs.NewDir("jujud-unit-mysql-0.conf", 0755),
		fs.NewDir("something-random", 0755),
	}

	names, err := s.init.List()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(names, jc.SameContents, []string{
		"jujud-machine-0",
		"something-else",
		"jujud-unit-wordpress-0",
	})
}

func (s *initSystemSuite) TestInitSystemListLimited(c *gc.C) {
	s.files.Returns.DirEntries = []os.FileInfo{
		fs.NewFile("jujud-machine-0.conf", 0644, nil),
		fs.NewFile("something-else.conf", 0644, nil),
		fs.NewFile("jujud-unit-wordpress-0.conf", 0644, nil),
		fs.NewDir("jujud-unit-mysql-0.conf", 0755),
		fs.NewDir("something-random", 0755),
	}

	names, err := s.init.List("jujud-machine-0")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(names, jc.SameContents, []string{"jujud-machine-0"})
}

func (s *initSystemSuite) TestInitSystemListLimitedEmpty(c *gc.C) {
	names, err := s.init.List("jujud-machine-0")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(names, jc.SameContents, []string{})
}

func (s *initSystemSuite) TestInitSystemStart(c *gc.C) {
	s.files.Returns.Exists = true

	name := "jujud-unit-wordpress-0"
	err := s.init.Start(name)
	c.Assert(err, jc.ErrorIsNil)

	s.fake.CheckCalls(c, []testing.FakeCall{{
		FuncName: "Exists",
		Args: testing.FakeCallArgs{
			"name": s.initDir + "/" + name + ".conf",
		},
	}, {
		FuncName: "RunCommand",
		Args: testing.FakeCallArgs{
			"cmd":  "status",
			"args": []string{"--system", name},
		},
	}, {
		FuncName: "RunCommand",
		Args: testing.FakeCallArgs{
			"cmd":  "start",
			"args": []string{"--system", name},
		},
	}})
}

func (s *initSystemSuite) TestInitSystemStartAlreadyRunning(c *gc.C) {
	name := "jujud-unit-wordpress-0"
	s.files.Returns.Exists = true
	s.cmd.Out = []byte("service " + name + " start/running, process 12345\n")

	err := s.init.Start(name)

	c.Check(err, jc.Satisfies, errors.IsAlreadyExists)
}

func (s *initSystemSuite) TestInitSystemStartNotEnabled(c *gc.C) {
	err := s.init.Start("jujud-unit-wordpress-0")

	c.Check(err, jc.Satisfies, errors.IsNotFound)
}

func (s *initSystemSuite) TestInitSystemStop(c *gc.C) {
	name := "jujud-unit-wordpress-0"
	s.files.Returns.Exists = true
	s.cmd.Out = []byte("service " + name + " start/running, process 12345\n")

	err := s.init.Stop(name)
	c.Assert(err, jc.ErrorIsNil)

	s.fake.CheckCalls(c, []testing.FakeCall{{
		FuncName: "Exists",
		Args: testing.FakeCallArgs{
			"name": s.initDir + "/" + name + ".conf",
		},
	}, {
		FuncName: "RunCommand",
		Args: testing.FakeCallArgs{
			"cmd":  "status",
			"args": []string{"--system", name},
		},
	}, {
		FuncName: "RunCommand",
		Args: testing.FakeCallArgs{
			"cmd":  "stop",
			"args": []string{"--system", name},
		},
	}})
}

func (s *initSystemSuite) TestInitSystemStopNotRunning(c *gc.C) {
	s.files.Returns.Exists = true

	err := s.init.Stop("jujud-unit-wordpress-0")

	c.Check(err, jc.Satisfies, errors.IsNotFound)
}

func (s *initSystemSuite) TestInitSystemStopNotEnabled(c *gc.C) {
	err := s.init.Stop("jujud-unit-wordpress-0")

	c.Check(err, jc.Satisfies, errors.IsNotFound)
}

func (s *initSystemSuite) TestInitSystemEnable(c *gc.C) {
	name := "jujud-unit-wordpress-0"
	data := s.newConfStr(name, "", nil, nil)
	s.files.Returns.Data = []byte(data)

	filename := "/var/lib/juju/init/" + name + ".conf"
	err := s.init.Enable(name, filename)
	c.Assert(err, jc.ErrorIsNil)

	initFile := s.initDir + "/" + name + ".conf"
	s.fake.CheckCalls(c, []testing.FakeCall{{
		FuncName: "Exists",
		Args: testing.FakeCallArgs{
			"name": initFile,
		},
	}, {
		FuncName: "ReadFile",
		Args: testing.FakeCallArgs{
			"filename": filename,
		},
	}, {
		FuncName: "Symlink",
		Args: testing.FakeCallArgs{
			"oldName": filename,
			"newName": initFile,
		},
	}})
}

func (s *initSystemSuite) TestInitSystemEnableAlreadyEnabled(c *gc.C) {
	s.files.Returns.Exists = true

	name := "jujud-unit-wordpress-0"
	filename := "/var/lib/juju/init/" + name
	err := s.init.Enable(name, filename)

	c.Check(err, jc.Satisfies, errors.IsAlreadyExists)
}

func (s *initSystemSuite) TestInitSystemDisable(c *gc.C) {
	s.files.Returns.Exists = true

	name := "jujud-unit-wordpress-0"
	err := s.init.Disable("jujud-unit-wordpress-0")
	c.Assert(err, jc.ErrorIsNil)

	initFile := s.initDir + "/" + name + ".conf"
	s.fake.CheckCalls(c, []testing.FakeCall{{
		FuncName: "Exists",
		Args: testing.FakeCallArgs{
			"name": s.initDir + "/" + name + ".conf",
		},
	}, {
		FuncName: "RemoveAll",
		Args: testing.FakeCallArgs{
			"name": initFile,
		},
	}})
}

func (s *initSystemSuite) TestInitSystemDisableNotEnabled(c *gc.C) {
	err := s.init.Disable("jujud-unit-wordpress-0")

	c.Check(err, jc.Satisfies, errors.IsNotFound)
}

func (s *initSystemSuite) TestInitSystemIsEnabledTrue(c *gc.C) {
	s.files.Returns.Exists = true

	name := "jujud-unit-wordpress-0"
	enabled, err := s.init.IsEnabled("jujud-unit-wordpress-0")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(enabled, jc.IsTrue)

	initFile := s.initDir + "/" + name + ".conf"
	s.fake.CheckCalls(c, []testing.FakeCall{{
		FuncName: "Exists",
		Args: testing.FakeCallArgs{
			"name": initFile,
		},
	}})
}

func (s *initSystemSuite) TestInitSystemIsEnabledFalse(c *gc.C) {
	enabled, err := s.init.IsEnabled("jujud-unit-wordpress-0")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(enabled, jc.IsFalse)
}

func (s *initSystemSuite) TestInitSystemInfoRunning(c *gc.C) {
	name := "jujud-unit-wordpress-0"
	data := s.newConfStr(name, "", nil, nil)
	s.files.Returns.Data = []byte(data)
	s.files.Returns.Exists = true
	s.cmd.Out = []byte("service " + name + " start/running, process 12345\n")

	info, err := s.init.Info(name)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(info, jc.DeepEquals, &initsystems.ServiceInfo{
		Name:        name,
		Description: "juju agent for " + name,
		Status:      initsystems.StatusRunning,
	})

	initFile := s.initDir + "/" + name + ".conf"
	s.fake.CheckCalls(c, []testing.FakeCall{{
		FuncName: "Exists",
		Args: testing.FakeCallArgs{
			"name": s.initDir + "/" + name + ".conf",
		},
	}, {
		FuncName: "ReadFile",
		Args: testing.FakeCallArgs{
			"filename": initFile,
		},
	}, {
		FuncName: "RunCommand",
		Args: testing.FakeCallArgs{
			"cmd":  "status",
			"args": []string{"--system", name},
		},
	}})
}

func (s *initSystemSuite) TestInitSystemInfoNotRunning(c *gc.C) {
	name := "jujud-unit-wordpress-0"
	data := s.newConfStr(name, "", nil, nil)
	s.files.Returns.Data = []byte(data)
	s.files.Returns.Exists = true

	info, err := s.init.Info(name)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(info, jc.DeepEquals, &initsystems.ServiceInfo{
		Name:        name,
		Description: "juju agent for " + name,
		Status:      initsystems.StatusStopped,
	})
}

func (s *initSystemSuite) TestInitSystemInfoNotEnabled(c *gc.C) {
	_, err := s.init.Info("jujud-unit-wordpress-0")

	c.Check(err, jc.Satisfies, errors.IsNotFound)
}

func (s *initSystemSuite) TestInitSystemConf(c *gc.C) {
	name := "jujud-unit-wordpress-0"
	data := s.newConfStr(name, "", nil, nil)
	s.files.Returns.Data = []byte(data)

	conf, err := s.init.Conf(name)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(conf, jc.DeepEquals, &initsystems.Conf{
		Desc: `juju agent for jujud-unit-wordpress-0`,
		Cmd:  "/var/lib/juju/init/" + name + "/script.sh",
	})

	initFile := s.initDir + "/" + name + ".conf"
	s.fake.CheckCalls(c, []testing.FakeCall{{
		FuncName: "ReadFile",
		Args: testing.FakeCallArgs{
			"filename": initFile,
		},
	}})
}

func (s *initSystemSuite) TestInitSystemConfNotEnabled(c *gc.C) {
	s.files.SetErrors(os.ErrNotExist)

	_, err := s.init.Conf("jujud-unit-wordpress-0")

	c.Check(err, jc.Satisfies, errors.IsNotFound)
}

func (s *initSystemSuite) TestInitSystemValidate(c *gc.C) {
	err := s.init.Validate("jujud-unit-wordpress-0", s.conf)
	c.Assert(err, jc.ErrorIsNil)

	s.fake.CheckCalls(c, nil)
}

func (s *initSystemSuite) TestInitSystemValidateInvalid(c *gc.C) {
	s.conf.Cmd = ""

	err := s.init.Validate("jujud-unit-wordpress-0", s.conf)

	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

func (s *initSystemSuite) TestInitSystemSerializeBasic(c *gc.C) {
	data, err := s.init.Serialize("jujud-machine-0", s.conf)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(string(data), gc.Equals, s.confStr)

	s.fake.CheckCalls(c, nil)
}

func (s *initSystemSuite) TestInitSystemSerializeFull(c *gc.C) {
	name := "jujud-unit-wordpress-0"
	conf := initsystems.Conf{
		Desc: `juju agent for ` + name,
		Cmd:  "/var/lib/juju/init/" + name + "/script.sh",
		Env: map[string]string{
			"x": "y",
		},
		Limit: map[string]string{
			"w": "z",
		},
		Out: "/var/log/juju/" + name + ".log",
	}
	data, err := s.init.Serialize("jujud-machine-0", conf)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(string(data), gc.Equals, s.newConfStr(
		name,
		"/var/log/juju/"+name+".log",
		map[string]string{
			"x": "y",
		},
		map[string]string{
			"w": "z",
		},
	))
}

func (s *initSystemSuite) TestInitSystemDeserializeBasic(c *gc.C) {
	name := "jujud-unit-wordpress-0"
	data := s.newConfStr(name, "", nil, nil)
	conf, err := s.init.Deserialize([]byte(data))
	c.Assert(err, jc.ErrorIsNil)

	c.Check(conf, jc.DeepEquals, &initsystems.Conf{
		Desc: `juju agent for jujud-unit-wordpress-0`,
		Cmd:  "/var/lib/juju/init/" + name + "/script.sh",
	})

	s.fake.CheckCalls(c, nil)
}

func (s *initSystemSuite) TestInitSystemDeserializeFull(c *gc.C) {
	name := "jujud-unit-wordpress-0"
	env := map[string]string{
		"x": "y",
	}
	limit := map[string]string{
		"w": "z",
	}
	data := s.newConfStr(name, "/var/log/juju/"+name+".log", env, limit)
	conf, err := s.init.Deserialize([]byte(data))
	c.Assert(err, jc.ErrorIsNil)

	c.Check(conf, jc.DeepEquals, &initsystems.Conf{
		Desc: `juju agent for ` + name,
		Cmd:  "/var/lib/juju/init/" + name + "/script.sh",
		Env: map[string]string{
			"x": "y",
		},
		Limit: map[string]string{
			"w": "z",
		},
		Out: "/var/log/juju/" + name + ".log",
	})
}
