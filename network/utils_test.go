// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !windows

package network_test

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/network"
)

type UtilsSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&UtilsSuite{})

func (*UtilsSuite) TestParseResolvConfEmptyOrMissingPath(c *gc.C) {
	emptyPath := ""
	missingPath := filepath.Join(c.MkDir(), "missing")

	for _, path := range []string{emptyPath, missingPath} {
		result, err := network.ParseResolvConf(path)
		c.Check(err, jc.ErrorIsNil)
		c.Check(result, gc.IsNil)
	}
}

func (*UtilsSuite) TestParseResolvConfNotReadablePath(c *gc.C) {
	unreadableConf := makeResolvConf(c, "#empty", 0000)
	result, err := network.ParseResolvConf(unreadableConf)
	expected := fmt.Sprintf("open %s: permission denied", unreadableConf)
	c.Check(err, gc.ErrorMatches, expected)
	c.Check(result, gc.IsNil)
}

func makeResolvConf(c *gc.C, content string, perms os.FileMode) string {
	fakeConfPath := filepath.Join(c.MkDir(), "fake")
	err := ioutil.WriteFile(fakeConfPath, []byte(content), perms)
	c.Check(err, jc.ErrorIsNil)
	return fakeConfPath
}

func (*UtilsSuite) TestParseResolvConfEmptyFile(c *gc.C) {
	emptyConf := makeResolvConf(c, "", 0644)
	result, err := network.ParseResolvConf(emptyConf)
	c.Check(err, jc.ErrorIsNil)
	// Expected non-nil, but empty result.
	c.Check(result, jc.DeepEquals, &network.DNSConfig{})
}

func (*UtilsSuite) TestParseResolvConfCommentsAndWhitespaceHandling(c *gc.C) {
	const exampleConf = `
  ;; comment
# also comment
;# ditto
  #nameserver ;still comment

  search    foo example.com       bar.     ;comment, leading/trailing ignored
nameserver 8.8.8.8 #comment #still the same comment
`
	fakeConf := makeResolvConf(c, exampleConf, 0644)
	result, err := network.ParseResolvConf(fakeConf)
	c.Check(err, jc.ErrorIsNil)
	c.Check(result, jc.DeepEquals, &network.DNSConfig{
		Nameservers:   network.NewAddresses("8.8.8.8"),
		SearchDomains: []string{"foo", "example.com", "bar."},
	})
}

func (*UtilsSuite) TestParseResolvConfSearchWithoutValue(c *gc.C) {
	badConf := makeResolvConf(c, "search # no value\n", 0644)
	result, err := network.ParseResolvConf(badConf)
	c.Check(err, gc.ErrorMatches, `parsing ".*", line 1: "search": required value\(s\) missing`)
	c.Check(result, gc.IsNil)
}

func (*UtilsSuite) TestParseResolvConfNameserverWithoutValue(c *gc.C) {
	badConf := makeResolvConf(c, "nameserver", 0644)
	result, err := network.ParseResolvConf(badConf)
	c.Check(err, gc.ErrorMatches, `parsing ".*", line 1: "nameserver": required value\(s\) missing`)
	c.Check(result, gc.IsNil)
}

func (*UtilsSuite) TestParseResolvConfValueFollowedByCommentWithoutWhitespace(c *gc.C) {
	badConf := makeResolvConf(c, "search foo bar#bad rest;is#ignored: still part of the comment", 0644)
	result, err := network.ParseResolvConf(badConf)
	c.Check(err, gc.ErrorMatches, `parsing ".*", line 1: "search": invalid value "bar#bad"`)
	c.Check(result, gc.IsNil)
}

func (*UtilsSuite) TestParseResolvConfNameserverWithMultipleValues(c *gc.C) {
	badConf := makeResolvConf(c, "nameserver one two 42 ;;; comment still-inside-comment\n", 0644)
	result, err := network.ParseResolvConf(badConf)
	c.Check(err, gc.ErrorMatches, `parsing ".*", line 1: one value expected for "nameserver", got 3`)
	c.Check(result, gc.IsNil)
}

func (*UtilsSuite) TestParseResolvConfLastSearchWins(c *gc.C) {
	const multiSearchConf = `
search zero five
search one
# this below overrides all of the above
search two three #comment ;also-comment still-comment
`
	fakeConf := makeResolvConf(c, multiSearchConf, 0644)
	result, err := network.ParseResolvConf(fakeConf)
	c.Check(err, jc.ErrorIsNil)
	c.Check(result, jc.DeepEquals, &network.DNSConfig{
		SearchDomains: []string{"two", "three"},
	})
}

