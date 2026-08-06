// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap_test

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/environs/bootstrap"
)

type snapVersionParserSuite struct{}

func TestSnapVersionParserSuite(t *testing.T) {
	tc.Run(t, &snapVersionParserSuite{})
}

func (*snapVersionParserSuite) TestParseSnapVersion(c *tc.C) {
	type testCase struct {
		raw     string
		want    string
		wantErr bool
	}
	cases := []testCase{
		// Version Formats table: plain and tagged releases.
		{raw: "4.0.5", want: "4.0.5"},
		{raw: "4.1-beta2", want: "4.1-beta2"},
		// Edge builds carry a trailing -<sha7>.
		{raw: "4.1-beta2-06aa059", want: "4.1-beta2"},
		{raw: "4.0.10-e0c5d0b", want: "4.0.10"},
		// Devel builds carry +<branch>-<sha7>.
		{raw: "4.1-beta2+main-06aa059", want: "4.1-beta2"},
		// Build number is never stripped.
		{raw: "4.1-beta2.3", want: "4.1-beta2.3"},
		// A valid raw semantic version is preserved, not treated as a sha
		// suffix (tag=abcdef, patch=1).
		{raw: "4.1-abcdef1", want: "4.1-abcdef1"},
		// A -rc1 tag parses as a valid raw semver, so it is not stripped.
		{raw: "4.1-rc1", want: "4.1-rc1"},
		// A 6- or 8-character hex tail is not a valid sha suffix.
		{raw: "4.1-beta2-06aa05", wantErr: true},
		{raw: "4.1-beta2-06aa059a", wantErr: true},
		// A non-hex 7-character tail is not a valid sha suffix.
		{raw: "4.1-beta2-06aa05z", wantErr: true},
		// A value that cannot be parsed at all after stripping.
		{raw: "not-a-version", wantErr: true},
	}

	for _, tt := range cases {
		c.Run(tt.raw, func(t *testing.T) {
			got, err := bootstrap.ParseSnapVersion(tt.raw)
			if tt.wantErr {
				tc.Assert(t, err, tc.NotNil)
				tc.Assert(t, got.IsZero(), tc.IsTrue)
				return
			}
			tc.Assert(t, err, tc.ErrorIsNil)
			tc.Assert(t, got.String(), tc.Equals, tt.want)
		})
	}
}
