package version_test

import (
	"."
	"reflect"
	"testing"
)

var cmpTests = []struct {
	v1, v2 string
	compat bool
	less   bool
	eq     bool
}{
	{"1.0.0", "1.0.0", true, false, true},
	{"01.0.0", "1.0.0", true, false, true},
	{"10.0.0", "9.0.0", false, false, false},
	{"1.0.0", "1.0.1", true, true, false},
	{"1.0.1", "1.0.0", true, false, false},
	{"1.0.0", "1.1.0", true, true, false},
	{"1.1.0", "1.0.0", false, false, false},
	{"1.0.0", "2.0.0", false, true, false},
	{"2.0.0", "1.0.0", false, false, false},

	// pre-release
	{"1.0.0", "1.0.0-alpha", true, false, false},
	{"1.0.0", "1.0.1-alpha", false, true, false},
	{"1.0.0-alpha", "1.0.0-beta", true, true, false},
	{"1.0.0-9999", "1.0.0-beta", true, true, false},
	{"1.0.0-99999999999", "1.0.0-199999999999.0.0", true, true, false},
	{"1.0.0-beta.9", "1.0.0-alpha.10", true, false, false},
	{"1.0.0-alpha.9", "1.0.0-alpha.10", true, true, false},
	{"1.0.0-alpha", "1.0.0-alpha-x", true, true, false},
	{"1.0.0-01", "1.0.0-1", true, false, true}, // N.B. equal but strings are not.

	// build
	{"1.0.0", "1.0.0+build", true, true, false},
	{"1.0.0+build", "1.0.0+crime", true, true, false},
	{"1.0.0+build.9", "1.0.0+build.10", true, true, false},
	{"1.0.0+build", "1.0.0+build-x", true, true, false},

	// semantics of pre-release vs build are poorly specified.
	{"1.0.0+build", "1.0.0-alpha+build", true, false, false},
}

func TestComparison(t *testing.T) {
	for i, test := range cmpTests {
		v1, err := version.Parse(test.v1)
		if err != nil {
			t.Fatalf("test %d; %q failed to parse: %v", i, v1, err)
		}
		v2, err := version.Parse(test.v2)
		if err != nil {
			t.Fatalf("test %d; %q failed to parse: %v", i, v2, err)
		}
		compat := v1.Compatible(v2)
		less := v1.Less(v2)
		gteq := v2.Less(v1)
		if compat != test.compat {
			t.Errorf("test %d; %q vs %q compatibility mismatch; expected %v", i, test.v1, test.v2, test.compat)
		}
		if less != test.less {
			t.Errorf("test %d; %q vs %q (%v vs %v) less mismatch; expected %v", i, test.v1, test.v2, v1, v2, test.less)
		}
		if test.eq {
			if gteq {
				t.Errorf("test %d; %q vs %q; a==b but b < a", i, test.v1, test.v2)
			}
		} else {
			if gteq != !test.less {
				t.Errorf("test %d; %q vs %q; inverted less unexpected result; expected %v", i, test.v1, test.v2, !test.less)
			}
		}
	}
}

var parseTests = []struct {
	v      string
	expect *version.Version
}{
	{"11.234.3456", &version.Version{Major: 11, Minor: 234, Patch: 3456}},
	{"0.0.0-09az-x-y.z.foo", &version.Version{Prerelease: []string{"09az-x-y", "z", "foo"}}},
	{"0.0.0+xyz.p-q", &version.Version{Build: []string{"xyz", "p-q"}}},
	{"1.2.3-alpha+build", &version.Version{
		Major:      1,
		Minor:      2,
		Patch:      3,
		Prerelease: []string{"alpha"},
		Build:      []string{"build"},
	}},
	{"1.0.0+build", &version.Version{
		Major: 1,
		Build: []string{"build"},
	}},
}

func TestParse(t *testing.T) {
	for i, test := range parseTests {
		v, err := version.Parse(test.v)
		if err != nil {
			t.Fatalf("test %d; %q parse error %v", i, v, err)
		}
		if !reflect.DeepEqual(v, test.expect) {
			t.Errorf("test %d; %q expected %v got %v", i, test.v, test.expect, v)
		}
	}
}
