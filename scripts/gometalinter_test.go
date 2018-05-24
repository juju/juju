// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package scripts_test

import (
	"regexp"
	"testing"

	"github.com/alecthomas/gometalinter/regressiontests"
	gc "gopkg.in/check.v1"

	jc "github.com/juju/testing/checkers"
)

func Test(t *testing.T) {
	gc.TestingT(t)
}

type gometalinterTest struct{}

var _ = gc.Suite(&gometalinterTest{})

// TestIssueFormat validates that the gometalinter outputs the same format that
// we expecting in the gometalinter.bash script.
// This also has the added benefit that we can add the gometalinter to the
// dependencies.tsv lock file, without this we won't be able to revision lock
// the gometalinter to a specific commit ref.
func (*gometalinterTest) TestIssueFormat(c *gc.C) {
	issue := regressiontests.Issue{
		Linter:   "foo",
		Severity: "warning",
		Path:     "/a/b/c/file.go",
		Line:     20,
		Col:      1,
		Message:  "message",
	}

	pattern := regexp.MustCompile(`(.+[^:]\:){1}(([0-9]+)?\:){2}(.+[^:]\:)(.+[^\(])\((.+)?\)`)
	result := pattern.MatchString(issue.String())
	c.Assert(result, jc.IsTrue)
}
