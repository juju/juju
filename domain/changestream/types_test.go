// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package changestream

import (
	"testing"
	"time"

	"github.com/juju/tc"
)

type typesSuite struct{}

func TestServiceSuite(t *testing.T) {
	tc.Run(t, &typesSuite{})
}

func (s *typesSuite) TestWindowContains(c *tc.C) {
	now := time.Now()
	testCases := []struct {
		window   Window
		other    Window
		expected bool
	}{{
		window:   Window{Start: now, End: now},
		other:    Window{Start: now, End: now},
		expected: true,
	}, {
		window:   Window{Start: now.Add(-time.Minute), End: now.Add(time.Minute)},
		other:    Window{Start: now, End: now},
		expected: true,
	}, {
		window:   Window{Start: now.Add(time.Minute), End: now.Add(-time.Minute)},
		other:    Window{Start: now, End: now},
		expected: false,
	}, {
		window:   Window{Start: now.Add(time.Minute), End: now.Add(time.Minute)},
		other:    Window{Start: now, End: now},
		expected: false,
	}, {
		window:   Window{Start: now.Add(-time.Minute), End: now.Add(-time.Minute)},
		other:    Window{Start: now, End: now},
		expected: false,
	}, {
		window:   Window{Start: now, End: now.Add(time.Minute * 2)},
		other:    Window{Start: now.Add(time.Minute), End: now.Add(time.Minute + time.Second)},
		expected: true,
	}, {
		window:   Window{Start: now, End: now.Add(time.Minute * 2)},
		other:    Window{Start: now.Add(time.Nanosecond), End: now.Add((time.Minute * 2) - time.Nanosecond)},
		expected: true,
	}, {
		window:   Window{Start: now, End: now.Add(time.Minute * 2)},
		other:    Window{Start: now, End: now.Add((time.Minute * 2) - time.Nanosecond)},
		expected: false,
	}, {
		window:   Window{Start: now, End: now.Add(time.Minute * 2)},
		other:    Window{Start: now.Add(time.Nanosecond), End: now.Add(time.Minute * 2)},
		expected: false,
	}}
	for i, test := range testCases {
		c.Logf("test %d", i)

		got := test.window.Contains(test.other)
		c.Check(got, tc.Equals, test.expected)
	}
}

func (s *typesSuite) TestWindowEquals(c *tc.C) {
	now := time.Now()
	testCases := []struct {
		window   Window
		other    Window
		expected bool
	}{{
		window:   Window{Start: now, End: now},
		other:    Window{Start: now, End: now},
		expected: true,
	}, {
		window:   Window{Start: now.Add(-time.Minute), End: now.Add(time.Minute)},
		other:    Window{Start: now, End: now},
		expected: false,
	}}
	for i, test := range testCases {
		c.Logf("test %d", i)

		got := test.window.Equals(test.other)
		c.Check(got, tc.Equals, test.expected)
	}
}
