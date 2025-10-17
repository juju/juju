// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package flightrecorder

import (
	"testing"

	"github.com/juju/tc"
)

func TestParseKind(t *testing.T) {
	tc.Run(t, &parseSuite{})
}

type parseSuite struct{}

func (s *parseSuite) TestParse(c *tc.C) {
	tests := []struct {
		input       string
		expected    Kind
		expectError bool
	}{
		{input: "request", expected: KindRequest, expectError: false},
		{input: "error", expected: KindError, expectError: false},
		{input: "all", expected: KindAll, expectError: false},
		{input: "", expected: KindAll, expectError: false},
		{input: "invalid", expected: "", expectError: true},
	}

	for _, test := range tests {
		c.Run(test.input, func(t *testing.T) {
			result, err := ParseKind(test.input)
			if test.expectError {
				if err == nil {
					t.Fatalf("expected error for input %q, got nil", test.input)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error for input %q: %v", test.input, err)
				}
				if result != test.expected {
					t.Fatalf("expected %q for input %q, got %q", test.expected, test.input, result)
				}
			}
		})
	}
}
