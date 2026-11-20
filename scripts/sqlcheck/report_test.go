// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/juju/tc"

	coretesting "github.com/juju/juju/internal/testing"
)

type reportSuite struct {
	coretesting.BaseSuite
}

func TestReportSuite(t *testing.T) {
	tc.Run(t, &reportSuite{})
}

func (s *reportSuite) TestMapsViolationToGoPosition(c *tc.C) {
	src := `
package p

func f() {
	st.Prepare(` + "`" + `
select id from t
` + "`" + `)
}
`
	pkg, stmts, rend := parseAndRender(c, "foo.go", src)
	line, col := runePos(rend.Synthetic, "select")
	diags := mapDiagnostics(pkg.fset, stmts, map[string]*FileRender{"foo.go.sql": rend}, []fluffFile{{
		Filepath: "foo.go.sql",
		Violations: []fluffViolation{{
			LineNo:      line,
			LinePos:     col,
			Code:        "CP01",
			Description: "Keywords must be consistently upper case.",
		}},
	}})
	c.Assert(diags, tc.HasLen, 1)
	c.Check(diags[0].File, tc.Equals, "foo.go")
	c.Check(diags[0].Line, tc.Equals, 5)
	c.Check(diags[0].Rule, tc.Equals, "CP01")
	c.Check(diags[0].Column > 0, tc.IsTrue)
}

func (s *reportSuite) TestDropsVirtualDiagnostics(c *tc.C) {
	src := `
package p

func f() {
	st.Prepare("SELECT id FROM t WHERE x = 1")
	st.Prepare("SELECT name FROM u WHERE y = 2")
}
`
	pkg, stmts, rend := parseAndRender(c, "foo.go", src)
	var semiLine, semiCol int
	found := false
	off := 0
	line, col := 1, 1
	for off < len(rend.Synthetic) {
		seg, ok := rend.lookup(off)
		if ok && seg.Kind == KindVirtual && rend.Synthetic[off] == ';' {
			semiLine, semiCol = line, col
			found = true
			break
		}
		if rend.Synthetic[off] == '\n' {
			line++
			col = 1
		} else {
			_, size := utf8.DecodeRune(rend.Synthetic[off:])
			if size < 1 {
				size = 1
			}
			off += size - 1
			col++
		}
		off++
	}
	c.Assert(found, tc.IsTrue)
	diags := mapDiagnostics(pkg.fset, stmts, map[string]*FileRender{"foo.go.sql": rend}, []fluffFile{{
		Filepath: "foo.go.sql",
		Violations: []fluffViolation{{
			LineNo:      semiLine,
			LinePos:     semiCol,
			Code:        "LT01",
			Description: "layout",
		}},
	}})
	c.Check(diags, tc.HasLen, 0)
}

func (s *reportSuite) TestIgnoreSuppressesRule(c *tc.C) {
	src := `
package p

func f() {
	// sqlcheck-ignore CP01
	st.Prepare("select id from t where x = 1")
}
`
	pkg, stmts, rend := parseAndRender(c, "foo.go", src)
	line, col := runePos(rend.Synthetic, "select")
	diags := mapDiagnostics(pkg.fset, stmts, map[string]*FileRender{"foo.go.sql": rend}, []fluffFile{{
		Filepath: "foo.go.sql",
		Violations: []fluffViolation{{
			LineNo:      line,
			LinePos:     col,
			Code:        "CP01",
			Description: "Keywords must be consistently upper case.",
		}},
	}})
	c.Check(diags, tc.HasLen, 0)
}

func (s *reportSuite) TestGitHubAnnotationEscaping(c *tc.C) {
	d := Diagnostic{
		File:    "foo.go",
		Line:    4,
		Column:  2,
		Rule:    "CP01",
		Message: "bad\n%value",
	}
	got := formatDiagnostic(d, true)
	c.Check(strings.Contains(got, "::error file=foo.go,line=4,col=2::"), tc.IsTrue)
	// The runner hides the command properties from the job log, so the
	// location is repeated in the message itself.
	c.Check(strings.HasPrefix(got, "::error file=foo.go,line=4,col=2::foo.go:4:2: CP01 "), tc.IsTrue)
	c.Check(strings.Contains(got, "%0A"), tc.IsTrue)
	c.Check(strings.Contains(got, "%25"), tc.IsTrue)
}

