// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/core/devices"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/testhelpers"
)

var _ = tc.Suite(&FlagSuite{})

type FlagSuite struct {
	testhelpers.IsolationSuite
}

func (s *FlagSuite) TestStringMapNilOk(c *tc.C) {
	// note that the map may start out nil
	var values map[string]string
	c.Assert(values, tc.IsNil)
	sm := stringMap{&values}
	err := sm.Set("foo=foovalue")
	c.Assert(err, tc.ErrorIsNil)
	err = sm.Set("bar=barvalue")
	c.Assert(err, tc.ErrorIsNil)

	// now the map is non-nil and filled
	c.Assert(values, tc.DeepEquals, map[string]string{
		"foo": "foovalue",
		"bar": "barvalue",
	})
}

func (s *FlagSuite) TestStringMapBadVal(c *tc.C) {
	sm := stringMap{&map[string]string{}}
	err := sm.Set("foo")
	c.Assert(err, tc.ErrorIs, errors.NotValid)
	c.Assert(err, tc.ErrorMatches, "badly formatted name value pair: foo")
}

func (s *FlagSuite) TestStringMapDupVal(c *tc.C) {
	sm := stringMap{&map[string]string{}}
	err := sm.Set("bar=somevalue")
	c.Assert(err, tc.ErrorIsNil)
	err = sm.Set("bar=someothervalue")
	c.Assert(err, tc.ErrorMatches, ".*duplicate.*bar.*")
}

func (s *FlagSuite) TestStorageFlag(c *tc.C) {
	var stores map[string]storage.Directive
	flag := storageFlag{&stores, nil}
	err := flag.Set("foo=bar")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(stores, tc.DeepEquals, map[string]storage.Directive{
		"foo": {Pool: "bar", Count: 1},
	})
}

func (s *FlagSuite) TestStorageFlagErrors(c *tc.C) {
	flag := storageFlag{new(map[string]storage.Directive), nil}
	err := flag.Set("foo")
	c.Assert(err, tc.ErrorMatches, `expected <store>=<directive>`)
	err = flag.Set("foo:bar=baz")
	c.Assert(err, tc.ErrorMatches, `expected <store>=<directive>`)
	err = flag.Set("foo=")
	c.Assert(err, tc.ErrorMatches, `cannot parse disk storage directive: storage directives require at least one field to be specified`)
}

func (s *FlagSuite) TestStorageFlagBundleStorage(c *tc.C) {
	var stores map[string]storage.Directive
	var bundleStores map[string]map[string]storage.Directive
	flag := storageFlag{&stores, &bundleStores}
	err := flag.Set("foo=bar")
	c.Assert(err, tc.ErrorIsNil)
	err = flag.Set("app:baz=qux")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(stores, tc.DeepEquals, map[string]storage.Directive{
		"foo": {Pool: "bar", Count: 1},
	})
	c.Assert(bundleStores, tc.DeepEquals, map[string]map[string]storage.Directive{
		"app": {
			"baz": {Pool: "qux", Count: 1},
		},
	})
}

func (s *FlagSuite) TestStorageFlagBundleStorageErrors(c *tc.C) {
	flag := storageFlag{new(map[string]storage.Directive), new(map[string]map[string]storage.Directive)}
	err := flag.Set("foo")
	c.Assert(err, tc.ErrorMatches, `expected \[<application>\:]<store>=<directive>`)
}

func (s *FlagSuite) TestAttachStorageFlag(c *tc.C) {
	var stores []string
	flag := attachStorageFlag{&stores}
	err := flag.Set("foo/0,bar/1")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(stores, tc.DeepEquals, []string{"foo/0", "bar/1"})
}

func (s *FlagSuite) TestAttachStorageFlagErrors(c *tc.C) {
	flag := attachStorageFlag{new([]string)}
	err := flag.Set("zing")
	c.Assert(err, tc.ErrorMatches, `storage ID "zing" not valid`)
}

func (s *FlagSuite) TestDevicesFlag(c *tc.C) {
	var devs map[string]devices.Constraints
	flag := devicesFlag{&devs, nil}
	err := flag.Set("foo=3,nvidia.com/gpu,gpu=nvidia-tesla-p100")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(devs, tc.DeepEquals, map[string]devices.Constraints{
		"foo": {
			Type:  "nvidia.com/gpu",
			Count: 3,
			Attributes: map[string]string{
				"gpu": "nvidia-tesla-p100",
			},
		},
	})
}

func testFlagErrors(c *tc.C, flag devicesFlag, flagStr string, expectedErr string) {
	err := flag.Set(flagStr)
	c.Assert(err, tc.ErrorMatches, expectedErr)
}

func (s *FlagSuite) TestDevicesFlagErrors(c *tc.C) {
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

func (s *FlagSuite) TestDevicesFlagBundleDevices(c *tc.C) {
	var devs map[string]devices.Constraints
	var bundleDevices map[string]map[string]devices.Constraints
	flag := devicesFlag{&devs, &bundleDevices}
	err := flag.Set("foo=bar")
	c.Assert(err, tc.ErrorIsNil)
	err = flag.Set("app:baz=qux")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(devs, tc.DeepEquals, map[string]devices.Constraints{
		"foo": {Type: "bar", Count: 1},
	})
	c.Assert(bundleDevices, tc.DeepEquals, map[string]map[string]devices.Constraints{
		"app": {
			"baz": {Type: "qux", Count: 1},
		},
	})
}

func (s *FlagSuite) TestDevicesFlagBundleDevicesErrors(c *tc.C) {
	flag := devicesFlag{new(map[string]devices.Constraints), new(map[string]map[string]devices.Constraints)}
	err := flag.Set("foo")
	c.Assert(err, tc.ErrorMatches, `expected \[<application>\:]<device>=<constraints>`)
}
