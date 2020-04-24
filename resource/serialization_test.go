// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource_test

import (
	"strings"

	charmresource "github.com/juju/charm/v7/resource"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/resource"
)

type SerializationSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&SerializationSuite{})

func (s *SerializationSuite) TestDeserializeFingerprintOkay(c *gc.C) {
	content := "some data\n..."
	expected, err := charmresource.GenerateFingerprint(strings.NewReader(content))
	c.Assert(err, jc.ErrorIsNil)

	fp, err := resource.DeserializeFingerprint(expected.Bytes())
	c.Assert(err, jc.ErrorIsNil)

	c.Check(fp, jc.DeepEquals, expected)
}

func (s *SerializationSuite) TestDeserializeFingerprintInvalid(c *gc.C) {
	_, err := resource.DeserializeFingerprint([]byte("<too short>"))

	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

func (s *SerializationSuite) TestDeserializeFingerprintZeroValue(c *gc.C) {
	fp, err := resource.DeserializeFingerprint(nil)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(fp, jc.DeepEquals, charmresource.Fingerprint{})
}