func (s *reportSuite) TestGitHubSummaryAggregatesPerFile(c *tc.C) {
	diags := []Diagnostic{
		{File: "a.go", Line: 1, Column: 1, Rule: "LT01", Message: "m1"},
		{File: "a.go", Line: 2, Column: 1, Rule: "LT01", Message: "m2"},
		{File: "a.go", Line: 3, Column: 1, Rule: "CP01", Message: "m3"},
		{File: "b.go", Line: 9, Column: 1, Rule: "LT02", Message: "m4"},
	}
	var b strings.Builder
	err := writeGitHubSummary(&b, 10, 2, diags, nil, nil)
	c.Assert(err, tc.ErrorIsNil)
	got := b.String()
	c.Check(strings.Contains(got, "Extracted 10 statement(s) from 2 file(s): 4 sqlfluff violation(s), 0 SQL site(s) not linted."), tc.IsTrue)
	// The file with more violations sorts first; rules are ordered by
	// count then name.
	c.Check(strings.Contains(got, "| `a.go` | 3 | LT01 (2), CP01 (1) |\n"), tc.IsTrue)
	c.Check(strings.Contains(got, "| `b.go` | 1 | LT02 (1) |\n"), tc.IsTrue)
	c.Check(strings.Index(got, "`a.go`") < strings.Index(got, "`b.go`"), tc.IsTrue)
}

func (s *reportSuite) TestGitHubSummaryWithoutViolations(c *tc.C) {
	var b strings.Builder
	err := writeGitHubSummary(&b, 5, 2, nil, nil, nil)
	c.Assert(err, tc.ErrorIsNil)
	got := b.String()
	c.Check(strings.Contains(got, "No sqlfluff violations found."), tc.IsTrue)
	c.Check(strings.Contains(got, "Extracted 5 statement(s) from 2 file(s): 0 sqlfluff violation(s), 0 SQL site(s) not linted."), tc.IsTrue)
}

func (s *reportSuite) TestGitHubSummaryListsSkips(c *tc.C) {
	var b strings.Builder
	err := writeGitHubSummary(&b, 1, 1, nil, []Skip{{
		File:   "foo.go",
		Line:   48,
		Column: 35,
		Reason: "could not reconstruct SQL: string is appended to at runtime",
		Expr:   `q + "-1"`,
	}}, nil)
	c.Assert(err, tc.ErrorIsNil)
	got := b.String()
	c.Check(strings.Contains(got, "| `foo.go:48:35` | could not reconstruct SQL: string is appended to at runtime `q + \"-1\"` |"), tc.IsTrue)
}

func (s *reportSuite) TestGitHubSummaryCapsRuleList(c *tc.C) {
	var diags []Diagnostic
	for _, rule := range []string{"R1", "R2", "R3", "R4", "R5", "R6"} {
		diags = append(diags, Diagnostic{File: "a.go", Rule: rule})
	}
	var b strings.Builder
	err := writeGitHubSummary(&b, 6, 1, diags, nil, nil)
	c.Assert(err, tc.ErrorIsNil)
	got := b.String()
	c.Check(strings.Contains(got, "| `a.go` | 6 | R1 (1), R2 (1), R3 (1), R4 (1), R5 (1), … +1 |\n"), tc.IsTrue)
}

