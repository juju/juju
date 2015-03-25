// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudinit_test

import (
	"fmt"
	"testing"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloudconfig/cloudinit"
	"github.com/juju/juju/cloudconfig/cloudinit/packaging"
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
	expect    map[string]interface{}
	setOption func(cfg cloudinit.CloudConfig)
}{
	{
		"User",
		map[string]interface{}{"user": "me"},
		func(cfg cloudinit.CloudConfig) {
			cfg.SetUser("me")
		},
	},
	{
		"PackageUpgrade",
		map[string]interface{}{"apt_upgrade": true},
		func(cfg cloudinit.CloudConfig) {
			cfg.SetSystemUpgrade(true)
		},
	},
	{
		"PackageUpdate",
		map[string]interface{}{"apt_update": true},
		func(cfg cloudinit.CloudConfig) {
			cfg.SetSystemUpdate(true)
		},
	},
	{
		"PackageProxy",
		map[string]interface{}{"apt_proxy": "http://foo.com"},
		func(cfg cloudinit.CloudConfig) {
			cfg.SetPackageProxy("http://foo.com")
		},
	},
	{
		"PackageMirror",
		map[string]interface{}{"apt_mirror": "http://foo.com"},
		func(cfg cloudinit.CloudConfig) {
			cfg.SetPackageMirror("http://foo.com")
		},
	},
	{
		"DisableEC2Metadata",
		map[string]interface{}{"disable_ec2_metadata": true},
		func(cfg cloudinit.CloudConfig) {
			cfg.SetDisableEC2Metadata(true)
		},
	},
	{
		"FinalMessage",
		map[string]interface{}{"final_message": "goodbye"},
		func(cfg cloudinit.CloudConfig) {
			cfg.SetFinalMessage("goodbye")
		},
	},
	{
		"Locale",
		map[string]interface{}{"locale": "en_us"},
		func(cfg cloudinit.CloudConfig) {
			cfg.SetLocale("en_us")
		},
	},
	{
		"DisableRoot",
		map[string]interface{}{"disable_root": false},
		func(cfg cloudinit.CloudConfig) {
			cfg.SetDisableRoot(false)
		},
	},
	{
		"SSHAuthorizedKeys",
		map[string]interface{}{"ssh_authorized_keys": []string{
			fmt.Sprintf("%s Juju:user@host", sshtesting.ValidKeyOne.Key),
			fmt.Sprintf("%s Juju:another@host", sshtesting.ValidKeyTwo.Key),
		}},
		func(cfg cloudinit.CloudConfig) {
			cfg.AddSSHAuthorizedKeys(sshtesting.ValidKeyOne.Key + " Juju:user@host")
			cfg.AddSSHAuthorizedKeys(sshtesting.ValidKeyTwo.Key + " another@host")
		},
	},
	{
		"SSHAuthorizedKeys",
		map[string]interface{}{"ssh_authorized_keys": []string{
			fmt.Sprintf("%s Juju:sshkey", sshtesting.ValidKeyOne.Key),
			fmt.Sprintf("%s Juju:user@host", sshtesting.ValidKeyTwo.Key),
			fmt.Sprintf("%s Juju:another@host", sshtesting.ValidKeyThree.Key),
		}},
		func(cfg cloudinit.CloudConfig) {
			cfg.AddSSHAuthorizedKeys("#command\n" + sshtesting.ValidKeyOne.Key)
			cfg.AddSSHAuthorizedKeys(
				sshtesting.ValidKeyTwo.Key + " user@host\n# comment\n\n" +
					sshtesting.ValidKeyThree.Key + " another@host")
			cfg.AddSSHAuthorizedKeys("")
		},
	},
	{
		"SSHKeys RSAPrivate",
		map[string]interface{}{"ssh_keys": map[string]interface{}{
			"rsa_private": "key1data",
		}},
		func(cfg cloudinit.CloudConfig) {
			cfg.AddSSHKey(cloudinit.RSAPrivate, "key1data")
		},
	},
	{
		"SSHKeys RSAPublic",
		map[string]interface{}{"ssh_keys": map[string]interface{}{
			"rsa_public": "key2data",
		}},
		func(cfg cloudinit.CloudConfig) {
			cfg.AddSSHKey(cloudinit.RSAPublic, "key2data")
		},
	},
	{
		"SSHKeys DSAPublic",
		map[string]interface{}{"ssh_keys": map[string]interface{}{
			"dsa_public": "key1data",
		}},
		func(cfg cloudinit.CloudConfig) {
			cfg.AddSSHKey(cloudinit.DSAPublic, "key1data")
		},
	},
	{
		"SSHKeys DSAPrivate",
		map[string]interface{}{"ssh_keys": map[string]interface{}{
			"dsa_private": "key2data",
		}},
		func(cfg cloudinit.CloudConfig) {
			cfg.AddSSHKey(cloudinit.DSAPrivate, "key2data")
		},
	},
	{
		"Output",
		map[string]interface{}{"output": map[string]interface{}{
			"all": []string{">foo", "|bar"},
		}},
		func(cfg cloudinit.CloudConfig) {
			cfg.SetOutput("all", ">foo", "|bar")
		},
	},
	{
		"Output",
		map[string]interface{}{"output": map[string]interface{}{
			"all": ">foo",
		}},
		func(cfg cloudinit.CloudConfig) {
			cfg.SetOutput(cloudinit.OutAll, ">foo", "")
		},
	},
	{
		"PackageSources",
		map[string]interface{}{"apt_sources": []map[string]interface{}{
			{
				"source": "keyName",
				"key":    "someKey",
			},
		}},
		func(cfg cloudinit.CloudConfig) {
			cfg.AddPackageSource(packaging.Source{Name: "keyName", Key: "someKey"})
		},
	},
	{
		"PackageSources with preferences",
		map[string]interface{}{
			"apt_sources": []map[string]interface{}{
				{
					"source": "keyName",
					"key":    "someKey",
				},
			},
			"bootcmd": []string{
				"install -D -m 644 /dev/null '/some/path'",
				"printf '%s\\n' 'Explanation: test\n" +
					"Package: *\n" +
					"Pin: release n=series\n" +
					"Pin-Priority: 123\n" +
					"' > '/some/path'",
			},
		},
		func(cfg cloudinit.CloudConfig) {
			prefs := packaging.PackagePreferences{
				Path:        "/some/path",
				Explanation: "test",
				Package:     "*",
				Pin:         "release n=series",
				Priority:    123,
			}
			cfg.AddPackageSource(packaging.Source{Name: "keyName", Key: "someKey"})
			cfg.AddPackagePreferences(prefs)
		},
	},
	{
		"Packages",
		map[string]interface{}{"packages": []string{
			"juju",
			"ubuntu",
		}},
		func(cfg cloudinit.CloudConfig) {
			cfg.AddPackage("juju")
			cfg.AddPackage("ubuntu")
		},
	},
	{
		"BootCmd",
		map[string]interface{}{"bootcmd": []interface{}{
			"ls > /dev",
			[]string{"ls", ">with space"},
		}},
		func(cfg cloudinit.CloudConfig) {
			cfg.AddBootCmd("ls > /dev")
			cfg.AddBootCmd("ls >with space")
		},
	},
	{
		"Mounts",
		map[string]interface{}{"mounts": [][]string{
			{"x", "y"},
			{"z", "w"},
		}},
		func(cfg cloudinit.CloudConfig) {
			cfg.AddMount("x", "y")
			cfg.AddMount("z", "w")
		},
	},
	{
		"Attr",
		map[string]interface{}{"arbitraryAttr": "someValue"},
		func(cfg cloudinit.CloudConfig) {
			cfg.SetAttr("arbitraryAttr", "someValue")
		},
	},
	{
		"RunCmd",
		map[string]interface{}{"runcmd": []string{
			"ifconfig",
		}},
		func(cfg cloudinit.CloudConfig) {
			cfg.AddRunCmd("ifconfig")
		},
	},
	{
		"AddScripts",
		map[string]interface{}{"runcmd": []string{
			"echo 'Hello World'",
			"ifconfig",
		}},
		func(cfg cloudinit.CloudConfig) {
			cfg.AddScripts(
				"echo 'Hello World'",
				"ifconfig",
			)
		},
	},
	{
		"AddTextFile",
		map[string]interface{}{"runcmd": []string{
			"install -D -m 644 /dev/null '/etc/apt/apt.conf.d/99proxy'",
			"printf '%s\\n' '\"Acquire::http::Proxy \"http://10.0.3.1:3142\";' > '/etc/apt/apt.conf.d/99proxy'",
		}},
		func(cfg cloudinit.CloudConfig) {
			cfg.AddRunTextFile(
				"/etc/apt/apt.conf.d/99proxy",
				`"Acquire::http::Proxy "http://10.0.3.1:3142";`,
				0644,
			)
		},
	},
	{
		"AddBinaryFile",
		map[string]interface{}{"runcmd": []string{
			"install -D -m 644 /dev/null '/dev/nonsense'",
			"printf %s AAECAw== | base64 -d > '/dev/nonsense'",
		}},
		func(cfg cloudinit.CloudConfig) {
			cfg.AddRunBinaryFile(
				"/dev/nonsense",
				[]byte{0, 1, 2, 3},
				0644,
			)
		},
	},
	{
		"AddBootTextFile",
		map[string]interface{}{"bootcmd": []string{
			"install -D -m 644 /dev/null '/etc/apt/apt.conf.d/99proxy'",
			"printf '%s\\n' '\"Acquire::http::Proxy \"http://10.0.3.1:3142\";' > '/etc/apt/apt.conf.d/99proxy'",
		}},
		func(cfg cloudinit.CloudConfig) {
			cfg.AddBootTextFile(
				"/etc/apt/apt.conf.d/99proxy",
				`"Acquire::http::Proxy "http://10.0.3.1:3142";`,
				0644,
			)
		},
	},
}

