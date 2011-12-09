package cloudinit_test

import (
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/cloudinit"
	"testing"
)
// TODO integration tests, but how?

type S struct{}

var _ = Suite(S{})

func Test1(t *testing.T) {
	TestingT(t)
}

var ctests = []struct {
	expect string
	name   string
	setOption func(cfg *cloudinit.Config)
}{
	{"user: me\n",
		"User", func(cfg *cloudinit.Config){cfg.SetUser("me")}},
	{"apt_upgrade: true\n",
		"AptUpgrade", func(cfg *cloudinit.Config){cfg.SetAptUpgrade(true)}},
	{"apt_update: true\n",
		"AptUpdate", func(cfg *cloudinit.Config){cfg.SetAptUpdate(true)}},
	{"apt_mirror: http://foo.com\n",
		"AptMirror", func(cfg *cloudinit.Config){cfg.SetAptMirror("http://foo.com")}},
	{"apt_mirror: true\n",
		"AptPreserveSourcesList", func(cfg *cloudinit.Config){cfg.SetAptPreserveSourcesList(true)}},
	{"apt_old_mirror: http://foo.com\n",
		"AptOldMirror", func(cfg *cloudinit.Config){cfg.SetAptOldMirror("http://foo.com")}},
	{"debconf_selections: true\n",
		"DebConfSelections", func(cfg *cloudinit.Config){cfg.SetDebConfSelections(true)}},
	{"disable_ec2_metadata: true\n",
		"DisableEC2Metadata", func(cfg *cloudinit.Config){cfg.SetDisableEC2Metadata(true)}},
	{"final_message: goodbye\n",
		"FinalMessage", func(cfg *cloudinit.Config){cfg.SetFinalMessage("goodbye")}},
	{"locale: en_us\n",
		"Locale", func(cfg *cloudinit.Config){cfg.SetLocale("en_us")}},
	{"disable_root: false\n",
		"DisableRoot", func(cfg *cloudinit.Config){cfg.SetDisableRoot(false)}},


	{"ssh_authorized_keys:\n- key1\n- key2\n",
		"SSHAuthorizedKeys",
		func(cfg *cloudinit.Config){
			cfg.AddSSHAuthorizedKey("key1")
			cfg.AddSSHAuthorizedKey("key2")
		}},
	{"ssh_keys:\n- - rsa_private\n  - key1data\n- - rsa_private\n  - key2data\n",
		"SSHKeys RSA private", 
		func(cfg *cloudinit.Config){
			cfg.AddSSHKey(cloudinit.RSA, true, "key1data")
			cfg.AddSSHKey(cloudinit.RSA, true, "key2data")
		}},
	{"ssh_keys:\n- - dsa_public\n  - key1data\n- - dsa_public\n  - key2data\n",
		"SSHKeys DSA public",
		func(cfg *cloudinit.Config){
			cfg.AddSSHKey(cloudinit.DSA, false, "key1data")
			cfg.AddSSHKey(cloudinit.DSA, false, "key2data")
		}},
	{"output:\n  all:\n  - '>foo'\n  - '|bar'\n",
		"Output",
		func(cfg *cloudinit.Config){
			cfg.SetOutput("all", ">foo", "|bar")
		}},
	{"output:\n  all: '>foo'\n",
		"Output",
		func(cfg *cloudinit.Config){
			cfg.SetOutput("all", ">foo", "")
		}},
	{"apt_sources:\n- source: keyName\n  key: someKey\n",
		"AptSources", func(cfg *cloudinit.Config){cfg.AddAptSource("keyName", "someKey")}},
	{"apt_sources:\n- source: keyName\n  keyid: someKey\n  keyserver: foo.com\n",
		"AptSources", func(cfg *cloudinit.Config){cfg.AddAptSourceWithKeyId("keyName", "someKey", "foo.com")}},
	{"packages:\n- juju\n- ubuntu\n",
		"Packages",
		func(cfg *cloudinit.Config){
			cfg.AddPackage("juju")
			cfg.AddPackage("ubuntu")
		}},
	{"bootcmd:\n- ls > /dev\n- - ls\n  - '>with space'\n",
		"BootCmd", func(cfg *cloudinit.Config){
			cfg.AddBootCmd("ls > /dev")
			cfg.AddBootCmdArgs("ls", ">with space")
		}},
	{"mounts:\n- - x\n  - \"y\"\n- - z\n  - w\n",
		"Mounts", func(cfg *cloudinit.Config){
			cfg.AddMount("x", "y")
			cfg.AddMount("z", "w")
		}},
}

const header = "#cloud-config\n"

func (S) TestOutput(c *C) {
	for _, t := range ctests {
		cfg := cloudinit.New()
		t.setOption(cfg)
		data, err := cfg.Render()
		c.Assert(err, IsNil)
		c.Assert(data, NotNil)
		c.Assert(string(data), Equals, header+t.expect, Bug("test %q output differs", t.name))
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
