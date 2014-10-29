// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudinit_test

import (
	"fmt"
	"path"
	"testing"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloudinit"
	coretesting "github.com/juju/juju/testing"
	sshtesting "github.com/juju/juju/utils/ssh/testing"
)

// TODO integration tests, but how?

type S struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(S{})

func Test1(t *testing.T) {
	gc.TestingT(t)
}

var ctests = []struct {
	name      string
	expect    string
	setOption func(cfg *cloudinit.Config)
}{
	{
		"User",
		"user: me\n",
		func(cfg *cloudinit.Config) {
			cfg.SetUser("me")
		},
	},
	{
		"AptUpgrade",
		"apt_upgrade: true\n",
		func(cfg *cloudinit.Config) {
			cfg.SetAptUpgrade(true)
		},
	},
	{
		"AptUpdate",
		"apt_update: true\n",
		func(cfg *cloudinit.Config) {
			cfg.SetAptUpdate(true)
		},
	},
	{
		"AptProxy",
		"apt_proxy: http://foo.com\n",
		func(cfg *cloudinit.Config) {
			cfg.SetAptProxy("http://foo.com")
		},
	},
	{
		"AptMirror",
		"apt_mirror: http://foo.com\n",
		func(cfg *cloudinit.Config) {
			cfg.SetAptMirror("http://foo.com")
		},
	},
	{
		"AptPreserveSourcesList",
		"apt_mirror: true\n",
		func(cfg *cloudinit.Config) {
			cfg.SetAptPreserveSourcesList(true)
		},
	},
	{
		"DebconfSelections",
		"debconf_selections: '# Force debconf priority to critical.\n\n  debconf debconf/priority select critical\n\n'\n",
		func(cfg *cloudinit.Config) {
			cfg.SetDebconfSelections("# Force debconf priority to critical.\ndebconf debconf/priority select critical\n")
		},
	},
	{
		"DisableEC2Metadata",
		"disable_ec2_metadata: true\n",
		func(cfg *cloudinit.Config) {
			cfg.SetDisableEC2Metadata(true)
		},
	},
	{
		"FinalMessage",
		"final_message: goodbye\n",
		func(cfg *cloudinit.Config) {
			cfg.SetFinalMessage("goodbye")
		},
	},
	{
		"Locale",
		"locale: en_us\n",
		func(cfg *cloudinit.Config) {
			cfg.SetLocale("en_us")
		},
	},
	{
		"DisableRoot",
		"disable_root: false\n",
		func(cfg *cloudinit.Config) {
			cfg.SetDisableRoot(false)
		},
	},
	{
		"SSHAuthorizedKeys",
		fmt.Sprintf(
			"ssh_authorized_keys:\n- %s\n  Juju:user@host\n- %s\n  Juju:another@host\n",
			sshtesting.ValidKeyOne.Key, sshtesting.ValidKeyTwo.Key),
		func(cfg *cloudinit.Config) {
			cfg.AddSSHAuthorizedKeys(sshtesting.ValidKeyOne.Key + " Juju:user@host")
			cfg.AddSSHAuthorizedKeys(sshtesting.ValidKeyTwo.Key + " another@host")
		},
	},
	{
		"SSHAuthorizedKeys",
		fmt.Sprintf(
			"ssh_authorized_keys:\n- %s\n  Juju:sshkey\n- %s\n  Juju:user@host\n- %s\n  Juju:another@host\n",
			sshtesting.ValidKeyOne.Key, sshtesting.ValidKeyTwo.Key, sshtesting.ValidKeyThree.Key),
		func(cfg *cloudinit.Config) {
			cfg.AddSSHAuthorizedKeys("#command\n" + sshtesting.ValidKeyOne.Key)
			cfg.AddSSHAuthorizedKeys(
				sshtesting.ValidKeyTwo.Key + " user@host\n# comment\n\n" +
					sshtesting.ValidKeyThree.Key + " another@host")
			cfg.AddSSHAuthorizedKeys("")
		},
	},
	{
		"SSHKeys RSAPrivate",
		"ssh_keys:\n  rsa_private: key1data\n",
		func(cfg *cloudinit.Config) {
			cfg.AddSSHKey(cloudinit.RSAPrivate, "key1data")
		},
	},
	{
		"SSHKeys RSAPublic",
		"ssh_keys:\n  rsa_public: key2data\n",
		func(cfg *cloudinit.Config) {
			cfg.AddSSHKey(cloudinit.RSAPublic, "key2data")
		},
	},
	{
		"SSHKeys DSAPublic",
		"ssh_keys:\n  dsa_public: key1data\n",
		func(cfg *cloudinit.Config) {
			cfg.AddSSHKey(cloudinit.DSAPublic, "key1data")
		},
	},
	{
		"SSHKeys DSAPrivate",
		"ssh_keys:\n  dsa_private: key2data\n",
		func(cfg *cloudinit.Config) {
			cfg.AddSSHKey(cloudinit.DSAPrivate, "key2data")
		},
	},
	{
		"Output",
		"output:\n  all:\n  - '>foo'\n  - '|bar'\n",
		func(cfg *cloudinit.Config) {
			cfg.SetOutput("all", ">foo", "|bar")
		},
	},
	{
		"Output",
		"output:\n  all: '>foo'\n",
		func(cfg *cloudinit.Config) {
			cfg.SetOutput(cloudinit.OutAll, ">foo", "")
		},
	},
	{
		"AptSources",
		"apt_sources:\n- source: keyName\n  key: someKey\n",
		func(cfg *cloudinit.Config) {
			cfg.AddAptSource("keyName", "someKey", nil)
		},
	},
	{
		"AptSources with preferences",
		`apt_sources:
- source: keyName
  key: someKey
bootcmd:
- install -D -m 644 /dev/null '/some/path'
- 'printf ''%s\n'' ''Explanation: test

  Package: *

  Pin: release n=series

  Pin-Priority: 123

  '' > ''/some/path'''
`,
		func(cfg *cloudinit.Config) {
			prefs := &cloudinit.AptPreferences{
				Path:        "/some/path",
				Explanation: "test",
				Package:     "*",
				Pin:         "release n=series",
				PinPriority: 123,
			}
			cfg.AddAptSource("keyName", "someKey", prefs)
		},
	},
	{
		"Packages",
		"packages:\n- juju\n- ubuntu\n",
		func(cfg *cloudinit.Config) {
			cfg.AddPackage("juju")
			cfg.AddPackage("ubuntu")
		},
	},
	{
		"Packages with --target-release",
		"packages:\n- --target-release 'precise-updates/cloud-tools' 'mongodb-server'\n",
		func(cfg *cloudinit.Config) {
			cfg.AddPackageFromTargetRelease("mongodb-server", "precise-updates/cloud-tools")
		},
	},
	{
		"BootCmd",
		"bootcmd:\n- ls > /dev\n- - ls\n  - '>with space'\n",
		func(cfg *cloudinit.Config) {
			cfg.AddBootCmd("ls > /dev")
			cfg.AddBootCmdArgs("ls", ">with space")
		},
	},
	{
		"Mounts",
		"mounts:\n- - x\n  - \"y\"\n- - z\n  - w\n",
		func(cfg *cloudinit.Config) {
			cfg.AddMount("x", "y")
			cfg.AddMount("z", "w")
		},
	},
	{
		"Attr",
		"arbitraryAttr: someValue\n",
		func(cfg *cloudinit.Config) {
			cfg.SetAttr("arbitraryAttr", "someValue")
		},
	},
	{
		"RunCmd",
		"runcmd:\n- ifconfig\n",
		func(cfg *cloudinit.Config) {
			cfg.AddRunCmd("ifconfig")
		},
	},
	{
		"AddScripts",
		"runcmd:\n- echo 'Hello World'\n- ifconfig\n",
		func(cfg *cloudinit.Config) {
			cfg.AddScripts(
				"echo 'Hello World'",
				"ifconfig",
			)
		},
	},
	{
		"AddTextFile",
		addTextFileExpected,
		func(cfg *cloudinit.Config) {
			cfg.AddTextFile(
				"/etc/apt/apt.conf.d/99proxy",
				`"Acquire::http::Proxy "http://10.0.3.1:3142";`,
				0644,
			)
		},
	},
	{
		"AddBinaryFile",
		addBinaryFileExpected,
		func(cfg *cloudinit.Config) {
			cfg.AddBinaryFile(
				"/dev/nonsense",
				[]byte{0, 1, 2, 3},
				0644,
			)
		},
	},
	{
		"SetAptGetWrapper",
		"apt_get_wrapper:\n  command: eatmydata\n  enabled: auto\n",
		func(cfg *cloudinit.Config) {
			cfg.SetAptGetWrapper("eatmydata")
		},
	},
}

