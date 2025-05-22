// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package resource_test

import (
	"crypto/sha512"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/charm/resource"
)

func newFingerprint(c *tc.C, data string) ([]byte, string) {
	hash := sha512.New384()
	_, err := hash.Write([]byte(data))
	c.Assert(err, tc.ErrorIsNil)
	raw := hash.Sum(nil)

	hexStr := hex.EncodeToString(raw)
	return raw, hexStr
}
func TestFingerprintSuite(t *testing.T) {
	tc.Run(t, &FingerprintSuite{})
}

type FingerprintSuite struct{}

func (s *FingerprintSuite) TestNewFingerprintOkay(c *tc.C) {
	expected, _ := newFingerprint(c, "spamspamspam")

	fp, err := resource.NewFingerprint(expected)
	c.Assert(err, tc.ErrorIsNil)
	raw := fp.Bytes()

	c.Check(raw, tc.DeepEquals, expected)
}

func (s *FingerprintSuite) TestNewFingerprintTooSmall(c *tc.C) {
	expected, _ := newFingerprint(c, "spamspamspam")

	_, err := resource.NewFingerprint(expected[:10])

	c.Check(err, tc.ErrorIs, errors.NotValid)
	c.Check(err, tc.ErrorMatches, `.*too small.*`)
}

func (s *FingerprintSuite) TestNewFingerprintTooBig(c *tc.C) {
	expected, _ := newFingerprint(c, "spamspamspam")

	_, err := resource.NewFingerprint(append(expected, 1, 2, 3))

	c.Check(err, tc.ErrorIs, errors.NotValid)
	c.Check(err, tc.ErrorMatches, `.*too big.*`)
}

func (s *FingerprintSuite) TestParseFingerprintOkay(c *tc.C) {
	_, expected := newFingerprint(c, "spamspamspam")

	fp, err := resource.ParseFingerprint(expected)
	c.Assert(err, tc.ErrorIsNil)
	hex := fp.String()

	c.Check(hex, tc.DeepEquals, expected)
}

func (s *FingerprintSuite) TestParseFingerprintNonHex(c *tc.C) {
	_, err := resource.ParseFingerprint("abc") // not hex

	c.Check(err, tc.ErrorMatches, `.*odd length hex string.*`)
}

func (s *FingerprintSuite) TestGenerateFingerprint(c *tc.C) {
	expected, _ := newFingerprint(c, "spamspamspam")
	data := strings.NewReader("spamspamspam")

	fp, err := resource.GenerateFingerprint(data)
	c.Assert(err, tc.ErrorIsNil)
	raw := fp.Bytes()

	c.Check(raw, tc.DeepEquals, expected)
}

func (s *FingerprintSuite) TestString(c *tc.C) {
	raw, expected := newFingerprint(c, "spamspamspam")
	fp, err := resource.NewFingerprint(raw)
	c.Assert(err, tc.ErrorIsNil)

	hex := fp.String()

	c.Check(hex, tc.Equals, expected)
}

func (s *FingerprintSuite) TestRoundtripString(c *tc.C) {
	_, expected := newFingerprint(c, "spamspamspam")

	fp, err := resource.ParseFingerprint(expected)
	c.Assert(err, tc.ErrorIsNil)
	hex := fp.String()

	c.Check(hex, tc.Equals, expected)
}

func (s *FingerprintSuite) TestBytes(c *tc.C) {
	expected, _ := newFingerprint(c, "spamspamspam")
	fp, err := resource.NewFingerprint(expected)
	c.Assert(err, tc.ErrorIsNil)

	raw := fp.Bytes()

	c.Check(raw, tc.DeepEquals, expected)
}

func (s *FingerprintSuite) TestRoundtripBytes(c *tc.C) {
	expected, _ := newFingerprint(c, "spamspamspam")

	fp, err := resource.NewFingerprint(expected)
	c.Assert(err, tc.ErrorIsNil)
	raw := fp.Bytes()

	c.Check(raw, tc.DeepEquals, expected)
}

func (s *FingerprintSuite) TestValidateOkay(c *tc.C) {
	raw, _ := newFingerprint(c, "spamspamspam")
	fp, err := resource.NewFingerprint(raw)
	c.Assert(err, tc.ErrorIsNil)

	err = fp.Validate()

	c.Check(err, tc.ErrorIsNil)
}

func (s *FingerprintSuite) TestValidateZero(c *tc.C) {
	var fp resource.Fingerprint
	err := fp.Validate()

	c.Check(err, tc.ErrorIs, errors.NotValid)
	c.Check(err, tc.ErrorMatches, `zero-value fingerprint not valid`)
}
