// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"errors"
	"fmt"
	"os/exec"
	"testing"

	"github.com/juju/tc"

	coretesting "github.com/juju/juju/internal/testing"
)

type lintSuite struct {
	coretesting.BaseSuite
}

func TestLintSuite(t *testing.T) {
	tc.Run(t, &lintSuite{})
}

// exitError runs sh with the given exit status and returns the
// resulting error.
func exitError(c *tc.C, code int) error {
	c.Helper()
	err := exec.Command("sh", "-c", fmt.Sprintf("exit %d", code)).Run()
	c.Assert(err, tc.NotNil)
	return err
}

func (s *lintSuite) TestIsFluffLintExit(c *tc.C) {
	c.Check(isFluffLintExit(nil), tc.IsFalse)
	c.Check(isFluffLintExit(errors.New("boom")), tc.IsFalse)

	// sqlfluff lint signals violations with exit code 1.
	c.Check(isFluffLintExit(exitError(c, 1)), tc.IsTrue)

	// Other exit codes are genuine failures.
	c.Check(isFluffLintExit(exitError(c, 2)), tc.IsFalse)
	c.Check(isFluffLintExit(exitError(c, 127)), tc.IsFalse)

	// Wrapped exit errors are still recognised.
	c.Check(isFluffLintExit(fmt.Errorf("lint: %w", exitError(c, 1))), tc.IsTrue)
	c.Check(isFluffLintExit(fmt.Errorf("lint: %w", exitError(c, 2))), tc.IsFalse)
}

func (s *lintSuite) TestLookSQLFluffNotFound(c *tc.C) {
	// Point PATH at an empty directory so sqlfluff cannot resolve.
	empty := c.MkDir()
	s.PatchEnvironment("PATH", empty)

	_, err := lookSQLFluff()
	c.Check(err, tc.ErrorMatches, "sqlfluff not found in PATH; install the repository-pinned version: .*")
	c.Check(errors.Is(err, exec.ErrNotFound), tc.IsTrue)
}

func (s *lintSuite) TestTruncate(c *tc.C) {
	c.Check(truncate("short", 10), tc.Equals, "short")
	c.Check(truncate("exactlyten", 10), tc.Equals, "exactlyten")
	c.Check(truncate("a bit too long", 6), tc.Equals, "a bit ...")
}

func (s *lintSuite) TestMatchSynthExact(c *tc.C) {
	rend := &FileRender{GoPath: "a/state.go"}
	bySynth := map[string]*FileRender{
		"/tmp/x/a/state.go.sql": rend,
	}

	c.Check(matchSynth(bySynth, "/tmp/x/a/state.go.sql"), tc.Equals, rend)
}

func (s *lintSuite) TestMatchSynthCleaned(c *tc.C) {
	rend := &FileRender{GoPath: "a/state.go"}
	bySynth := map[string]*FileRender{
		"/tmp/x/a/state.go.sql": rend,
	}

	// Redundant separators and dot elements resolve via Clean.
	c.Check(matchSynth(bySynth, "/tmp/x/a/./state.go.sql"), tc.Equals, rend)
	c.Check(matchSynth(bySynth, "/tmp/x//a/state.go.sql"), tc.Equals, rend)
}

func (s *lintSuite) TestMatchSynthSuffix(c *tc.C) {
	rend := &FileRender{GoPath: "a/state.go"}
	other := &FileRender{GoPath: "b/other.go"}
	bySynth := map[string]*FileRender{
		"/tmp/extract/a/state.go.sql": rend,
		"/tmp/extract/b/other.go.sql": other,
	}

	// sqlfluff reported a relative path; the absolute synthetic path
	// ends with it.
	c.Check(matchSynth(bySynth, "a/state.go.sql"), tc.Equals, rend)

	// The reverse direction: the reported path is longer than the
	// synthetic key.
	short := map[string]*FileRender{
		"extract/a/state.go.sql": rend,
	}
	c.Check(matchSynth(short, "/tmp/extract/a/state.go.sql"), tc.Equals, rend)
}

func (s *lintSuite) TestMatchSynthAliasesAreNotAmbiguous(c *tc.C) {
	rend := &FileRender{GoPath: "a/state.go"}
	bySynth := map[string]*FileRender{
		"a/state.go.sql":                rend,
		"/tmp/extract/a/state.go.sql":   rend,
		"/tmp/extract/b/other.go.sql":   {GoPath: "b/other.go"},
		"extract/a/state.go.sql":        rend,
		"/tmp/extract/c/another.go.sql": {GoPath: "c/another.go"},
	}
	// Several keys alias the same render. They must not make a unique
	// basename or suffix match look like an ambiguous cross-file match.
	c.Check(matchSynth(bySynth, "state.go.sql"), tc.Equals, rend)
}

func (s *lintSuite) TestMatchSynthBasename(c *tc.C) {
	rend := &FileRender{GoPath: "a/state.go"}
	bySynth := map[string]*FileRender{
		"/tmp/x/a/state.go.sql": rend,
	}

	// A unique basename match resolves deterministically regardless of
	// map iteration order.
	for range 20 {
		c.Check(matchSynth(bySynth, "/somewhere/else/state.go.sql"), tc.Equals, rend)
	}
}

func (s *lintSuite) TestMatchSynthAmbiguousBasename(c *tc.C) {
	first := &FileRender{GoPath: "a/state.go"}
	second := &FileRender{GoPath: "b/state.go"}
	bySynth := map[string]*FileRender{
		"/tmp/x/a/state.go.sql": first,
		"/tmp/x/b/state.go.sql": second,
	}

	// Two synthetic files share the basename: no guess is made, even
	// though one of them is a suffix match.
	c.Check(matchSynth(bySynth, "a/state.go.sql"), tc.IsNil)
	c.Check(matchSynth(bySynth, "state.go.sql"), tc.IsNil)

	// Exact and cleaned lookups still resolve unambiguously.
	c.Check(matchSynth(bySynth, "/tmp/x/b/state.go.sql"), tc.Equals, second)
}

func (s *lintSuite) TestMatchSynthNoMatch(c *tc.C) {
	bySynth := map[string]*FileRender{
		"/tmp/x/a/state.go.sql": {GoPath: "a/state.go"},
	}

	c.Check(matchSynth(bySynth, "unrelated.go.sql"), tc.IsNil)
	c.Check(matchSynth(bySynth, "state/other.go.sql"), tc.IsNil)
}
