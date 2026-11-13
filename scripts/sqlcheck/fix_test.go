// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/juju/tc"

	coretesting "github.com/juju/juju/internal/testing"
)

type fixSuite struct {
	coretesting.BaseSuite
}

func TestFixSuite(t *testing.T) {
	tc.Run(t, &fixSuite{})
}

// fixFixture extracts, renders and plans fixes for the given violations
// against a single-file fixture.
func fixFixture(c *tc.C, src string, violations []fluffViolation) ([]sourceEdit, int) {
	c.Helper()
	src = trimFixture(src)
	fset := token.NewFileSet()
	pkg, err := parsePackage(fset, []goFile{{Path: "foo.go", Src: []byte(src)}})
	c.Assert(err, tc.ErrorIsNil)
	next := 1
	stmts, skips := extractPackage(pkg, &next)
	c.Assert(skips, tc.HasLen, 0)
	c.Assert(stmts, tc.Not(tc.HasLen), 0)
	rend := renderPackage(pkg, stmts)["foo.go"]
	c.Assert(rend, tc.NotNil)
	edits, skipped := planFixes(stmts, map[string]*FileRender{
		"foo.go.sql": rend,
	}, []fluffFile{{
		Filepath:   "foo.go.sql",
		Violations: violations,
	}})
	return edits, skipped
}

// violationAt builds a violation anchored at the first byte of needle in
// the rendered file.
func violationAt(c *tc.C, rend *FileRender, needle, code, desc string) fluffViolation {
	c.Helper()
	idx := strings.Index(string(rend.Synthetic), needle)
	c.Assert(idx >= 0, tc.IsTrue, tc.Commentf("needle %q not found in %q", needle, rend.Synthetic))
	line, col := 1, 1
	for i := range idx {
		if rend.Synthetic[i] == '\n' {
			line++
			col = 1
		} else {
			col++
		}
	}
	return fluffViolation{LineNo: line, LinePos: col, Code: code, Description: desc}
}

func renderFixture(c *tc.C, src string) (*FileRender, []Statement) {
	c.Helper()
	src = trimFixture(src)
	fset := token.NewFileSet()
	pkg, err := parsePackage(fset, []goFile{{Path: "foo.go", Src: []byte(src)}})
	c.Assert(err, tc.ErrorIsNil)
	next := 1
	stmts, skips := extractPackage(pkg, &next)
	c.Assert(skips, tc.HasLen, 0)
	rend := renderPackage(pkg, stmts)["foo.go"]
	c.Assert(rend, tc.NotNil)
	return rend, stmts
}

func applyTo(c *tc.C, src string, edits []sourceEdit) string {
	c.Helper()
	for _, e := range edits {
		c.Assert(e.file, tc.Equals, "foo.go")
		c.Assert(e.start >= 0 && e.end <= len(src) && e.start <= e.end, tc.IsTrue)
		src = src[:e.start] + e.repl + src[e.end:]
	}
	return src
}

func (s *fixSuite) TestCollapseSpace(c *tc.C) {
	src := `package p

func f() {
	st.Prepare("SELECT id FROM   t WHERE x = 1")
}
`
	rend, _ := renderFixture(c, src)
	v := violationAt(c, rend, "   t", "LT01",
		"Expected only single space before naked identifier. Found '   '.")
	edits, skipped := fixFixture(c, src, []fluffViolation{v})
	c.Assert(skipped, tc.Equals, 0)
	c.Assert(edits, tc.HasLen, 1)
	out := applyTo(c, src, edits)
	c.Check(strings.Contains(out, "FROM t WHERE"), tc.IsTrue)
}

func (s *fixSuite) TestRemoveTrailingWhitespace(c *tc.C) {
	src := "package p\n\nfunc f() {\n\tst.Prepare(`SELECT id\nFROM t \nWHERE x = 1`)\n}\n"
	rend, _ := renderFixture(c, src)
	v := violationAt(c, rend, " \nWHERE", "LT01", "Unnecessary trailing whitespace.")
	edits, skipped := fixFixture(c, src, []fluffViolation{v})
	c.Assert(skipped, tc.Equals, 0)
	out := applyTo(c, src, edits)
	c.Check(strings.Contains(out, "FROM t\nWHERE"), tc.IsTrue)
}