const (
	header              = "#cloud-config\n"
	addTextFileExpected = `runcmd:
- install -D -m 644 /dev/null '/etc/apt/apt.conf.d/99proxy'
- printf '%s\n' '"Acquire::http::Proxy "http://10.0.3.1:3142";' > '/etc/apt/apt.conf.d/99proxy'
`
	addBinaryFileExpected = `runcmd:
- install -D -m 644 /dev/null '/dev/nonsense'
- printf %s AAECAw== | base64 -d > '/dev/nonsense'
`
)

func (S) TestOutput(c *gc.C) {
	for _, t := range ctests {
		cfg := cloudinit.New()
		t.setOption(cfg)
		renderer, err := cloudinit.NewRenderer("quantal")
		c.Assert(err, gc.IsNil)
		data, err := renderer.Render(cfg)
		c.Assert(err, gc.IsNil)
		c.Assert(data, gc.NotNil)
		c.Assert(string(data), gc.Equals, header+t.expect, gc.Commentf("test %q output differs", t.name))
	}
}

func (S) TestRunCmds(c *gc.C) {
	cfg := cloudinit.New()
	c.Assert(cfg.RunCmds(), gc.HasLen, 0)
	cfg.AddScripts("a", "b")
	cfg.AddRunCmdArgs("c", "d")
	cfg.AddRunCmd("e")
	c.Assert(cfg.RunCmds(), gc.DeepEquals, []interface{}{
		"a", "b", []string{"c", "d"}, "e",
	})
}