func (s *UtilsSuite) TestSupportsIPv6Error(c *gc.C) {
	s.PatchValue(network.NetListen, func(netFamily, bindAddress string) (net.Listener, error) {
		c.Check(netFamily, gc.Equals, "tcp6")
		c.Check(bindAddress, gc.Equals, "[::1]:0")
		return nil, errors.New("boom!")
	})
	c.Check(network.SupportsIPv6(), jc.IsFalse)
}

func (s *UtilsSuite) TestSupportsIPv6OK(c *gc.C) {
	s.PatchValue(network.NetListen, func(_, _ string) (net.Listener, error) {
		return &mockListener{}, nil
	})
	c.Check(network.SupportsIPv6(), jc.IsTrue)
}

func (*UtilsSuite) TestParseInterfaceType(c *gc.C) {
	fakeSysPath := filepath.Join(c.MkDir(), network.SysClassNetPath)
	err := os.MkdirAll(fakeSysPath, 0700)
	c.Check(err, jc.ErrorIsNil)

	writeFakeUEvent := func(interfaceName string, lines ...string) string {
		fakeInterfacePath := filepath.Join(fakeSysPath, interfaceName)
		err := os.MkdirAll(fakeInterfacePath, 0700)
		c.Check(err, jc.ErrorIsNil)

		fakeUEventPath := filepath.Join(fakeInterfacePath, "uevent")
		contents := strings.Join(lines, "\n")
		err = ioutil.WriteFile(fakeUEventPath, []byte(contents), 0644)
		c.Check(err, jc.ErrorIsNil)
		return fakeUEventPath
	}

	result := network.ParseInterfaceType(fakeSysPath, "missing")
	c.Check(result, gc.Equals, network.UnknownInterface)

	writeFakeUEvent("eth0", "IFINDEX=1", "INTERFACE=eth0")
	result = network.ParseInterfaceType(fakeSysPath, "eth0")
	c.Check(result, gc.Equals, network.UnknownInterface)

	fakeUEventPath := writeFakeUEvent("eth0.42", "DEVTYPE=vlan")
	result = network.ParseInterfaceType(fakeSysPath, "eth0.42")
	c.Check(result, gc.Equals, network.VLAN_8021QInterface)

	os.Chmod(fakeUEventPath, 0000) // permission denied error is OK
	result = network.ParseInterfaceType(fakeSysPath, "eth0.42")
	c.Check(result, gc.Equals, network.UnknownInterface)

	writeFakeUEvent("bond0", "DEVTYPE=bond")
	result = network.ParseInterfaceType(fakeSysPath, "bond0")
	c.Check(result, gc.Equals, network.BondInterface)

	writeFakeUEvent("br-ens4", "DEVTYPE=bridge")
	result = network.ParseInterfaceType(fakeSysPath, "br-ens4")
	c.Check(result, gc.Equals, network.BridgeInterface)

	// First DEVTYPE found wins.
	writeFakeUEvent("foo", "DEVTYPE=vlan", "DEVTYPE=bridge")
	result = network.ParseInterfaceType(fakeSysPath, "foo")
	c.Check(result, gc.Equals, network.VLAN_8021QInterface)

	writeFakeUEvent("fake", "DEVTYPE=warp-drive")
	result = network.ParseInterfaceType(fakeSysPath, "fake")
	c.Check(result, gc.Equals, network.UnknownInterface)
}

func (*UtilsSuite) TestGetBridgePorts(c *gc.C) {
	fakeSysPath := filepath.Join(c.MkDir(), network.SysClassNetPath)
	err := os.MkdirAll(fakeSysPath, 0700)
	c.Check(err, jc.ErrorIsNil)

	writeFakePorts := func(bridgeName string, portNames ...string) {
		fakePortsPath := filepath.Join(fakeSysPath, bridgeName, "brif")
		err := os.MkdirAll(fakePortsPath, 0700)
		c.Check(err, jc.ErrorIsNil)

		for _, portName := range portNames {
			portPath := filepath.Join(fakePortsPath, portName)
			err = ioutil.WriteFile(portPath, []byte(""), 0644)
			c.Check(err, jc.ErrorIsNil)
		}
	}

	result := network.GetBridgePorts(fakeSysPath, "missing")
	c.Check(result, gc.IsNil)

	writeFakePorts("br-eth0")
	result = network.GetBridgePorts(fakeSysPath, "br-eth0")
	c.Check(result, gc.IsNil)

	writeFakePorts("br-eth0", "eth0")
	result = network.GetBridgePorts(fakeSysPath, "br-eth0")
	c.Check(result, jc.DeepEquals, []string{"eth0"})

	writeFakePorts("br-ovs", "eth0", "eth1", "eth2")
	result = network.GetBridgePorts(fakeSysPath, "br-ovs")
	c.Check(result, jc.DeepEquals, []string{"eth0", "eth1", "eth2"})
}

type mockListener struct {
	net.Listener
}

func (*mockListener) Close() error {
	return nil
}
