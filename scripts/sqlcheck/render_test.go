// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"go/token"
	"slices"
	"strings"
	"testing"

	"github.com/juju/tc"

	coretesting "github.com/juju/juju/internal/testing"
)

type renderSuite struct {
	coretesting.BaseSuite
}

func TestRenderSuite(t *testing.T) {
	tc.Run(t, &renderSuite{})
}

func parseAndRender(c *tc.C, path, src string) (*parsedPackage, []Statement, *FileRender) {
	c.Helper()
	src = trimFixture(src)
	fset := token.NewFileSet()
	pkg, err := parsePackage(fset, []goFile{{Path: path, Src: []byte(src)}})
	c.Assert(err, tc.ErrorIsNil)
	next := 1
	stmts, skips := extractPackage(pkg, &next)
	c.Assert(skips, tc.HasLen, 0)
	c.Assert(len(stmts) > 0, tc.IsTrue)
	rend := renderPackage(pkg, stmts)[path]
	c.Assert(rend, tc.NotNil)
	return pkg, stmts, rend
}

func (s *renderSuite) TestPreservesSQLLineAndMapsColumns(c *tc.C) {
	src := `
package p

func f() {
	st.Prepare(` + "`" + `
SELECT id FROM t
` + "`" + `)
}
`
	pkg, _, rend := parseAndRender(c, "foo.go", src)
	c.Check(strings.Contains(string(rend.Synthetic), "SELECT id FROM t"), tc.IsTrue)

	idx := strings.Index(string(rend.Synthetic), "SELECT")
	c.Assert(idx >= 0, tc.IsTrue)
	seg, ok := rend.lookup(idx)
	c.Assert(ok, tc.IsTrue)
	c.Check(seg.Kind, tc.Equals, KindLiteral)
	origOff := seg.OriginalStart + (idx - seg.RenderedStart)
	pos := filePosition(pkg.fset, "foo.go", origOff)
	c.Check(pos.Line, tc.Equals, 5)
	c.Check(strings.Contains(src, "SELECT"), tc.IsTrue)
}

func (s *renderSuite) TestReplacementMapsToExpressionStart(c *tc.C) {
	src := `
package p

func f() {
	st.Prepare("SELECT &Foo.x FROM t WHERE k = 1")
}
`
	_, stmts, rend := parseAndRender(c, "foo.go", src)
	c.Assert(stmts, tc.HasLen, 1)
	idx := strings.Index(string(rend.Synthetic), "c")
	c.Assert(idx >= 0, tc.IsTrue)
	seg, ok := rend.lookup(idx)
	c.Assert(ok, tc.IsTrue)
	c.Check(seg.Kind, tc.Equals, KindReplacement)
}

func (s *renderSuite) TestVirtualSemicolonDoesNotLeak(c *tc.C) {
	src := `
package p

func f() {
	st.Prepare("SELECT id FROM t WHERE x = 1")
	st.Prepare("SELECT name FROM u WHERE y = 2")
}
`
	_, _, rend := parseAndRender(c, "foo.go", src)
	c.Check(strings.Contains(string(rend.Synthetic), ";"), tc.IsTrue)
	for _, seg := range rend.Segments {
		if seg.Kind == KindVirtual && rend.Synthetic[seg.RenderedStart] == ';' {
			c.Check(seg.StmtID, tc.Equals, 0)
		}
	}
}

func (s *renderSuite) TestUnicodeMappingUsesCharacters(c *tc.C) {
	src := `
package p

func f() {
	st.Prepare("SELECT 'é' AS x FROM t")
}
`
	pkg, _, rend := parseAndRender(c, "foo.go", src)
	idx := strings.Index(string(rend.Synthetic), "AS")
	c.Assert(idx >= 0, tc.IsTrue)
	line, col := 1, 1
	for i := range idx {
		if rend.Synthetic[i] == '\n' {
			line++
			col = 1
		} else {
			col++
		}
	}
	offset := lineColToOffset(rend.Synthetic, line, col)
	seg, ok := rend.lookup(offset)
	c.Assert(ok, tc.IsTrue)
	pos := filePosition(pkg.fset, "foo.go", seg.OriginalStart+(offset-seg.RenderedStart))
	c.Check(pos.Line > 0, tc.IsTrue)
	c.Check(pos.Column > 0, tc.IsTrue)
}