func (S) TestPackages(c *gc.C) {
	cfg := cloudinit.New()
	c.Assert(cfg.Packages(), gc.HasLen, 0)
	cfg.AddPackage("a b c")
	cfg.AddPackage("d!")
	expectedPackages := []string{"a b c", "d!"}
	c.Assert(cfg.Packages(), gc.DeepEquals, expectedPackages)
	cfg.AddPackageFromTargetRelease("package", "series")
	expectedPackages = append(expectedPackages, "--target-release 'series' 'package'")
	c.Assert(cfg.Packages(), gc.DeepEquals, expectedPackages)
}

func (S) TestSetOutput(c *gc.C) {
	type test struct {
		kind   cloudinit.OutputKind
		stdout string
		stderr string
	}
	tests := []test{{
		cloudinit.OutAll, "a", "",
	}, {
		cloudinit.OutAll, "", "b",
	}, {
		cloudinit.OutInit, "a", "b",
	}, {
		cloudinit.OutAll, "a", "b",
	}, {
		cloudinit.OutAll, "", "",
	}}

	cfg := cloudinit.New()
	stdout, stderr := cfg.Output(cloudinit.OutAll)
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")
	for i, t := range tests {
		c.Logf("test %d: %+v", i, t)
		cfg.SetOutput(t.kind, t.stdout, t.stderr)
		stdout, stderr = cfg.Output(t.kind)
		c.Assert(stdout, gc.Equals, t.stdout)
		c.Assert(stderr, gc.Equals, t.stderr)
	}
}

//#cloud-config
//packages:
//- juju
//- ubuntu
func ExampleConfig() {
	cfg := cloudinit.New()
	cfg.AddPackage("juju")
	cfg.AddPackage("ubuntu")
	renderer, err := cloudinit.NewRenderer("quantal")
	if err != nil {
		fmt.Printf("render error: %v", err)
		return
	}
	data, err := renderer.Render(cfg)
	if err != nil {
		fmt.Printf("render error: %v", err)
		return
	}
	fmt.Printf("%s", data)
}

func (S) TestUbuntuMkdir(c *gc.C) {
	compareOutput := "mkdir -p 'fake_dir'"
	render, err := cloudinit.NewRenderer("precise")
	c.Assert(err, gc.IsNil)
	output := render.Mkdir("fake_dir")
	c.Assert(err, gc.IsNil)
	c.Assert(output, gc.NotNil)
	c.Assert(output[0], gc.Equals, compareOutput, gc.Commentf("test %q output differs", "windows mkdir"))
}

