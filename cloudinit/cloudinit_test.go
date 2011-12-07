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
	option cloudinit.Option
}{
	{"user: me\n",
		"User", cloudinit.User("me")},
	{"apt_upgrade: true\n",
		"AptUpgrade", cloudinit.AptUpgrade(true)},
	{"apt_update: true\n",
		"AptUpdate", cloudinit.AptUpdate(true)},
	{"apt_mirror: http://foo.com\n",
		"AptMirror", cloudinit.AptMirror("http://foo.com")},
	{"apt_mirror: true\n",
		"AptPreserveSourcesList", cloudinit.AptPreserveSourcesList(true)},
	{"apt_old_mirror: http://foo.com\n",
		"AptOldMirror", cloudinit.AptOldMirror("http://foo.com")},
	{"apt_sources:\n- source: keyName\n  key: someKey\n",
		"AptSources", cloudinit.AptSources(cloudinit.NewSource("keyName", "someKey"))},
	{"apt_sources:\n- source: keyName\n  keyid: someKey\n  keyserver: foo.com\n",
		"AptSources", cloudinit.AptSources(cloudinit.NewSourceWithKeyId("keyName", "someKey", "foo.com"))},
	{"debconf_selections: true\n",
		"DebConfSelections", cloudinit.DebConfSelections(true)},
	{"packages:\n- juju\n- ubuntu\n",
		"Packages", cloudinit.Packages("juju", "ubuntu")},
	{"bootcmd:\n- ls > /dev\n- - ls\n  - '>with space'\n",
		"BootCmd", cloudinit.BootCmd(cloudinit.NewLiteralCommand("ls > /dev"), cloudinit.NewArgListCommand("ls", ">with space"))},
	{"disable_ec2_metadata: true\n",
		"DisableEC2Metadata", cloudinit.DisableEC2Metadata(true)},
	{"final_message: goodbye\n",
		"FinalMessage", cloudinit.FinalMessage("goodbye")},
	{"locale: en_us\n",
		"Locale", cloudinit.Locale("en_us")},
	{"mounts:\n- - x\n  - \"y\"\n- - z\n  - w\n",
		"Mounts", cloudinit.Mounts([][]string{{"x", "y"}, {"z", "w"}})},
	{"output:\n  all:\n    stdout: '>foo'\n    stderr: '|bar'\n",
		"Output", cloudinit.Output(map[string]cloudinit.OutputSpec{"all": {">foo", "|bar"}})},
	{"ssh_keys:\n- - rsa_private\n  - key1data\n- - dsa_public\n  - key2data\n",
		"SSHKeys", cloudinit.SSHKeys([]cloudinit.Key{
			{cloudinit.RSA | cloudinit.Private, "key1data"},
			{cloudinit.DSA | cloudinit.Public, "key2data"},
		})},
	{"disable_root: false\n",
		"DisableRoot", cloudinit.DisableRoot(false)},
	{"ssh_authorized_keys:\n- key1\n- key2\n",
		"SSHAuthorizedKeys", cloudinit.SSHAuthorizedKeys("key1", "key2")},
}

const header = "#cloud-config\n"

func (S) TestOutput(c *C) {
	for _, t := range ctests {
		cfg := cloudinit.New()
		cfg.Set(t.option)
		data, err := cfg.Render()
		c.Assert(err, IsNil)
		c.Assert(data, NotNil)
		c.Assert(string(data), Equals, header+t.expect, Bug("test %q output differs", t.name))
	}
}

var atests = []struct {
	expect  string
	name    string
	options []cloudinit.Option
}{
	{"ssh_authorized_keys:\n- key1\n- key2\n- key3\n- key4\n",
		"SSHAuthorizedKeys",
		[]cloudinit.Option{
			cloudinit.SSHAuthorizedKeys("key1", "key2"),
			cloudinit.SSHAuthorizedKeys("key3", "key4"),
		},
	},
	{"apt_sources:\n- source: keyName\n  keyid: someKey\n  keyserver: foo.com\n- source: keyName\n  key: someKey\n",
		"AptSources",
		[]cloudinit.Option{
			cloudinit.AptSources(cloudinit.NewSourceWithKeyId("keyName", "someKey", "foo.com")),
			cloudinit.AptSources(cloudinit.NewSource("keyName", "someKey")),
		},
	},
}

func (S) TestAppend(c *C) {
	for _, t := range atests {
		cfg := cloudinit.New()
		cfg.Set(t.options[0])
		for _, o := range t.options[1:] {
			cfg.Append(o)
		}
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
	cfg.Set(cloudinit.Packages("juju", "ubuntu"))
	data, err := cfg.Render()
	if err != nil {
		fmt.Printf("render error: %v", err)
		return
	}
	fmt.Printf("%s", data)
}