func (s *fixSuite) TestInsertSpaceBeforeBracket(c *tc.C) {
	src := `package p

func f() {
	st.Prepare("INSERT INTO t (a) VALUES (1) ON CONFLICT(a) DO NOTHING")
}
`
	rend, _ := renderFixture(c, src)
	v := violationAt(c, rend, "(a) DO NOTHING", "LT01",
		"Expected single whitespace between 'CONFLICT' keyword and start bracket '('.")
	edits, skipped := fixFixture(c, src, []fluffViolation{v})
	c.Assert(skipped, tc.Equals, 0)
	out := applyTo(c, src, edits)
	c.Check(strings.Contains(out, "ON CONFLICT (a)"), tc.IsTrue)
}

func (s *fixSuite) TestInsertSpaceAroundOperator(c *tc.C) {
	src := `package p

func f() {
	st.Prepare("UPDATE t SET name='x' WHERE id = 1")
}
`
	rend, _ := renderFixture(c, src)
	v := violationAt(c, rend, "='x'", "LT01",
		"Expected single whitespace between naked identifier and raw comparison operator '='.")
	edits, skipped := fixFixture(c, src, []fluffViolation{v})
	c.Assert(skipped, tc.Equals, 0)
	out := applyTo(c, src, edits)
	c.Check(strings.Contains(out, "name ="), tc.IsTrue)
}

func (s *fixSuite) TestIndentLine(c *tc.C) {
	src := "package p\n\nfunc f() {\n\tst.Prepare(`SELECT a FROM t\nAND b = 1`)\n}\n"
	rend, _ := renderFixture(c, src)
	v := violationAt(c, rend, "AND b", "LT02", "Expected indent of 4 spaces.")
	edits, skipped := fixFixture(c, src, []fluffViolation{v})
	c.Assert(skipped, tc.Equals, 0)
	out := applyTo(c, src, edits)
	c.Check(strings.Contains(out, "FROM t\n    AND b = 1"), tc.IsTrue)
}

func (s *fixSuite) TestDedentLine(c *tc.C) {
	src := "package p\n\nfunc f() {\n\tst.Prepare(`SELECT a\n    FROM t\nWHERE b = 1`)\n}\n"
	rend, _ := renderFixture(c, src)
	v := violationAt(c, rend, "    FROM t", "LT02", "Line should not be indented.")
	edits, skipped := fixFixture(c, src, []fluffViolation{v})
	c.Assert(skipped, tc.Equals, 0)
	out := applyTo(c, src, edits)
	c.Check(strings.Contains(out, "SELECT a\nFROM t\nWHERE"), tc.IsTrue)
}

func (s *fixSuite) TestLineBreakInRawString(c *tc.C) {
	// A newline insertion into a raw string literal stays a real
	// newline in the Go source.
	src := "package p\n\nfunc f() {\n\tst.Prepare(`SELECT a, b FROM t WHERE x = 1`)\n}\n"
	rend, _ := renderFixture(c, src)
	v := violationAt(c, rend, "a, b", "LT02",
		"Expected line break and indent of 4 spaces before 'a'.")
	edits, _ := fixFixture(c, src, []fluffViolation{v})
	out := applyTo(c, src, edits)
	c.Check(strings.Contains(out, "SELECT\n    a, b FROM"), tc.IsTrue)
}