func (s *reportSuite) TestGitHubSummaryCapsFileRows(c *tc.C) {
	diags := make([]Diagnostic, 0, maxSummaryRows+1)
	for i := 0; i <= maxSummaryRows; i++ {
		diags = append(diags, Diagnostic{File: fmt.Sprintf("f%04d.go", i), Rule: "LT01"})
	}
	var b strings.Builder
	err := writeGitHubSummary(&b, len(diags), len(diags), diags, nil, nil)
	c.Assert(err, tc.ErrorIsNil)
	got := b.String()
	c.Check(strings.Contains(got, "… and 1 more file(s) with violations."), tc.IsTrue)
	c.Check(strings.Contains(got, "| `f0000.go` | 1 | LT01 (1) |\n"), tc.IsTrue)
	c.Check(strings.Contains(got, "| `f1000.go` |"), tc.IsFalse)
}

func (s *reportSuite) TestAppendGitHubSummary(c *tc.C) {
	path := filepath.Join(c.MkDir(), "summary.md")
	c.Assert(os.WriteFile(path, []byte("before\n"), 0o600), tc.ErrorIsNil)
	s.PatchEnvironment("GITHUB_STEP_SUMMARY", path)

	err := appendGitHubSummary(2, 1, []Diagnostic{{
		File: "a.go", Line: 3, Column: 1, Rule: "LT01", Message: "m",
	}}, nil, nil)
	c.Assert(err, tc.ErrorIsNil)

	data, err := os.ReadFile(path)
	c.Assert(err, tc.ErrorIsNil)
	got := string(data)
	c.Check(strings.HasPrefix(got, "before\n"), tc.IsTrue)
	c.Check(strings.Contains(got, "| `a.go` | 1 | LT01 (1) |\n"), tc.IsTrue)
}

func (s *reportSuite) TestAppendGitHubSummaryNoEnvironment(c *tc.C) {
	s.PatchEnvironment("GITHUB_STEP_SUMMARY", "")
	c.Check(appendGitHubSummary(0, 0, nil, nil, nil), tc.ErrorIsNil)
}

func (s *reportSuite) TestParseErrorIsMapped(c *tc.C) {
	src := `
package p

func f() {
	st.Prepare("SELECT FROM WHERE x = 1")
}
`
	pkg, stmts, rend := parseAndRender(c, "foo.go", src)
	line, col := runePos(rend.Synthetic, "SELECT")
	diags := mapDiagnostics(pkg.fset, stmts, map[string]*FileRender{"foo.go.sql": rend}, []fluffFile{{
		Filepath: "foo.go.sql",
		Violations: []fluffViolation{{
			LineNo:      line,
			LinePos:     col,
			Code:        "PRS",
			Description: "Line 1, Position 1: Found unparsable section",
		}},
	}})
	c.Assert(diags, tc.HasLen, 1)
	c.Check(diags[0].Rule, tc.Equals, "PRS")
	c.Check(diags[0].File, tc.Equals, "foo.go")
}

func (s *reportSuite) TestSummarySeparatesNotLintedFromViolations(c *tc.C) {
	got := writeSummary(3, 1, []Skip{{
		File:   "foo.go",
		Line:   48,
		Column: 35,
		Reason: "could not reconstruct SQL: string is appended to at runtime",
		Expr:   "q",
	}}, nil, 12)
	c.Check(strings.Contains(got, "sqlcheck: 12 sqlfluff violation(s)"), tc.IsTrue)
	c.Check(strings.Contains(got, "sqlcheck: 1 SQL site(s) not linted"), tc.IsTrue)
	c.Check(strings.Contains(got, "not linted: foo.go:48:35:"), tc.IsTrue)
	c.Check(strings.Contains(got, "coverage:"), tc.IsFalse)
}

func runePos(src []byte, token string) (int, int) {
	idx := strings.Index(string(src), token)
	if idx < 0 {
		return 1, 1
	}
	line, col := 1, 1
	i := 0
	for i < idx {
		if src[i] == '\n' {
			line++
			col = 1
			i++
			continue
		}
		_, size := utf8.DecodeRune(src[i:])
		if size < 1 {
			size = 1
		}
		i += size
		col++
	}
	return line, col
}
