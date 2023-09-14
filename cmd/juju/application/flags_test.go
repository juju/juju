// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/devices"
	"github.com/juju/juju/internal/storage"
)

var _ = gc.Suite(&FlagSuite{})

type FlagSuite struct {
	testing.IsolationSuite
}

func (FlagSuite) TestStringMapNilOk(c *gc.C) {
	// note that the map may start out nil
	var values map[string]string
	c.Assert(values, gc.IsNil)
	sm := stringMap{&values}
	err := sm.Set("foo=foovalue")
	c.Assert(err, jc.ErrorIsNil)
	err = sm.Set("bar=barvalue")
	c.Assert(err, jc.ErrorIsNil)

	// now the map is non-nil and filled
	c.Assert(values, gc.DeepEquals, map[string]string{
		"foo": "foovalue",
		"bar": "barvalue",
	})
}

func (FlagSuite) TestStringMapBadVal(c *gc.C) {
	sm := stringMap{&map[string]string{}}
	err := sm.Set("foo")
	c.Assert(err, jc.ErrorIs, errors.NotValid)
	c.Assert(err, gc.ErrorMatches, "badly formatted name value pair: foo")
}

func (FlagSuite) TestStringMapDupVal(c *gc.C) {
	sm := stringMap{&map[string]string{}}
	err := sm.Set("bar=somevalue")
	c.Assert(err, jc.ErrorIsNil)
	err = sm.Set("bar=someothervalue")
	c.Assert(err, gc.ErrorMatches, ".*duplicate.*bar.*")
}

func (FlagSuite) TestStorageFlag(c *gc.C) {
	var stores map[string]storage.Constraints
	flag := storageFlag{&stores, nil}
	err := flag.Set("foo=bar")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(stores, jc.DeepEquals, map[string]storage.Constraints{
		"foo": {Pool: "bar", Count: 1},
	})
}

func (FlagSuite) TestStorageFlagErrors(c *gc.C) {
	flag := storageFlag{new(map[string]storage.Constraints), nil}
	err := flag.Set("foo")
	c.Assert(err, gc.ErrorMatches, `expected <store>=<constraints>`)
	err = flag.Set("foo:bar=baz")
	c.Assert(err, gc.ErrorMatches, `expected <store>=<constraints>`)
	err = flag.Set("foo=")
	c.Assert(err, gc.ErrorMatches, `cannot parse disk constraints: storage constraints require at least one field to be specified`)
}

func (FlagSuite) TestStorageFlagBundleStorage(c *gc.C) {
	var stores map[string]storage.Constraints
	var bundleStores map[string]map[string]storage.Constraints
	flag := storageFlag{&stores, &bundleStores}
	err := flag.Set("foo=bar")
	c.Assert(err, jc.ErrorIsNil)
	err = flag.Set("app:baz=qux")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(stores, jc.DeepEquals, map[string]storage.Constraints{
		"foo": {Pool: "bar", Count: 1},
	})
	c.Assert(bundleStores, jc.DeepEquals, map[string]map[string]storage.Constraints{
		"app": {
			"baz": {Pool: "qux", Count: 1},
		},
	})
}

func (FlagSuite) TestStorageFlagBundleStorageErrors(c *gc.C) {
	flag := storageFlag{new(map[string]storage.Constraints), new(map[string]map[string]storage.Constraints)}
	err := flag.Set("foo")
	c.Assert(err, gc.ErrorMatches, `expected \[<application>\:]<store>=<constraints>`)
}

func (FlagSuite) TestAttachStorageFlag(c *gc.C) {
	var stores []string
	flag := attachStorageFlag{&stores}
	err := flag.Set("foo/0,bar/1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(stores, jc.DeepEquals, []string{"foo/0", "bar/1"})
}

func (FlagSuite) TestAttachStorageFlagErrors(c *gc.C) {
	flag := attachStorageFlag{new([]string)}
	err := flag.Set("zing")
	c.Assert(err, gc.ErrorMatches, `storage ID "zing" not valid`)
}

func (FlagSuite) TestDevicesFlag(c *gc.C) {
	var devs map[string]devices.Constraints
	flag := devicesFlag{&devs, nil}
	err := flag.Set("foo=3,nvidia.com/gpu,gpu=nvidia-tesla-p100")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(devs, jc.DeepEquals, map[string]devices.Constraints{
		"foo": {
			Type:  "nvidia.com/gpu",
			Count: 3,
			Attributes: map[string]string{
				"gpu": "nvidia-tesla-p100",
			},
		},
	})
}

func testFlagErrors(c *gc.C, flag devicesFlag, flagStr string, expectedErr string) {
	err := flag.Set(flagStr)
	c.Assert(err, gc.ErrorMatches, expectedErr)
}

func (FlagSuite) TestDevicesFlagErrors(c *gc.C) {
	flag := devicesFlag{new(map[string]devices.Constraints), nil}
	testFlagErrors(c, flag, "foo", `expected <device>=<constraints>`)
	testFlagErrors(c, flag, "foo:bar=baz", `expected <device>=<constraints>`)
	testFlagErrors(c, flag, "foo:bar=", `expected <device>=<constraints>`)

	testFlagErrors(c, flag, "foo=2,nvidia.com/gpu,gpu=nvidia-tesla-p100,a=b", `cannot parse device constraints string, supported format is \[<count>,\]<device-class>|<vendor/type>\[,<key>=<value>;...\]`)
	testFlagErrors(c, flag, "foo=2,nvidia.com/gpu,gpu=b=c", `cannot parse device constraints: device attribute key/value pair has bad format: \"gpu=b=c\"`)
	testFlagErrors(c, flag, "foo=badCount,nvidia.com/gpu", `cannot parse device constraints: count must be greater than zero, got \"badCount\"`)
	testFlagErrors(c, flag, "foo=0,nvidia.com/gpu", `cannot parse device constraints: count must be greater than zero, got \"0\"`)
	testFlagErrors(c, flag, "foo=-1,nvidia.com/gpu", `cannot parse device constraints: count must be greater than zero, got \"-1\"`)
}

func (FlagSuite) TestDevicesFlagBundleDevices(c *gc.C) {
	var devs map[string]devices.Constraints
	var bundleDevices map[string]map[string]devices.Constraints
	flag := devicesFlag{&devs, &bundleDevices}
	err := flag.Set("foo=bar")
	c.Assert(err, jc.ErrorIsNil)
	err = flag.Set("app:baz=qux")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(devs, jc.DeepEquals, map[string]devices.Constraints{
		"foo": {Type: "bar", Count: 1},
	})
	c.Assert(bundleDevices, jc.DeepEquals, map[string]map[string]devices.Constraints{
		"app": {
			"baz": {Type: "qux", Count: 1},
		},
	})
}

func (FlagSuite) TestDevicesFlagBundleDevicesErrors(c *gc.C) {
	flag := devicesFlag{new(map[string]devices.Constraints), new(map[string]map[string]devices.Constraints)}
	err := flag.Set("foo")
	c.Assert(err, gc.ErrorMatches, `expected \[<application>\:]<device>=<constraints>`)
}
