// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package keyvalue_test

import (
	"path"
	stdtesting "testing"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/worker/keyvalue"
)

func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
}

type keyvalueSuite struct {
	path  string
	store *keyvalue.KeyValueStore
}

var _ = gc.Suite(&keyvalueSuite{})

func (t *keyvalueSuite) SetUpTest(c *gc.C) {
	t.path = path.Join(c.MkDir(), "data.yaml")
	t.store = keyvalue.NewKeyValueStore(t.path)
}

func (t *keyvalueSuite) TestReadNonExist(c *gc.C) {
	value, err := t.store.Get("charm-url")
	c.Assert(err, gc.NotNil)
	c.Assert(keyvalue.IsNotSetError(err), gc.Equals, true)
	c.Assert(err, gc.ErrorMatches, "value for key \"charm-url\" not set")
	c.Assert(value, gc.Equals, nil)
}

func (t *keyvalueSuite) TestWriteRead(c *gc.C) {
	err := t.store.Set("charm-url", "cs:wordpress-42")
	c.Assert(err, jc.ErrorIsNil)

	value, err := t.store.Get("charm-url")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(value, gc.Equals, "cs:wordpress-42")
}

func (t *keyvalueSuite) TestWriteResetRead(c *gc.C) {
	err := t.store.Set("charm-url", "cs:wordpress-42")
	c.Assert(err, jc.ErrorIsNil)

	t.store.ResetData()

	value, err := t.store.Get("charm-url")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(value, gc.Equals, "cs:wordpress-42")
}