func (s *renderSuite) TestSemicolonAttachesAfterTrailingWhitespace(c *tc.C) {
	src := `
package p

func f(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, ` + "`" + `
		INSERT INTO cloud (uuid)
		VALUES (?)
	` + "`" + `)
	return err
}
`
	_, _, rend := parseAndRender(c, "foo.go", src)
	synthetic := string(rend.Synthetic)
	// The raw string closes with "\n\t"; the virtual semicolon must
	// attach to the last SQL token, not to the whitespace-only tail.
	// The ? placeholder is rendered as 1 (sqlfluff cannot parse ?).
	c.Check(strings.Contains(synthetic, "VALUES (1);"), tc.IsTrue)
}

func (s *renderSuite) TestEscapedNewline(c *tc.C) {
	src := `
package p

func f() {
	st.Prepare("SELECT id FROM t WHERE x = 1\nAND y = 2")
}
`
	_, _, rend := parseAndRender(c, "foo.go", src)
	c.Check(strings.Contains(string(rend.Synthetic), "\n"), tc.IsTrue)
	c.Check(strings.Contains(string(rend.Synthetic), "AND y = 2"), tc.IsTrue)
}

func (s *renderSuite) TestReplacementAndVirtualAreClassified(c *tc.C) {
	src := `
package p

func f() {
	st.Prepare("SELECT &Foo.field FROM t WHERE x = 1")
}
`
	_, _, rend := parseAndRender(c, "foo.go", src)
	var kinds []SegmentKind
	for _, seg := range rend.Segments {
		kinds = append(kinds, seg.Kind)
	}
	c.Check(containsKind(kinds, KindReplacement), tc.IsTrue)
	c.Check(containsKind(kinds, KindVirtual), tc.IsTrue)
}

func (s *renderSuite) TestSemicolonBeforeTrailingLineComment(c *tc.C) {
	// Appending the terminator after a -- comment would comment out the
	// semicolon itself and produce an unterminated-statement finding.
	src := `
package p

func f() {
	db.Exec(` + "`" + `SELECT id FROM t -- note
` + "`" + `)
}
`
	_, _, rend := parseAndRender(c, "foo.go", src)
	c.Check(strings.Contains(string(rend.Synthetic), "SELECT id FROM t; -- note"), tc.IsTrue)
}

func (s *renderSuite) TestSemicolonBeforeCommentOnFollowingLine(c *tc.C) {
	src := `
package p

func f() {
	db.Exec(` + "`" + `SELECT id FROM t
-- note` + "`" + `)
}
`
	_, _, rend := parseAndRender(c, "foo.go", src)
	c.Check(strings.Contains(string(rend.Synthetic), "SELECT id FROM t;\n-- note"), tc.IsTrue)
}

func (s *renderSuite) TestCommentMarkerInsideQuotedLiteral(c *tc.C) {
	src := `
package p

func f() {
	db.Exec("SELECT '-- not a comment' AS note FROM t")
}
`
	_, _, rend := parseAndRender(c, "foo.go", src)
	c.Check(strings.Contains(string(rend.Synthetic), "SELECT '-- not a comment' AS note FROM t;"), tc.IsTrue)
}

func containsKind(kinds []SegmentKind, want SegmentKind) bool {
	return slices.Contains(kinds, want)
}

func (s *renderSuite) TestLineColToOffsetUnicode(c *tc.C) {
	src := []byte("SELECT 'é' as x")
	// Python/sqlfluff columns are 1-based rune positions.
	// 'a' of as is after: SELECT 'é'
	offset := lineColToOffset(src, 1, 12)
	c.Check(src[offset], tc.Equals, byte('a'))
}