func (S) TestUbuntuWriteFile(c *gc.C) {
	filePath := path.Join("fake_dir", "test_file")
	compareOutput := "install -m 17141 /dev/null 'fake_dir/test_file'"

	render, err := cloudinit.NewRenderer("precise")
	c.Assert(err, gc.IsNil)
	output := render.WriteFile(filePath, "fake output", 7777)
	c.Assert(err, gc.IsNil)
	c.Assert(output, gc.NotNil)
	c.Assert(output[0], gc.Equals, compareOutput, gc.Commentf("test %q output differs", "windows writefile"))
}

func (S) TestUbuntuFromSlash(c *gc.C) {
	filePath := path.Join("tmp/file\\path//value\\")
	compareOutput := "tmp/file\\path/value\\"

	render, err := cloudinit.NewRenderer("precise")
	c.Assert(err, gc.IsNil)
	output := render.FromSlash(filePath)
	c.Assert(err, gc.IsNil)
	c.Assert(output, gc.NotNil)
	c.Assert(output, gc.Equals, compareOutput, gc.Commentf("test %q output differs", "windows fromslash"))
}

func (S) TestUbuntuPathJoin(c *gc.C) {
	dirPath := path.Join("fake", "dir")
	compareOutput := "fake/dir/fakeFile"

	render, err := cloudinit.NewRenderer("precise")
	c.Assert(err, gc.IsNil)
	output := render.PathJoin(dirPath, "fakeFile")
	c.Assert(err, gc.IsNil)
	c.Assert(output, gc.NotNil)
	c.Assert(output, gc.Equals, compareOutput, gc.Commentf("test %q output differs", "windows writefile"))
}

func (S) TestWindowsRender(c *gc.C) {
	compareOutput := "#ps1_sysnative\r\n\r\npowershell"
	cfg := cloudinit.New()
	cfg.AddRunCmd("powershell")
	render, err := cloudinit.NewRenderer("win8")
	c.Assert(err, gc.IsNil)
	data, err := render.Render(cfg)
	c.Assert(err, gc.IsNil)
	c.Assert(data, gc.NotNil)
	c.Assert(string(data), gc.Equals, compareOutput, gc.Commentf("test %q output differs", "windows renderer"))
}

func (S) TestWindowsMkdir(c *gc.C) {
	render, err := cloudinit.NewRenderer("win8")
	compareOutput := "mkdir fake_dir"
	c.Assert(err, gc.IsNil)
	output := render.Mkdir("fake_dir")
	c.Assert(err, gc.IsNil)
	c.Assert(output, gc.NotNil)
	c.Assert(output[0], gc.Equals, compareOutput, gc.Commentf("test %q output differs", "windows mkdir"))
}

func (S) TestWindowsWriteFile(c *gc.C) {
	filePath := path.Join("fake_dir", "test_file")
	compareOutput := "Set-Content '" + filePath + "' @\"\nfake output\n\"@"

	render, err := cloudinit.NewRenderer("win8")
	c.Assert(err, gc.IsNil)
	output := render.WriteFile(filePath, "fake output", 7777)
	c.Assert(err, gc.IsNil)
	c.Assert(output, gc.NotNil)
	c.Assert(output[0], gc.Equals, compareOutput, gc.Commentf("test %q output differs", "windows writefile"))
}

func (S) TestWindowsFromSlash(c *gc.C) {
	filePath := path.Join("fake/file\\path//value\\/\\")
	compareOutput := "fake\\file\\path\\value\\\\\\"

	render, err := cloudinit.NewRenderer("win8")
	c.Assert(err, gc.IsNil)
	output := render.FromSlash(filePath)
	c.Assert(err, gc.IsNil)
	c.Assert(output, gc.NotNil)
	c.Assert(output, gc.Equals, compareOutput, gc.Commentf("test %q output differs", "windows fromslash"))
}

func (S) TestWindowsPathJoin(c *gc.C) {
	dirPath := path.Join("fake", "dir")
	compareOutput := "fake\\dir\\fakeFile"

	render, err := cloudinit.NewRenderer("win8")
	c.Assert(err, gc.IsNil)
	output := render.PathJoin(render.FromSlash(dirPath), "fakeFile")
	c.Assert(err, gc.IsNil)
	c.Assert(output, gc.NotNil)
	c.Assert(output, gc.Equals, compareOutput, gc.Commentf("test %q output differs", "windows writefile"))
}