func (s *fixSuite) TestLineBreakRawStringAfterQuotedIdentifier(c *tc.C) {
	// The raw-string bit comes from literal decoding, not from scanning
	// nearby source bytes. An SQL quote before the anchor must therefore
	// not cause a literal backslash-n to be inserted into the raw string.
	src := "package p\n\nfunc f() {\n\tst.Prepare(`SELECT \"col name\", x FROM t WHERE x = 1`)\n}\n"
	rend, _ := renderFixture(c, src)
	v := violationAt(c, rend, "x FROM", "LT02",
		"Expected line break and indent of 4 spaces before 'x'.")
	edits, skipped := fixFixture(c, src, []fluffViolation{v})
	c.Assert(skipped, tc.Equals, 0)
	c.Assert(edits, tc.HasLen, 1)
	out := applyTo(c, src, edits)
	c.Check(strings.Contains(out, "\"col name\",\n    x FROM"), tc.IsTrue)
	c.Check(strings.Contains(out, `,\n    x FROM`), tc.IsFalse)
}

func (s *fixSuite) TestBlankLineAfterCTE(c *tc.C) {
	src := "package p\n\nfunc f() {\n\tst.Prepare(`WITH x AS (\n    SELECT a\n)\nSELECT a FROM x WHERE a = 1`)\n}\n"
	rend, _ := renderFixture(c, src)
	v := violationAt(c, rend, "SELECT a FROM x", "LT08",
		"Blank line expected but not found after CTE closing bracket.")
	edits, skipped := fixFixture(c, src, []fluffViolation{v})
	c.Assert(skipped, tc.Equals, 0)
	out := applyTo(c, src, edits)
	c.Check(strings.Contains(out, ")\n\nSELECT a FROM x"), tc.IsTrue)
}

func (s *fixSuite) TestInsertionInInterpretedStringEscapesNewline(c *tc.C) {
	// A newline insertion into an interpreted string must insert the
	// two-byte \n escape, not a raw newline that would break the Go
	// source.
	src := "package p\n\nfunc f() {\n\tst.Prepare(\"SELECT a, b FROM t WHERE x = 1\")\n}\n"
	rend, _ := renderFixture(c, src)
	v := violationAt(c, rend, "a, b", "LT02",
		"Expected line break and indent of 4 spaces before 'a'.")
	edits, _ := fixFixture(c, src, []fluffViolation{v})
	out := applyTo(c, src, edits)
	c.Check(strings.Contains(out, `SELECT\n    a, b FROM`), tc.IsTrue)
	c.Check(strings.Contains(out, "\n"), tc.IsTrue)
}

func (s *fixSuite) TestSkipsVirtualAnchoredViolation(c *tc.C) {
	// A violation anchored in synthetic content (the blanked Go code
	// before the first statement) must not produce an edit.
	src := "package p\n\nfunc f() {\n\tst.Prepare(`SELECT a FROM t WHERE a = 1`)\n}\n"
	edits, skipped := fixFixture(c, src, []fluffViolation{{
		LineNo:      1,
		LinePos:     1,
		Code:        "LT01",
		Description: "Unnecessary trailing whitespace.",
	}})
	c.Check(len(edits), tc.Equals, 0)
	c.Check(skipped, tc.Equals, 1)
}

func (s *fixSuite) TestSkipsUnfixableMessage(c *tc.C) {
	src := "package p\n\nfunc f() {\n\tst.Prepare(`SELECT a FROM t WHERE a = 1`)\n}\n"
	edits, skipped := fixFixture(c, src, []fluffViolation{{
		LineNo:      4,
		LinePos:     14,
		Code:        "PRS",
		Description: "Line 4, Position 14: Found unparsable section",
	}})
	c.Check(len(edits), tc.Equals, 0)
	c.Check(skipped, tc.Equals, 1)
}

func (s *fixSuite) TestRemoveSpaceBeforeBracket(c *tc.C) {
	src := `package p

func f() {
	st.Prepare("SELECT EXISTS (uuid) FROM t WHERE x = 1")
}
`
	rend, _ := renderFixture(c, src)
	v := violationAt(c, rend, " (uuid)", "LT01",
		"Unexpected whitespace before start bracket '('.")
	edits, skipped := fixFixture(c, src, []fluffViolation{v})
	c.Assert(skipped, tc.Equals, 0)
	out := applyTo(c, src, edits)
	c.Check(strings.Contains(out, "EXISTS(uuid)"), tc.IsTrue)
}

