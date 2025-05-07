// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource_test

import (
	"strings"

	"github.com/juju/tc"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/resource"
	charmresource "github.com/juju/juju/internal/charm/resource"
)

type SerializationSuite struct {
	testing.IsolationSuite
}

var _ = tc.Suite(&SerializationSuite{})

func (s *SerializationSuite) TestDeserializeFingerprintOkay(c *tc.C) {
	content := "some data\n..."
	expected, err := charmresource.GenerateFingerprint(strings.NewReader(content))
	c.Assert(err, jc.ErrorIsNil)

	fp, err := resource.DeserializeFingerprint(expected.Bytes())
	c.Assert(err, jc.ErrorIsNil)

	c.Check(fp, jc.DeepEquals, expected)
}

func (s *SerializationSuite) TestDeserializeFingerprintInvalid(c *tc.C) {
	_, err := resource.DeserializeFingerprint([]byte("<too short>"))

	c.Check(err, jc.ErrorIs, coreerrors.NotValid)
}

func (s *SerializationSuite) TestDeserializeFingerprintZeroValue(c *tc.C) {
	fp, err := resource.DeserializeFingerprint(nil)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(fp, jc.DeepEquals, charmresource.Fingerprint{})
}
