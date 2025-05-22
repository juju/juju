// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource_test

import (
	"strings"
	stdtesting "testing"

	"github.com/juju/tc"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/resource"
	charmresource "github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/testhelpers"
)

type SerializationSuite struct {
	testhelpers.IsolationSuite
}

func TestSerializationSuite(t *stdtesting.T) {
	tc.Run(t, &SerializationSuite{})
}

func (s *SerializationSuite) TestDeserializeFingerprintOkay(c *tc.C) {
	content := "some data\n..."
	expected, err := charmresource.GenerateFingerprint(strings.NewReader(content))
	c.Assert(err, tc.ErrorIsNil)

	fp, err := resource.DeserializeFingerprint(expected.Bytes())
	c.Assert(err, tc.ErrorIsNil)

	c.Check(fp, tc.DeepEquals, expected)
}

func (s *SerializationSuite) TestDeserializeFingerprintInvalid(c *tc.C) {
	_, err := resource.DeserializeFingerprint([]byte("<too short>"))

	c.Check(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *SerializationSuite) TestDeserializeFingerprintZeroValue(c *tc.C) {
	fp, err := resource.DeserializeFingerprint(nil)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(fp, tc.DeepEquals, charmresource.Fingerprint{})
}