func (s *fixSuite) TestCollapseSpaceWithEscapedTabMessage(c *tc.C) {
	// sqlfluff reports mixed runs as "Found ' \t'" with a literal
	// backslash-t; the class must still classify the message.
	src := "package p\n\nfunc f() {\n\tst.Prepare(`WITH \tblockdevice_pool AS (\n    SELECT x\n)`)\n}\n"
	rend, _ := renderFixture(c, src)
	v := violationAt(c, rend, " \t", "LT01",
		`Expected only single space before naked identifier. Found ' \t'.`)
	edits, skipped := fixFixture(c, src, []fluffViolation{v})
	c.Assert(skipped, tc.Equals, 0)
	out := applyTo(c, src, edits)
	c.Check(strings.Contains(out, "WITH blockdevice_pool AS"), tc.IsTrue)
}

func (s *fixSuite) TestSkipsEscapeAndReplacementAnchors(c *tc.C) {
	// An edit that started at an expanded escape would rewrite only the
	// escape's first source byte. One anchored at a sqlair replacement
	// would rewrite the expression's dummy token. Both must be refused.
	tests := []struct {
		name   string
		src    string
		needle string
		desc   string
	}{{
		name:   "escape",
		src:    "package p\n\nfunc f() {\n\tst.Prepare(\"SELECT id\\tFROM t WHERE x = 1\")\n}\n",
		needle: "\tFROM",
		desc:   "Expected only single space before 'FROM' keyword. Found '\\t'.",
	}, {
		name:   "replacement",
		src:    "package p\n\nfunc f() {\n\tst.Prepare(\"SELECT &Foo.* FROM t WHERE x = 1\")\n}\n",
		needle: "c FROM",
		desc:   "Expected single whitespace between naked identifier and 'FROM' keyword.",
	}}
	for _, test := range tests {
		c.Logf("case: %s", test.name)
		rend, _ := renderFixture(c, test.src)
		v := violationAt(c, rend, test.needle, "LT01", test.desc)
		edits, skipped := fixFixture(c, test.src, []fluffViolation{v})
		c.Check(edits, tc.HasLen, 0)
		c.Check(skipped, tc.Equals, 1)
	}
}

func (s *fixSuite) TestLiteralSpanRejectsUnsafeMappings(c *tc.C) {
	base := []origByte{{
		File: "foo.go", Offset: 10, Kind: KindLiteral,
	}, {
		File: "foo.go", Offset: 11, Kind: KindLiteral,
	}}
	tests := []struct {
		name string
		m    []origByte
	}{{
		name: "escape",
		m: []origByte{{
			File: "foo.go", Offset: 10, Kind: KindLiteral,
		}, {
			File: "foo.go", Offset: 11, Kind: KindEscape,
		}},
	}, {
		name: "replacement",
		m: []origByte{{
			File: "foo.go", Offset: 10, Kind: KindLiteral,
		}, {
			File: "foo.go", Offset: 11, Kind: KindReplacement,
		}},
	}, {
		name: "non-contiguous",
		m: []origByte{{
			File: "foo.go", Offset: 10, Kind: KindLiteral,
		}, {
			File: "foo.go", Offset: 12, Kind: KindLiteral,
		}},
	}, {
		name: "cross-file",
		m: []origByte{{
			File: "foo.go", Offset: 10, Kind: KindLiteral,
		}, {
			File: "bar.go", Offset: 11, Kind: KindLiteral,
		}},
	}}
	for _, test := range tests {
		c.Logf("case: %s", test.name)
		st := Statement{Eval: &evaluated{Content: "  ", Map: test.m}}
		_, ok := literalSpan(&st, 0, 2)
		c.Check(ok, tc.IsFalse)
	}
	st := Statement{Eval: &evaluated{Content: "  ", Map: base}}
	_, ok := literalSpan(&st, 0, 2)
	c.Check(ok, tc.IsTrue)
}

