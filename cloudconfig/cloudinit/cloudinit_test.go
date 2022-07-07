// Copyright 2011, 2012, 2013, 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudinit_test

import (
	"fmt"

	"github.com/juju/packaging/v2"
	jc "github.com/juju/testing/checkers"
	sshtesting "github.com/juju/utils/v3/ssh/testing"
	"golang.org/x/crypto/ssh"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloudconfig/cloudinit"
	coretesting "github.com/juju/juju/testing"
)

// TODO integration tests, but how?

type S struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(S{})

var ctests = []struct {
	name      string
	expect    map[string]any
	setOption func(cfg cloudinit.CloudConfig) error
}{{
	"PackageUpgrade",
	map[string]any{
		"package_upgrade": true,
	},
	func(cfg cloudinit.CloudConfig) error {
		cfg.SetSystemUpgrade(true)
		return nil
	},
}, {
	"PackageUpdate",
	map[string]any{
		"package_update": true,
	},
	func(cfg cloudinit.CloudConfig) error {
		cfg.SetSystemUpdate(true)
		return nil
	},
}, {
	"PackageProxy",
	map[string]any{
		"apt_proxy": "http://foo.com",
	},
	func(cfg cloudinit.CloudConfig) error {
		cfg.SetPackageProxy("http://foo.com")
		return nil
	},
}, {
	"PackageMirror",
	map[string]any{
		"apt_mirror": "http://foo.com",
	},
	func(cfg cloudinit.CloudConfig) error {
		cfg.SetPackageMirror("http://foo.com")
		return nil
	},
}, {
	"DisableEC2Metadata",
	map[string]any{
		"disable_ec2_metadata": true,
	},
	func(cfg cloudinit.CloudConfig) error {
		cfg.SetDisableEC2Metadata(true)
		return nil
	},
}, {
	"FinalMessage",
	map[string]any{
		"final_message": "goodbye",
	},
	func(cfg cloudinit.CloudConfig) error {
		cfg.SetFinalMessage("goodbye")
		return nil
	},
}, {
	"Locale",
	map[string]any{
		"locale": "en_us",
	},
	func(cfg cloudinit.CloudConfig) error {
		cfg.SetLocale("en_us")
		return nil
	},
}, {
	"DisableRoot",
	map[string]any{
		"disable_root": false,
	},
	func(cfg cloudinit.CloudConfig) error {
		cfg.SetDisableRoot(false)
		return nil
	},
}, {
	"SetSSHAuthorizedKeys with two keys",
	map[string]any{
		"ssh_authorized_keys": []string{
			fmt.Sprintf("%s Juju:user@host", sshtesting.ValidKeyOne.Key),
			fmt.Sprintf("%s Juju:another@host", sshtesting.ValidKeyTwo.Key),
		},
	},
	func(cfg cloudinit.CloudConfig) error {
		cfg.SetSSHAuthorizedKeys(
			sshtesting.ValidKeyOne.Key + " Juju:user@host\n" +
				sshtesting.ValidKeyTwo.Key + " another@host")
		return nil
	},
}, {
	"SetSSHAuthorizedKeys with comments in keys",
	map[string]any{
		"ssh_authorized_keys": []string{
			fmt.Sprintf("%s Juju:sshkey", sshtesting.ValidKeyOne.Key),
			fmt.Sprintf("%s Juju:user@host", sshtesting.ValidKeyTwo.Key),
			fmt.Sprintf("%s Juju:another@host", sshtesting.ValidKeyThree.Key),
		},
	},
	func(cfg cloudinit.CloudConfig) error {
		cfg.SetSSHAuthorizedKeys(
			"#command\n" + sshtesting.ValidKeyOne.Key + "\n" +
				sshtesting.ValidKeyTwo.Key + " user@host\n" +
				"# comment\n\n" +
				sshtesting.ValidKeyThree.Key + " another@host")
		return nil
	},
}, {
	"SetSSHAuthorizedKeys unsets keys",
	nil,
	func(cfg cloudinit.CloudConfig) error {
		cfg.SetSSHAuthorizedKeys(sshtesting.ValidKeyOne.Key)
		cfg.SetSSHAuthorizedKeys("")
		return nil
	},
}, {
	"AddUser with keys",
	map[string]any{
		"users": []any{
			map[string]any{
				"name":        "auser",
				"lock_passwd": true,
				"ssh-authorized-keys": []string{
					fmt.Sprintf("%s Juju:user@host", sshtesting.ValidKeyOne.Key),
					fmt.Sprintf("%s Juju:another@host", sshtesting.ValidKeyTwo.Key),
				},
			},
		},
	},
	func(cfg cloudinit.CloudConfig) error {
		keys := (sshtesting.ValidKeyOne.Key + " Juju:user@host\n" +
			sshtesting.ValidKeyTwo.Key + " another@host")
		cfg.AddUser(&cloudinit.User{
			Name:              "auser",
			SSHAuthorizedKeys: keys,
		})
		return nil
	},
}, {
	"AddUser with groups",
	map[string]any{
		"users": []any{
			map[string]any{
				"name":        "auser",
				"lock_passwd": true,
				"groups":      []string{"agroup", "bgroup"},
			},
		},
	},
	func(cfg cloudinit.CloudConfig) error {
		cfg.AddUser(&cloudinit.User{
			Name:   "auser",
			Groups: []string{"agroup", "bgroup"},
		})
		return nil
	},
}, {
	"AddUser with everything",
	map[string]any{
		"users": []any{
			map[string]any{
				"name":        "auser",
				"lock_passwd": true,
				"groups":      []string{"agroup", "bgroup"},
				"shell":       "/bin/sh",
				"ssh-authorized-keys": []string{
					sshtesting.ValidKeyOne.Key + " Juju:sshkey",
				},
				"sudo": []string{"ALL=(ALL) ALL"},
			},
		},
	},
	func(cfg cloudinit.CloudConfig) error {
		cfg.AddUser(&cloudinit.User{
			Name:              "auser",
			Groups:            []string{"agroup", "bgroup"},
			Shell:             "/bin/sh",
			SSHAuthorizedKeys: sshtesting.ValidKeyOne.Key + "\n",
			Sudo:              []string{"ALL=(ALL) ALL"},
		})
		return nil
	},
}, {
	"AddUser with only name",
	map[string]any{
		"users": []any{
			map[string]any{
				"name":        "auser",
				"lock_passwd": true,
			},
		},
	},
	func(cfg cloudinit.CloudConfig) error {
		cfg.AddUser(&cloudinit.User{
			Name: "auser",
		})
		return nil
	},
}, {
	"Output",
	map[string]any{
		"output": map[string]any{
			"all": []string{">foo", "|bar"},
		},
	},
	func(cfg cloudinit.CloudConfig) error {
		cfg.SetOutput("all", ">foo", "|bar")
		return nil
	},
}, {
	"Output",
	map[string]any{
		"output": map[string]any{
			"all": ">foo",
		},
	},
	func(cfg cloudinit.CloudConfig) error {
		cfg.SetOutput(cloudinit.OutAll, ">foo", "")
		return nil
	},
}, {
	"PackageSources",
	map[string]any{
		"apt_sources": []map[string]any{
			{
				"source": "keyName",
				"key":    "someKey",
			},
		},
	},
	func(cfg cloudinit.CloudConfig) error {
		cfg.AddPackageSource(packaging.PackageSource{URL: "keyName", Key: "someKey"})
		return nil
	},
}, {
	"PackageSources with preferences",
	map[string]any{
		"apt_sources": []map[string]any{
			{
				"source": "keyName",
				"key":    "someKey",
			},
		},
		"bootcmd": []string{
			"install -D -m 644 /dev/null '/some/path'",
			"echo 'Explanation: test\n" +
				"Package: *\n" +
				"Pin: release n=series\n" +
				"Pin-Priority: 123\n" +
				"' > '/some/path'",
		},
	},
	func(cfg cloudinit.CloudConfig) error {
		prefs := packaging.PackagePreferences{
			Path:        "/some/path",
			Explanation: "test",
			Package:     "*",
			Pin:         "release n=series",
			Priority:    123,
		}
		cfg.AddPackageSource(packaging.PackageSource{URL: "keyName", Key: "someKey"})
		cfg.AddPackagePreferences(prefs)
		return nil
	},
}, {
	"Packages",
	map[string]any{
		"packages": []string{
			"juju",
			"ubuntu",
		},
	},
	func(cfg cloudinit.CloudConfig) error {
		cfg.AddPackage("juju")
		cfg.AddPackage("ubuntu")
		return nil
	},
}, {
	"BootCmd",
	map[string]any{
		"bootcmd": []string{
			"ls > /dev",
			"ls >with space",
		},
	},
	func(cfg cloudinit.CloudConfig) error {
		cfg.AddBootCmd("ls > /dev")
		cfg.AddBootCmd("ls >with space")
		return nil
	},
}, {
	"Mounts",
	map[string]any{
		"mounts": [][]string{
			{"x", "y"},
			{"z", "w"},
		},
	},
	func(cfg cloudinit.CloudConfig) error {
		cfg.AddMount("x", "y")
		cfg.AddMount("z", "w")
		return nil
	},
}, {
	"Attr",
	map[string]any{
		"arbitraryAttr": "someValue"},
	func(cfg cloudinit.CloudConfig) error {
		cfg.SetAttr("arbitraryAttr", "someValue")
		return nil
	},
}, {
	"RunCmd",
	map[string]any{
		"runcmd": []string{
			"ifconfig",
		},
	},
	func(cfg cloudinit.CloudConfig) error {
		cfg.AddRunCmd("ifconfig")
		return nil
	},
}, {
	"PrependRunCmd",
	map[string]any{
		"runcmd": []string{
			"echo 'Hello World'",
			"ifconfig",
		},
	},
	func(cfg cloudinit.CloudConfig) error {
		cfg.AddRunCmd("ifconfig")
		cfg.PrependRunCmd(
			"echo 'Hello World'",
		)
		return nil
	},
}, {
	"AddScripts",
	map[string]any{
		"runcmd": []string{
			"echo 'Hello World'",
			"ifconfig",
		},
	},
	func(cfg cloudinit.CloudConfig) error {
		cfg.AddScripts(
			"echo 'Hello World'",
			"ifconfig",
		)
		return nil
	},
}, {
	"AddTextFile",
	map[string]any{
		"runcmd": []string{
			"install -D -m 644 /dev/null '/etc/apt/apt.conf.d/99proxy'",
			"echo '\"Acquire::http::Proxy \"http://10.0.3.1:3142\";' > '/etc/apt/apt.conf.d/99proxy'",
		},
	},
	func(cfg cloudinit.CloudConfig) error {
		cfg.AddRunTextFile(
			"/etc/apt/apt.conf.d/99proxy",
			`"Acquire::http::Proxy "http://10.0.3.1:3142";`,
			0644,
		)
		return nil
	},
}, {
	"AddBinaryFile",
	map[string]any{
		"runcmd": []string{
			"install -D -m 644 /dev/null '/dev/nonsense'",
			"echo -n AAECAw== | base64 -d > '/dev/nonsense'",
		},
	},
	func(cfg cloudinit.CloudConfig) error {
		cfg.AddRunBinaryFile(
			"/dev/nonsense",
			[]byte{0, 1, 2, 3},
			0644,
		)
		return nil
	},
}, {
	"AddBootTextFile",
	map[string]any{
		"bootcmd": []string{
			"install -D -m 644 /dev/null '/etc/apt/apt.conf.d/99proxy'",
			"echo '\"Acquire::http::Proxy \"http://10.0.3.1:3142\";' > '/etc/apt/apt.conf.d/99proxy'",
		},
	},
	func(cfg cloudinit.CloudConfig) error {
		cfg.AddBootTextFile(
			"/etc/apt/apt.conf.d/99proxy",
			`"Acquire::http::Proxy "http://10.0.3.1:3142";`,
			0644,
		)
		return nil
	},
}, {
	"ManageEtcHosts",
	map[string]any{
		"manage_etc_hosts": true},
	func(cfg cloudinit.CloudConfig) error {
		cfg.ManageEtcHosts(true)
		return nil
	},
}, {
	"SetSSHKeys",
	map[string]any{
		"ssh_keys": map[string]any{
			"rsa_private": "private",
			"rsa_public":  "public",
		},
	},
	func(cfg cloudinit.CloudConfig) error {
		return cfg.SetSSHKeys(cloudinit.SSHKeys{{
			Private:            "private",
			Public:             "public",
			PublicKeyAlgorithm: ssh.KeyAlgoRSA,
		},
		})
	},
}, {
	"SetSSHKeys unsets keys",
	nil,
	func(cfg cloudinit.CloudConfig) error {
		err := cfg.SetSSHKeys(cloudinit.SSHKeys{{
			Private:            "private",
			Public:             "public",
			PublicKeyAlgorithm: ssh.KeyAlgoRSA,
		},
		})
		if err != nil {
			return err
		}
		return cfg.SetSSHKeys(cloudinit.SSHKeys{})
	},
}, {
	"SetSSHKeysMultiple",
	map[string]any{
		"ssh_keys": map[string]any{
			"rsa_private":     "private-rsa",
			"rsa_public":      "public-rsa",
			"ecdsa_private":   "private-ecdsa",
			"ecdsa_public":    "public-ecdsa",
			"ed25519_private": "private-ed25519",
			"ed25519_public":  "public-ed25519",
		},
	},
	func(cfg cloudinit.CloudConfig) error {
		return cfg.SetSSHKeys(cloudinit.SSHKeys{
			{
				Private:            "private-rsa",
				Public:             "public-rsa",
				PublicKeyAlgorithm: ssh.KeyAlgoRSA,
			}, {
				Private:            "private-ecdsa",
				Public:             "public-ecdsa",
				PublicKeyAlgorithm: ssh.KeyAlgoECDSA256,
			}, {
				Private:            "private-ed25519",
				Public:             "public-ed25519",
				PublicKeyAlgorithm: ssh.KeyAlgoED25519,
			},
		})
	},
},
}

func (S) TestOutput(c *gc.C) {
	for i, t := range ctests {
		c.Logf("test %d: %s", i, t.name)
		cfg, err := cloudinit.New("precise")
		c.Assert(err, jc.ErrorIsNil)
		err = t.setOption(cfg)
		c.Assert(err, jc.ErrorIsNil)
		data, err := cfg.RenderYAML()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(data, gc.NotNil)
		c.Assert(string(data), jc.YAMLEquals, t.expect)
		data, err = cfg.RenderYAML()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(data, gc.NotNil)
		c.Assert(string(data), jc.YAMLEquals, t.expect)
	}
}

func (S) TestRunCmds(c *gc.C) {
	cfg, err := cloudinit.New("precise")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.RunCmds(), gc.HasLen, 0)
	cfg.AddScripts("a", "b")
	cfg.AddRunCmd("e")
	c.Assert(cfg.RunCmds(), gc.DeepEquals, []string{
		"a", "b", "e",
	})
}

func (S) TestPackages(c *gc.C) {
	cfg, err := cloudinit.New("precise")
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
	},
	}

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
