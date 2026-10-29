// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
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
	c.Check(strings.Contains(got, "%0A"), tc.IsTrue)
	c.Check(strings.Contains(got, "%25"), tc.IsTrue)
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
