// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package database

import (
	"encoding/binary"
	"time"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type typesSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&typesSuite{})

func (s *typesSuite) TestNullDuration(c *gc.C) {
	nd := NullDuration{Duration: 10 * time.Second, Valid: true}
	v, err := nd.Value()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(v, gc.Equals, int64(10*time.Second))

	nd = NullDuration{Valid: true}
	v, err = nd.Value()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(v, gc.Equals, int64(0))

	nd = NullDuration{Valid: false}
	v, err = nd.Value()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(v, gc.IsNil)

	err = nd.Scan("10s")
	c.Assert(err, gc.ErrorMatches, `cannot scan type string into NullDuration`)
	c.Assert(nd.Valid, jc.IsFalse)

	err = nd.Scan(int64(20 * time.Second))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(nd.Duration, gc.Equals, 20*time.Second)
	c.Assert(nd.Valid, jc.IsTrue)
}

func (s *typesSuite) TestUint64(c *gc.C) {
	uiv := Uint64{UnsignedValue: uint64(1844674407370955161), Valid: true}
	v, err := uiv.Value()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(v, jc.DeepEquals, []byte{0x99, 0x99, 0x99, 0x99, 0x99, 0x99, 0x99, 0x19})

	uiv = Uint64{Valid: true}
	v, err = uiv.Value()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(v, jc.DeepEquals, []byte{0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0})

	uiv = Uint64{Valid: false}
	v, err = uiv.Value()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(v, gc.IsNil)

	err = uiv.Scan("value")
	c.Assert(err, gc.ErrorMatches, `cannot scan type string into uint64`)
	c.Assert(uiv.Valid, jc.IsFalse)

	b := make([]byte, 8)
	binary.NativeEndian.PutUint64(b, uint64(1844674407370955161))
	err = uiv.Scan(b)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(uiv.UnsignedValue, gc.Equals, uint64(1844674407370955161))
	c.Assert(uiv.Valid, jc.IsTrue)
}
