// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package quota_test

import (
	"testing"

	"github.com/juju/tc"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/quota"
)

func TestKeyValueSuite(t *testing.T) {
	tc.Run(t, &KeyValueSuite{})
}

type KeyValueSuite struct {
}

func (s *KeyValueSuite) TestTupleSize(c *tc.C) {
	err := quota.CheckTupleSize("key", "value", 3, 5)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *KeyValueSuite) TestTupleSizeWithJSONValue(c *tc.C) {
	err := quota.CheckTupleSize("key", map[string]string{
		"hello": "world",
	}, 3, 17)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *KeyValueSuite) TestTupleSizeQuotaBypass(c *tc.C) {
	err := quota.CheckTupleSize("a long key", "a long value", 0, 0)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *KeyValueSuite) TestTupleSizeExceedMaxKeySize(c *tc.C) {
	err := quota.CheckTupleSize("a long key", "bar", 5, 3)
	c.Assert(err, tc.ErrorIs, coreerrors.QuotaLimitExceeded)
	c.Assert(err, tc.ErrorMatches, "max allowed key length.*", tc.Commentf("expected error about exceeding max key length"))
}

func (s *KeyValueSuite) TestTupleSizeExceedMaxValueSize(c *tc.C) {
	err := quota.CheckTupleSize("key", "a long value", 5, 3)
	c.Assert(err, tc.ErrorIs, coreerrors.QuotaLimitExceeded)
	c.Assert(err, tc.ErrorMatches, "max allowed value length.*", tc.Commentf("expected error about exceeding max value length"))
}

func (s *KeyValueSuite) TestTupleSizeMarshalError(c *tc.C) {
	err := quota.CheckTupleSize("key", make(chan struct{}), 3, 42)
	c.Assert(err, tc.ErrorMatches, "marshaling value to JSON:.*")
}

func (s *KeyValueSuite) TestKeyValueTotalSize(c *tc.C) {
	err := quota.CheckKeyValueTotalSize([]quota.KeyValue{
		testKeyValue{key: "key", value: "value"},
		testKeyValue{key: "a", value: "b"},
	}, 10)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *KeyValueSuite) TestKeyValueTotalSizeExactLimit(c *tc.C) {
	err := quota.CheckKeyValueTotalSize([]quota.KeyValue{
		testKeyValue{key: "key", value: "value"},
	}, 8)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *KeyValueSuite) TestKeyValueTotalSizeBypass(c *tc.C) {
	err := quota.CheckKeyValueTotalSize([]quota.KeyValue{
		testKeyValue{key: "key", value: "value"},
	}, 0)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *KeyValueSuite) TestKeyValueTotalSizeExceedsLimit(c *tc.C) {
	err := quota.CheckKeyValueTotalSize([]quota.KeyValue{
		testKeyValue{key: "key", value: "value"},
	}, 7)
	c.Assert(err, tc.ErrorIs, coreerrors.QuotaLimitExceeded)
	c.Assert(err, tc.ErrorMatches, "max allowed total size.*")
}

func (s *KeyValueSuite) TestKeyValueTotalSizeUsesRawBytes(c *tc.C) {
	err := quota.CheckKeyValueTotalSize([]quota.KeyValue{
		testKeyValue{key: "é", value: "€"},
	}, 4)
	c.Assert(err, tc.ErrorIs, coreerrors.QuotaLimitExceeded)
}

type testKeyValue struct {
	key   string
	value string
}

func (kv testKeyValue) Key() string {
	return kv.key
}

func (kv testKeyValue) Value() string {
	return kv.value
}
