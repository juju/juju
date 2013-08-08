// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudinit_test

import (
	"fmt"
	"testing"

	. "launchpad.net/gocheck"

	"launchpad.net/juju-core/cloudinit"
)

// TODO integration tests, but how?

type S struct{}

var _ = Suite(S{})

func Test1(t *testing.T) {
	TestingT(t)
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
		"ssh_authorized_keys:\n- key1\n- key2\n",
		func(cfg *cloudinit.Config) {
			cfg.AddSSHAuthorizedKeys("key1")
			cfg.AddSSHAuthorizedKeys("key2")
		},
	},
	{
		"SSHAuthorizedKeys",
		"ssh_authorized_keys:\n- key1\n- key2\n- key3\n",
		func(cfg *cloudinit.Config) {
			cfg.AddSSHAuthorizedKeys("#command\nkey1")
			cfg.AddSSHAuthorizedKeys("key2\n# comment\n\nkey3\n")
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
			cfg.AddAptSource("keyName", "someKey")
		},
	},
	{
		"AptSources",
		"apt_sources:\n- source: keyName\n  keyid: someKey\n  keyserver: foo.com\n",
		func(cfg *cloudinit.Config) {
			cfg.AddAptSourceWithKeyId("keyName", "someKey", "foo.com")
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
		"AddFile",
		addFileExpected,
		func(cfg *cloudinit.Config) {
			cfg.AddFile(
				"/etc/apt/apt.conf.d/99proxy",
				`"Acquire::http::Proxy "http://10.0.3.1:3142";`,
				0644,
			)
		},
	},
}

const (
	header          = "#cloud-config\n"
	addFileExpected = `runcmd:
- install -m 644 /dev/null '/etc/apt/apt.conf.d/99proxy'
- echo '"Acquire::http::Proxy "http://10.0.3.1:3142";' > '/etc/apt/apt.conf.d/99proxy'
`
)

func (S) TestOutput(c *C) {
	for _, t := range ctests {
		cfg := cloudinit.New()
		t.setOption(cfg)
		data, err := cfg.Render()
		c.Assert(err, IsNil)
		c.Assert(data, NotNil)
		c.Assert(string(data), Equals, header+t.expect, Commentf("test %q output differs", t.name))
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
	data, err := cfg.Render()
	if err != nil {
		fmt.Printf("render error: %v", err)
		return
	}
	fmt.Printf("%s", data)
}