func (s *fixSuite) TestFixConverges(c *tc.C) {
	// Re-run detection and fixing until the fixture reaches a fixed
	// point. A self-triggering edit would keep finding the same spacing.
	src := "package p\n\nfunc f() {\n\tst.Prepare(`SELECT id  FROM t WHERE x = 1`)\n}\n"
	for range 2 {
		rend, _ := renderFixture(c, src)
		if !strings.Contains(string(rend.Synthetic), "  FROM") {
			break
		}
		v := violationAt(c, rend, "  FROM", "LT01",
			"Expected only single space before 'FROM' keyword. Found '  '.")
		edits, skipped := fixFixture(c, src, []fluffViolation{v})
		c.Assert(skipped, tc.Equals, 0)
		c.Assert(edits, tc.HasLen, 1)
		src = applyTo(c, src, edits)
	}
	rend, _ := renderFixture(c, src)
	c.Check(strings.Contains(string(rend.Synthetic), "  FROM"), tc.IsFalse)
	c.Check(strings.Contains(string(rend.Synthetic), " FROM"), tc.IsTrue)
}

func (s *fixSuite) TestApplyEditsWritesFilesBottomUp(c *tc.C) {
	root := c.MkDir()
	c.Assert(os.MkdirAll(filepath.Join(root, "b"), 0o755), tc.ErrorIsNil)
	c.Assert(os.WriteFile(filepath.Join(root, "a.go"), []byte("abcdef"), 0o644), tc.ErrorIsNil)
	c.Assert(os.WriteFile(filepath.Join(root, "b", "b.go"), []byte("123456"), 0o644), tc.ErrorIsNil)

	changed, err := applyEdits(root, []sourceEdit{{
		file: "a.go", start: 1, end: 2, repl: "X",
	}, {
		file: "b/b.go", start: 0, end: 1, repl: "x",
	}, {
		file: "a.go", start: 4, end: 5, repl: "Y",
	}, {
		file: "b/b.go", start: 5, end: 6, repl: "y",
	}})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(changed, tc.Equals, 2)
	a, err := os.ReadFile(filepath.Join(root, "a.go"))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(string(a), tc.Equals, "aXcdYf")
	b, err := os.ReadFile(filepath.Join(root, "b", "b.go"))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(string(b), tc.Equals, "x2345y")
}

func (s *fixSuite) TestApplyEditsRejectsOutOfRangeWithoutWriting(c *tc.C) {
	root := c.MkDir()
	path := filepath.Join(root, "a.go")
	c.Assert(os.WriteFile(path, []byte("abcdef"), 0o644), tc.ErrorIsNil)
	changed, err := applyEdits(root, []sourceEdit{{
		file: "a.go", start: 4, end: 7, repl: "X",
	}})
	c.Check(err, tc.ErrorMatches, "edit out of range in a.go: \\[4,7\\)")
	c.Check(changed, tc.Equals, 0)
	data, readErr := os.ReadFile(path)
	c.Assert(readErr, tc.ErrorIsNil)
	c.Check(string(data), tc.Equals, "abcdef")
}

func (s *fixSuite) TestNoSpaceInsideBlobLiteral(c *tc.C) {
	// SQLite hex blob literals like x'' are mis-parsed by sqlfluff as an
	// identifier plus a quoted literal; the fixer must not insert the
	// space sqlfluff then asks for, as x '' is invalid SQL.
	src := `package p

func f() {
	db.Exec("INSERT INTO t (a, b) VALUES (?, x'')")
}
`
	rend, _ := renderFixture(c, src)
	v := violationAt(c, rend, "''", "LT01",
		"Expected single whitespace between naked identifier and quoted literal.")
	edits, skipped := fixFixture(c, src, []fluffViolation{v})
	c.Check(len(edits), tc.Equals, 0)
	c.Check(skipped, tc.Equals, 1)
}
