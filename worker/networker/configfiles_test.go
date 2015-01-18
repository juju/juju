// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package networker_test

import (
	"io/ioutil"
	"os"
	"path/filepath"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/network"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/worker/networker"
)

type configFilesSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&configFilesSuite{})

func (s *configFilesSuite) TestSimpleGetters(c *gc.C) {
	info := network.InterfaceInfo{
		InterfaceName: "blah",
	}
	data := []byte("some data")
	cf := networker.NewConfigFile("ethX", "/some/path", info, data)
	c.Assert(cf.InterfaceName(), gc.Equals, "ethX")
	c.Assert(cf.FileName(), gc.Equals, "/some/path")
	c.Assert(cf.InterfaceInfo(), jc.DeepEquals, info)
	c.Assert(cf.Data(), jc.DeepEquals, data)
	c.Assert(cf.NeedsUpdating(), jc.IsFalse)
	c.Assert(cf.IsPendingRemoval(), jc.IsFalse)
	c.Assert(cf.IsManaged(), jc.IsFalse)
}

func (s *configFilesSuite) TestRenderManaged(c *gc.C) {
	info := network.InterfaceInfo{
		InterfaceName: "ethX",
		VLANTag:       42,
	}
	cf := networker.NewConfigFile("ethX", "/some/path", info, nil)
	data := cf.RenderManaged()
	expectedVLAN := `
# Managed by Juju, please don't change.

auto ethX.42
iface ethX.42 inet dhcp
	vlan-raw-device ethX

`[1:]
	c.Assert(string(data), jc.DeepEquals, expectedVLAN)

	expectedNormal := `
# Managed by Juju, please don't change.

auto ethX
iface ethX inet dhcp

`[1:]
	info.VLANTag = 0
	cf = networker.NewConfigFile("ethX", "/some/path", info, nil)
	data = cf.RenderManaged()
	c.Assert(string(data), jc.DeepEquals, expectedNormal)
}

func (s *configFilesSuite) TestUpdateData(c *gc.C) {
	cf := networker.NewConfigFile("ethX", "", network.InterfaceInfo{}, nil)
	assertData := func(expectData []byte, expectNeedsUpdating bool) {
		c.Assert(string(cf.Data()), jc.DeepEquals, string(expectData))
		c.Assert(cf.NeedsUpdating(), gc.Equals, expectNeedsUpdating)
		c.Assert(cf.IsPendingRemoval(), jc.IsFalse)
	}

	assertData(nil, false)

	result := cf.UpdateData(nil)
	c.Assert(result, jc.IsFalse)
	assertData(nil, false)

	newData := []byte("new data")
	result = cf.UpdateData(newData)
	c.Assert(result, jc.IsTrue)
	assertData(newData, true)

	newData = []byte("newer data")
	result = cf.UpdateData(newData)
	c.Assert(result, jc.IsTrue)
	assertData(newData, true)
}

func (s *configFilesSuite) TestReadData(c *gc.C) {
	data := []byte("some\ndata\nhere")
	testFile := filepath.Join(c.MkDir(), "test")
	defer os.Remove(testFile)

	err := ioutil.WriteFile(testFile, data, 0644)
	c.Assert(err, jc.ErrorIsNil)
	cf := networker.NewConfigFile("ethX", testFile, network.InterfaceInfo{}, nil)
	err = cf.ReadData()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(cf.Data()), jc.DeepEquals, string(data))
	c.Assert(cf.NeedsUpdating(), jc.IsTrue)
}

func (s *configFilesSuite) TestMarkForRemoval(c *gc.C) {
	cf := networker.NewConfigFile("ethX", "", network.InterfaceInfo{}, nil)
	c.Assert(cf.IsPendingRemoval(), jc.IsFalse)
	c.Assert(cf.NeedsUpdating(), jc.IsFalse)
	cf.MarkForRemoval()
	c.Assert(cf.IsPendingRemoval(), jc.IsTrue)
	c.Assert(cf.NeedsUpdating(), jc.IsFalse)
}

func (s *configFilesSuite) TestIsManaged(c *gc.C) {
	info := network.InterfaceInfo{
		InterfaceName: "ethX",
	}
	cf := networker.NewConfigFile("ethX", "", info, nil)
	c.Assert(cf.IsManaged(), jc.IsFalse) // always false when no data
	c.Assert(cf.UpdateData([]byte("blah")), jc.IsTrue)
	c.Assert(cf.IsManaged(), jc.IsFalse) // false if header is missing
	c.Assert(cf.UpdateData(cf.RenderManaged()), jc.IsTrue)
	c.Assert(cf.IsManaged(), jc.IsTrue)
}

func (s *configFilesSuite) TestApply(c *gc.C) {
	data := []byte("some\ndata\nhere")
	testFile := filepath.Join(c.MkDir(), "test")
	defer os.Remove(testFile)

	cf := networker.NewConfigFile("ethX", testFile, network.InterfaceInfo{}, data)
	c.Assert(cf.NeedsUpdating(), jc.IsFalse)
	c.Assert(cf.IsPendingRemoval(), jc.IsFalse)
	c.Assert(string(cf.Data()), jc.DeepEquals, string(data))

	newData := []byte("new\ndata")
	c.Assert(cf.UpdateData(newData), jc.IsTrue)
	c.Assert(cf.NeedsUpdating(), jc.IsTrue)
	c.Assert(cf.IsPendingRemoval(), jc.IsFalse)

	err := cf.Apply()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cf.NeedsUpdating(), jc.IsFalse)
	c.Assert(cf.IsPendingRemoval(), jc.IsFalse)

	readData, err := ioutil.ReadFile(testFile)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(readData), jc.DeepEquals, string(newData))

	cf.MarkForRemoval()
	c.Assert(cf.NeedsUpdating(), jc.IsFalse)
	c.Assert(cf.IsPendingRemoval(), jc.IsTrue)
	err = cf.Apply()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cf.NeedsUpdating(), jc.IsFalse)
	c.Assert(cf.IsPendingRemoval(), jc.IsFalse)

	_, err = os.Stat(testFile)
	c.Assert(err, jc.Satisfies, os.IsNotExist)
}

func (s *configFilesSuite) TestRenderMainConfig(c *gc.C) {
	expect := `
# Managed by Juju, please don't change.

source /some/path/*.cfg

`[1:]
	data := networker.RenderMainConfig("/some/path")
	c.Assert(string(data), jc.DeepEquals, expect)
}
