// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/storage"
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
	c.Assert(err, jc.Satisfies, errors.IsNotValid)
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
		"app": map[string]storage.Constraints{
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
