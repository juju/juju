// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package networker_test

import (
	"fmt"
	"io/ioutil"
	"os"

	gc "launchpad.net/gocheck"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/worker/networker"
)

type configSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&configSuite{})

const interfacesTemplate = `# Blah-blah

# The loopback network interface
auto lo
iface lo inet loopback

auto eth1
source %s/eth1.config
auto eth0
source %s/eth0.config

auto eth1.2
iface eth1.2 inet dhcp

auto eth2
iface eth2 inet dhcp
`
const eth0DotConfigContents = `iface eth0 inet manual

auto br0
iface br0 inet dhcp
  bridge_ports eth0
`
const eth1DotConfigContents = `iface eth1 inet manual

auto br2
iface br2 inet dhcp
  bridge_ports eth1
`
const interfacesDSlashEth0DotCfgContents = `auto eth0
iface eth0 inet dhcp
`
const interfacesDSlashEth4DotCfgContents = `auto eth4
iface eth4 inet dhcp
`

const interfacesExpectedPrefix = `# Blah-blah

# The loopback network interface
auto lo
iface lo inet loopback

`

func (s *configSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	// Create temporary directory to store interfaces file.
	networker.ChangeConfigDirName(c.MkDir())
}

func (s *configSuite) TestConfigFileOperations(c *gc.C) {
	// Create sample config files
	interfacesContents := fmt.Sprintf(interfacesTemplate, networker.ConfigDirName, networker.ConfigDirName)
	err := ioutil.WriteFile(networker.ConfigFileName, []byte(interfacesContents), 0644)
	c.Assert(err, gc.IsNil)
	err = ioutil.WriteFile(networker.ConfigDirName+"/eth0.config", []byte(eth0DotConfigContents), 0644)
	c.Assert(err, gc.IsNil)
	err = ioutil.WriteFile(networker.ConfigDirName+"/eth1.config", []byte(eth1DotConfigContents), 0644)
	c.Assert(err, gc.IsNil)
	err = os.Mkdir(networker.ConfigSubDirName, 0755)
	c.Assert(err, gc.IsNil)
	err = ioutil.WriteFile(networker.ConfigSubDirName+"/eth0.cfg", []byte(interfacesDSlashEth0DotCfgContents), 0644)
	c.Assert(err, gc.IsNil)
	err = ioutil.WriteFile(networker.ConfigSubDirName+"/eth4.cfg", []byte(interfacesDSlashEth4DotCfgContents), 0644)
	c.Assert(err, gc.IsNil)

	cf := networker.ConfigFiles{}
	err = networker.ReadAll(&cf)
	c.Assert(err, gc.IsNil)
	expect := networker.ConfigFiles{
		"": &networker.ConfigFile{
			FileName: networker.ConfigFileName,
			Data:     interfacesContents,
		},
		"eth0": &networker.ConfigFile{
			FileName: networker.ConfigSubDirName + "/eth0.cfg",
			Data:     interfacesDSlashEth0DotCfgContents,
		},
		"eth4": &networker.ConfigFile{
			FileName: networker.ConfigSubDirName + "/eth4.cfg",
			Data:     interfacesDSlashEth4DotCfgContents,
		},
	}
	c.Assert(cf, gc.DeepEquals, expect)
	err = networker.FixMAAS(cf)
	c.Assert(err, gc.IsNil)
	expect = networker.ConfigFiles{
		"": &networker.ConfigFile{
			FileName: networker.ConfigFileName,
			Data: interfacesExpectedPrefix +
				fmt.Sprintf(networker.SourceCommentAndCommand,
					networker.ConfigSubDirName, networker.ConfigSubDirName,
					networker.ConfigSubDirName, networker.ConfigSubDirName),
			Op: networker.DoWrite,
		},
		"eth0": &networker.ConfigFile{
			FileName: networker.ConfigSubDirName + "/eth0.cfg",
			Data:     "auto eth0\niface eth0 inet manual\n",
			Op:       networker.DoWrite,
		},
		"br0": &networker.ConfigFile{
			FileName: networker.ConfigSubDirName + "/br0.cfg",
			Data:     "auto br0\niface br0 inet dhcp\n  bridge_ports eth0\n",
			Op:       networker.DoWrite,
		},
		"eth1": &networker.ConfigFile{
			FileName: networker.ConfigSubDirName + "/eth1.cfg",
			Data:     "auto eth1\niface eth1 inet manual\n",
			Op:       networker.DoWrite,
		},
		"br2": &networker.ConfigFile{
			FileName: networker.ConfigSubDirName + "/br2.cfg",
			Data:     "auto br2\niface br2 inet dhcp\n  bridge_ports eth1\n",
			Op:       networker.DoWrite,
		},
		"eth1.2": &networker.ConfigFile{
			FileName: networker.ConfigSubDirName + "/eth1.2.cfg",
			Data:     "auto eth1.2\niface eth1.2 inet dhcp\n",
			Op:       networker.DoWrite,
		},
		"eth2": &networker.ConfigFile{
			FileName: networker.ConfigSubDirName + "/eth2.cfg",
			Data:     "auto eth2\niface eth2 inet dhcp\n",
			Op:       networker.DoWrite,
		},
		"eth4": &networker.ConfigFile{
			FileName: networker.ConfigSubDirName + "/eth4.cfg",
			Data:     "",
			Op:       networker.DoRemove,
		},
		fmt.Sprintf("#%s/eth0.config", networker.ConfigDirName): &networker.ConfigFile{
			FileName: networker.ConfigDirName + "/eth0.config",
			Data:     "",
			Op:       networker.DoRemove,
		},
		fmt.Sprintf("#%s/eth1.config", networker.ConfigDirName): &networker.ConfigFile{
			FileName: networker.ConfigDirName + "/eth1.config",
			Data:     "",
			Op:       networker.DoRemove,
		},
	}
	c.Assert(cf, gc.DeepEquals, expect)
	err = networker.WriteOrRemove(cf)
	c.Assert(err, gc.IsNil)

	// Do another ineration
	cf = networker.ConfigFiles{}
	err = networker.ReadAll(&cf)
	c.Assert(err, gc.IsNil)
	expect = networker.ConfigFiles{
		"": &networker.ConfigFile{
			FileName: networker.ConfigFileName,
			Data: interfacesExpectedPrefix +
				fmt.Sprintf(networker.SourceCommentAndCommand,
					networker.ConfigSubDirName, networker.ConfigSubDirName,
					networker.ConfigSubDirName, networker.ConfigSubDirName),
		},
		"eth0": &networker.ConfigFile{
			FileName: networker.ConfigSubDirName + "/eth0.cfg",
			Data:     "auto eth0\niface eth0 inet manual\n",
		},
		"br0": &networker.ConfigFile{
			FileName: networker.ConfigSubDirName + "/br0.cfg",
			Data:     "auto br0\niface br0 inet dhcp\n  bridge_ports eth0\n",
		},
		"eth1": &networker.ConfigFile{
			FileName: networker.ConfigSubDirName + "/eth1.cfg",
			Data:     "auto eth1\niface eth1 inet manual\n",
		},
		"br2": &networker.ConfigFile{
			FileName: networker.ConfigSubDirName + "/br2.cfg",
			Data:     "auto br2\niface br2 inet dhcp\n  bridge_ports eth1\n",
		},
		"eth1.2": &networker.ConfigFile{
			FileName: networker.ConfigSubDirName + "/eth1.2.cfg",
			Data:     "auto eth1.2\niface eth1.2 inet dhcp\n",
		},
		"eth2": &networker.ConfigFile{
			FileName: networker.ConfigSubDirName + "/eth2.cfg",
			Data:     "auto eth2\niface eth2 inet dhcp\n",
		},
	}
	c.Assert(cf, gc.DeepEquals, expect)

	// fixMAAS should not change anything
	err = networker.FixMAAS(cf)
	c.Assert(err, gc.IsNil)
	c.Assert(cf, gc.DeepEquals, expect)
}

const interfacesData = `# comment 1
auto eth0.1 eth0
aaa
allow-x eth1:2
bbb
# comment 2

# comment 3
iface eth0.1
ccc
source eth2.cfg
ddd
mapping eth1:2
eee
source-directory somedir
fff
`

func (s *configSuite) TestSplitByInterfaces(c *gc.C) {
	split, err := networker.SplitByInterfaces(interfacesData)
	expect := map[string]string{
		"":       "source eth2.cfg\nddd\nsource-directory somedir\nfff\n",
		"eth0.1": "# comment 1\nauto eth0.1 eth0\naaa\n# comment 3\niface eth0.1\nccc\n",
		"eth1:2": "allow-x eth1:2\nbbb\n# comment 2\n\nmapping eth1:2\neee\n",
	}
	c.Assert(err, gc.IsNil)
	c.Assert(split, gc.DeepEquals, expect)
}
