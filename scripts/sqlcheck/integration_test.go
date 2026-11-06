// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"context"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/juju/tc"

	coretesting "github.com/juju/juju/internal/testing"
)

type integrationSuite struct {
	coretesting.BaseSuite
}

func TestIntegrationSuite(t *testing.T) {
	tc.Run(t, &integrationSuite{})
}

// sqlfluffBinary returns the sqlfluff binary or skips the calling test.
// Jobs that install sqlfluff set SQLCHECK_REQUIRE_SQLFLUFF so a missing
// installation fails loudly instead of silently skipping integration
// coverage. Other CI systems and local runs skip when it is unavailable.
func sqlfluffBinary(c *tc.C) string {
	c.Helper()
	root, err := findRepoRoot(".")
	if err != nil {
		// Outside a checkout there is no pin file; the version
		// check is skipped by lookSQLFluff.
		root = ""
	}
	bin, err := lookSQLFluff(root)
	if err == nil {
		return bin
	}
	if os.Getenv("SQLCHECK_REQUIRE_SQLFLUFF") == "1" {
		c.Fatalf("sqlfluff is required by SQLCHECK_REQUIRE_SQLFLUFF: %v", err)
	}
	c.Skip("sqlfluff not installed")
	return ""
}

func (s *integrationSuite) TestSQLFluffEndToEnd(c *tc.C) {
	bin := sqlfluffBinary(c)

	// Pinned sqlfluff with .github/.sqlfluff does not emit CP01 for
	// lowercase keywords. LT01 (layout.spacing) is a known hit.
	src := trimFixture(`
package p

func f() {
	st.Prepare(` + "`" + `
SELECT id  FROM t
WHERE x = 1
` + "`" + `)
}
`)
	fset := token.NewFileSet()
	pkg, err := parsePackage(fset, []goFile{{Path: "foo.go", Src: []byte(src)}})
	c.Assert(err, tc.ErrorIsNil)
	next := 1
	stmts, skips := extractPackage(pkg, &next)
	c.Assert(skips, tc.HasLen, 0)
	c.Assert(stmts, tc.HasLen, 1)
	rend := renderPackage(pkg, stmts)["foo.go"]
	c.Assert(rend, tc.NotNil)

	root, err := findRepoRoot(".")
	c.Assert(err, tc.ErrorIsNil)
	cfg := filepath.Join(root, ".github", ".sqlfluff")

	dir := c.MkDir()
	bySynth, err := writeExtracted(dir, map[string]*FileRender{"foo.go": rend})
	c.Assert(err, tc.ErrorIsNil)

	absSynth := map[string]*FileRender{}
	for p, r := range bySynth {
		abs, _ := filepath.Abs(p)
		absSynth[abs] = r
		absSynth[p] = r
	}

	fluff, stderr, err := runSQLFluff(context.Background(), bin, cfg, dir, nil)
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("stderr: %s", stderr))
	diags := mapDiagnostics(pkg.fset, stmts, absSynth, fluff)
	c.Assert(len(diags) > 0, tc.IsTrue)
	found := false
	for _, d := range diags {
		if d.Rule == "LT01" && d.Line == 5 {
			found = true
			c.Check(d.File, tc.Equals, "foo.go")
			c.Check(d.Column > 0, tc.IsTrue)
			c.Check(strings.Contains(strings.ToLower(d.Message), "space"), tc.IsTrue)
		}
	}
	c.Check(found, tc.IsTrue, tc.Commentf("diagnostics: %#v", diags))
}

func (s *integrationSuite) TestSQLFluffFixEndToEnd(c *tc.C) {
	bin := sqlfluffBinary(c)

	// Run the real plan/apply/re-lint cycle to validate the message
	// matchers against the pinned sqlfluff output and to prove that fixes
	// converge without making the Go source unparsable.
	root := c.MkDir()
	path := filepath.Join(root, "foo.go")
	src := "package p\n\nfunc f() {\n\tst.Prepare(`SELECT id  FROM   t WHERE x = 1`)\n}\n"
	c.Assert(os.WriteFile(path, []byte(src), 0o644), tc.ErrorIsNil)
	repo, err := findRepoRoot(".")
	c.Assert(err, tc.ErrorIsNil)
	cfg := filepath.Join(repo, ".github", ".sqlfluff")
	rules := map[string]bool{"LT01": true}
	var finalFluff []fluffFile
	for pass := 0; pass <= maxFixPasses; pass++ {
		data, err := os.ReadFile(path)
		c.Assert(err, tc.ErrorIsNil)
		fset := token.NewFileSet()
		pkg, err := parsePackage(fset, []goFile{{Path: "foo.go", Src: data}})
		c.Assert(err, tc.ErrorIsNil)
		next := 1
		stmts, skips := extractPackage(pkg, &next)
		c.Assert(skips, tc.HasLen, 0)
		c.Assert(stmts, tc.HasLen, 1)
		rend := renderPackage(pkg, stmts)["foo.go"]
		c.Assert(rend, tc.NotNil)
		dir := c.MkDir()
		bySynth, err := writeExtracted(dir, map[string]*FileRender{"foo.go": rend})
		c.Assert(err, tc.ErrorIsNil)
		fluff, stderr, err := runSQLFluff(context.Background(), bin, cfg, dir, rules)
		c.Assert(err, tc.ErrorIsNil, tc.Commentf("stderr: %s", stderr))
		finalFluff = fluff
		edits, _ := planFixes(stmts, bySynth, fluff)
		if len(edits) == 0 {
			break
		}
		changed, err := applyEdits(root, edits)
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(changed, tc.Equals, 1)
	}

	violations := 0
	for _, file := range finalFluff {
		violations += len(file.Violations)
	}
	c.Check(violations, tc.Equals, 0, tc.Commentf("sqlfluff findings: %#v", finalFluff))
	fixed, err := os.ReadFile(path)
	c.Assert(err, tc.ErrorIsNil)
	_, err = parser.ParseFile(token.NewFileSet(), "foo.go", fixed, parser.AllErrors)
	c.Check(err, tc.ErrorIsNil)
	c.Check(strings.Contains(string(fixed), "SELECT id FROM t WHERE x = 1"), tc.IsTrue)
}
