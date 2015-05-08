// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"flag"
	"fmt"
	"strings"
	"testing"

	gc "gopkg.in/check.v1"
)

// These are common tags used in juju tests.
const (
	TagBase       = "base"
	TagSmall      = "small"
	TagMedium     = "medium"
	TagLarge      = "large"
	TagSmoke      = "smoke"      // Fast sanity-check.
	TagExternal   = "external"   // Uses external resources.
	TagFunctional = "functional" // Does not use test doubles for low-level.
	TagCloud      = "cloud"      // Interacts with a cloud provider.
	TagVM         = "vm"         // Runs in a local VM (e.g. kvm).
)

var defaultTags = []string{
	TagBase,
}

var jujuTags []string

func init() {
	smoke := flag.Bool("smoke", false, "Run the basic set of fast tests.")
	raw := flag.String("juju-tags", "", "Tagged tests to run.")
	flag.Parse()

	jujuTags = strings.Split(*raw, ",")
	if *smoke {
		jujuTags = append(jujuTags, TagSmoke)
	}
	if len(jujuTags) == 0 {
		jujuTags = defaultTags
	}

	// TOOD(ericsnow) support impllied tags (e.g. VM -> Large)?
}

// CheckTag determines whether or not any of the given tags were passed
// in at the commandline.
func CheckTag(tags ...string) bool {
	return MatchTag(tags...) != ""
}

// MatchTag returns the first provided tag that matches the ones passed
// in at the commandline.
func MatchTag(tags ...string) string {
	for _, tag := range tags {
		for _, jujuTag := range jujuTags {
			if tag == jujuTag {
				return tag
			}
		}
	}
	return ""
}

// RegisterPackageTagged registers the package for testing if any of the
// given tags were passed in at the commandline.
func RegisterPackageTagged(t *testing.T, tags ...string) {
	if CheckTag(tags...) {
		gc.TestingT(t)
	}
}

// SuiteTagged registers the suite with the test runner if any of the
// given tags were passed in at the commandline.
func SuiteTagged(suite interface{}, tags ...string) {
	if CheckTag(tags...) {
		gc.Suite(suite)
	}
}

// RequireTag causes a test or suite to skip if none of the given tags
// were passed in at the commandline.
func RequireTag(c *gc.C, tags ...string) {
	if !CheckTag(tags...) {
		c.Skip(fmt.Sprintf("skipping due to no matching tags (%v)", tags))
	}
}

// SkipTag causes a test or suite to skip if any of the given tags were
// passed in at the commandline.
func SkipTag(c *gc.C, tags ...string) {
	matched := MatchTag(tags...)
	if matched != "" {
		c.Skip(fmt.Sprintf("skipping due to %q tag", matched))
	}
}