func (S) TestOutput(c *gc.C) {
	for i, t := range ctests {
		c.Logf("test %d: %s", i, t.name)
		cfg, err := cloudinit.New("trusty")
		c.Assert(err, jc.ErrorIsNil)
		t.setOption(cfg)
		renderer, err := cloudinit.NewRenderer("quantal")
		c.Assert(err, jc.ErrorIsNil)
		t.setOption(cfg)
		data, err := cfg.Render()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(data, gc.NotNil)
		c.Assert(string(data), jc.YAMLEquals, t.expect)
	}
}

func (S) TestRunCmds(c *gc.C) {
	cfg, err := cloudinit.New("trusty")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.RunCmds(), gc.HasLen, 0)
	cfg.AddScripts("a", "b")
	cfg.AddRunCmd("e")
	c.Assert(cfg.RunCmds(), gc.DeepEquals, []interface{}{
		"a", "b", "e",
	})
}

func (S) TestPackages(c *gc.C) {
	cfg, err := cloudinit.New("trusty")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.Packages(), gc.HasLen, 0)
	cfg.AddPackage("a b c")
	cfg.AddPackage("d!")
	expectedPackages := []string{"a b c", "d!"}
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

	cfg, err := cloudinit.New("trusty")
	c.Assert(err, jc.ErrorIsNil)
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

func (S) TestWindowsRender(c *gc.C) {
	compareOutput := "#ps1_sysnative\r\n\r\npowershell"
	cfg, err := cloudinit.New("trusty")
	c.Assert(err, jc.ErrorIsNil)
	cfg.AddRunCmd("powershell")
	render, err := cloudinit.NewRenderer("win8")
	c.Assert(err, jc.ErrorIsNil)
	cfg.AddRunCmd("powershell")
	data, err := cfg.Render()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(data, gc.NotNil)
	c.Assert(string(data), gc.Equals, compareOutput, gc.Commentf("test %q output differs", "windows renderer"))
}
