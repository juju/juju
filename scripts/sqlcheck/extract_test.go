// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"go/token"
	"strings"
	"testing"

	"github.com/juju/tc"

	coretesting "github.com/juju/juju/internal/testing"
)

type extractSuite struct {
	coretesting.BaseSuite
}

func TestExtractSuite(t *testing.T) {
	tc.Run(t, &extractSuite{})
}

func extractSrc(c *tc.C, src string) ([]Statement, []Skip) {
	c.Helper()
	src = trimFixture(src)
	fset := token.NewFileSet()
	pkg, err := parsePackage(fset, []goFile{{Path: "foo.go", Src: []byte(src)}})
	c.Assert(err, tc.ErrorIsNil)
	next := 1
	return extractPackage(pkg, &next)
}

func trimFixture(src string) string {
	if strings.HasPrefix(src, "\n") {
		src = src[1:]
	}
	return src
}

func (s *extractSuite) TestRawMultilineSQLair(c *tc.C) {
	stmts, skips := extractSrc(c, `
package p

import "github.com/canonical/sqlair"

func f() {
	sqlair.MustPrepare(`+"`"+`
SELECT &Foo.x
FROM   t
WHERE  id = $Bar.id
`+"`"+`, Foo{}, Bar{})
}
`)
	c.Assert(skips, tc.HasLen, 0)
	c.Assert(stmts, tc.HasLen, 1)
	c.Check(strings.Contains(stmts[0].Eval.Content, "SELECT c"), tc.IsTrue)
	c.Check(strings.Contains(stmts[0].Eval.Content, "FROM   t"), tc.IsTrue)
	c.Check(strings.Contains(stmts[0].Eval.Content, "id = 1"), tc.IsTrue)
}

func (s *extractSuite) TestPrepareLocalVar(c *tc.C) {
	stmts, skips := extractSrc(c, `
package p

func (st *State) F() {
	q := `+"`"+`
	SELECT (*) AS (&cloudWithAuthType.*)
    FROM v_cloud_auth
	WHERE uuid = $cloudID.uuid
`+"`"+`
	st.Prepare(q, cloudID{}, cloudWithAuthType{})
}
`)
	c.Assert(skips, tc.HasLen, 0)
	c.Assert(stmts, tc.HasLen, 1)
	c.Check(strings.Contains(stmts[0].Eval.Content, "FROM v_cloud_auth"), tc.IsTrue)
	c.Check(strings.Contains(stmts[0].Eval.Content, "&cloudWithAuthType"), tc.IsFalse)
}

func (s *extractSuite) TestConstantConcatAndSliceIdiom(c *tc.C) {
	stmts, skips := extractSrc(c, `
package p

const prefix = `+"`"+`
SELECT id
`+"`"+`[1:]

func f() {
	st.Prepare(prefix + "FROM t WHERE x = 1", nil)
}
`)
	c.Assert(skips, tc.HasLen, 0)
	c.Assert(stmts, tc.HasLen, 1)
	c.Check(strings.HasPrefix(strings.TrimSpace(stmts[0].Eval.Content), "SELECT"), tc.IsTrue)
	c.Check(strings.Contains(stmts[0].Eval.Content, "FROM t"), tc.IsTrue)
}

func (s *extractSuite) TestSprintfQueryWithoutSink(c *tc.C) {
	stmts, skips := extractSrc(c, `
package p

import "fmt"

func f() {
	table.Query = fmt.Sprintf("SELECT &%s.* FROM %q", structName, tableName)
}
`)
	c.Assert(skips, tc.HasLen, 0)
	c.Assert(stmts, tc.HasLen, 1)
	c.Check(strings.Contains(stmts[0].Eval.Content, "%"), tc.IsFalse)
	c.Check(strings.Contains(stmts[0].Eval.Content, "&"), tc.IsFalse)
}

func (s *extractSuite) TestSprintfVerbs(c *tc.C) {
	stmts, skips := extractSrc(c, `
package p

import "fmt"

func f() {
	query := fmt.Sprintf(`+"`"+`
SELECT COALESCE(SUM(%s), 0) AS &result.size
FROM %q
`+"`"+`, "octet_length(x)", "operation")
	st.Prepare(query)
}
`)
	c.Assert(skips, tc.HasLen, 0)
	c.Assert(stmts, tc.HasLen, 1)
	c.Check(strings.Contains(stmts[0].Eval.Content, "%s"), tc.IsFalse)
	c.Check(strings.Contains(stmts[0].Eval.Content, "&result.size"), tc.IsFalse)
}

func (s *extractSuite) TestStringsJoinStatic(c *tc.C) {
	stmts, skips := extractSrc(c, `
package p

import "strings"

func f() {
	st.Prepare(strings.Join([]string{
		"SELECT id",
		"FROM t",
		"WHERE x = 1",
	}, "\n"))
}
`)
	c.Assert(skips, tc.HasLen, 0)
	c.Assert(stmts, tc.HasLen, 1)
	c.Check(strings.Contains(stmts[0].Eval.Content, "FROM t"), tc.IsTrue)
}

func (s *extractSuite) TestDynamicSkip(c *tc.C) {
	stmts, skips := extractSrc(c, `
package p

func f(name string) {
	q := "SELECT id FROM t"
	q += " WHERE name = '" + name + "'"
	st.Prepare(q)
}
`)
	c.Check(stmts, tc.HasLen, 0)
	c.Assert(skips, tc.HasLen, 1)
	c.Check(skips[0].Reason, tc.Equals, "could not reconstruct SQL: string is appended to at runtime")
}

func (s *extractSuite) TestReassignedStraightLineExtractsEach(c *tc.C) {
	stmts, skips := extractSrc(c, `
package p

func (st *State) GetUnitState() {
	q := "SELECT &unitState.* FROM unit_state WHERE unit_uuid = $entityUUID.uuid"
	st.Prepare(q, unitState{}, entityUUID{})
	q = `+"`"+`
SELECT &unitCharmStateKeyVal.*
FROM unit_state_charm
WHERE unit_uuid = $entityUUID.uuid`+"`"+`
	st.Prepare(q, unitCharmStateKeyVal{}, entityUUID{})
	q = `+"`"+`
SELECT &unitRelationStateKeyVal.*
FROM unit_state_relation
WHERE unit_uuid = $entityUUID.uuid`+"`"+`
	st.Prepare(q, unitRelationStateKeyVal{}, entityUUID{})
}
`)
	c.Assert(skips, tc.HasLen, 0)
	c.Assert(stmts, tc.HasLen, 3)
	c.Check(strings.Contains(stmts[0].Eval.Content, "unit_state"), tc.IsTrue)
	c.Check(strings.Contains(stmts[1].Eval.Content, "unit_state_charm"), tc.IsTrue)
	c.Check(strings.Contains(stmts[2].Eval.Content, "unit_state_relation"), tc.IsTrue)
}

func (s *extractSuite) TestQueryNonSQLArgumentIgnored(c *tc.C) {
	stmts, skips := extractSrc(c, `
package p

func f() {
	path.Query("q", query)
	path.Query("fields", defaultInfoFilter())
}
`)
	c.Check(stmts, tc.HasLen, 0)
	c.Check(skips, tc.HasLen, 0)
}

func (s *extractSuite) TestDoesNotSkipQueryStatementObjects(c *tc.C) {
	_, skips := extractSrc(c, `
package p

func f(ctx context.Context, tx *sqlair.TX, stmt *sqlair.Statement) {
	tx.Query(ctx, stmt)
}
`)
	c.Check(skips, tc.HasLen, 0)
}

func (s *extractSuite) TestIgnoreDirectives(c *tc.C) {
	stmts, _ := extractSrc(c, `
package p

func f() {
	// sqlcheck-ignore CP01
	st.Prepare("SELECT id FROM t")
	st.Prepare("SELECT id FROM u") // sqlcheck-ignore
	// sqlcheck-ignore LT01,CP01
	const q = "SELECT id FROM v"
	st.Prepare(q)
}
`)
	c.Assert(stmts, tc.HasLen, 3)
	c.Check(stmts[0].IgnoreRules["CP01"], tc.IsTrue)
	c.Check(stmts[1].IgnoreAll, tc.IsTrue)
	c.Check(stmts[2].IgnoreRules["LT01"], tc.IsTrue)
	c.Check(stmts[2].IgnoreRules["CP01"], tc.IsTrue)
}

func (s *extractSuite) TestDedupSharedConstant(c *tc.C) {
	stmts, _ := extractSrc(c, `
package p

const q = "SELECT id FROM t WHERE x = 1"

func f() {
	st.Prepare(q)
	st.Prepare(q)
}
`)
	c.Assert(stmts, tc.HasLen, 1)
}

func (s *extractSuite) TestInterpretedStringAndEscapes(c *tc.C) {
	stmts, skips := extractSrc(c, `
package p

func f() {
	st.Prepare("SELECT \"id\" FROM t WHERE x = 1\n")
}
`)
	c.Assert(skips, tc.HasLen, 0)
	c.Assert(stmts, tc.HasLen, 1)
	c.Check(strings.Contains(stmts[0].Eval.Content, `"id"`), tc.IsTrue)
	c.Check(strings.Contains(stmts[0].Eval.Content, "\n"), tc.IsTrue)
}

func (s *extractSuite) TestInterpretedStringHexEscape(c *tc.C) {
	// A multi-digit hex escape must be consumed entirely: the last
	// hex digit must not leak into the rendered content as a
	// literal byte.
	stmts, skips := extractSrc(c, `
package p

func f() {
	db.Query("SELECT \"id\" FROM t WHERE x = \x41")
}
`)
	c.Assert(skips, tc.HasLen, 0)
	c.Assert(stmts, tc.HasLen, 1)
	c.Check(strings.Contains(stmts[0].Eval.Content, `"id"`), tc.IsTrue)
	c.Check(strings.Contains(stmts[0].Eval.Content, "1"), tc.IsFalse)
}

func (s *extractSuite) TestMakePatchSink(c *tc.C) {
	stmts, skips := extractSrc(c, `
package p

func f() {
	schema.MakePatch("CREATE TABLE foo(id INT PRIMARY KEY);")
}
`)
	c.Assert(skips, tc.HasLen, 0)
	c.Assert(stmts, tc.HasLen, 1)
}

func (s *extractSuite) TestHeuristicStandaloneSQL(c *tc.C) {
	stmts, skips := extractSrc(c, `
package p

const orphan = "SELECT id FROM orphan_table WHERE x = 1"
`)
	c.Assert(skips, tc.HasLen, 0)
	c.Assert(stmts, tc.HasLen, 1)
}

func (s *extractSuite) TestHeuristicRejectsEnglish(c *tc.C) {
	stmts, _ := extractSrc(c, `
package p

const help = `+"`"+`
Begin with all the log messages:

    juju debug-log --replay (maybe)
`+"`"+`
`)
	c.Check(stmts, tc.HasLen, 0)
}

func (s *extractSuite) TestHeuristicRejectsPlainStrings(c *tc.C) {
	stmts, _ := extractSrc(c, `
package p

const msg = "hello world this is not SQL"
`)
	c.Check(stmts, tc.HasLen, 0)
}

func (s *extractSuite) TestSinkSelectWithoutClause(c *tc.C) {
	// A recognised sink must not silently drop a SELECT statement just
	// because it has no FROM clause.
	stmts, skips := extractSrc(c, `
package p

func f() {
	db.Exec("SELECT last_insert_rowid()")
}
`)
	c.Assert(skips, tc.HasLen, 0)
	c.Assert(stmts, tc.HasLen, 1)
	c.Check(stmts[0].Eval.Content, tc.Equals, "SELECT last_insert_rowid()")
}

func (s *extractSuite) TestSinkMultiStatementString(c *tc.C) {
	stmts, skips := extractSrc(c, `
package p

func f() {
	db.Exec("INSERT INTO t VALUES (1); SELECT last_insert_rowid()")
}
`)
	c.Assert(skips, tc.HasLen, 0)
	c.Assert(stmts, tc.HasLen, 1)
	c.Check(strings.Contains(stmts[0].Eval.Content, "SELECT last_insert_rowid()"), tc.IsTrue)
}

func (s *extractSuite) TestCrossFilePackageVariable(c *tc.C) {
	// Package-level variables have no ast.Object at a cross-file use.
	// Resolve their static initialisers package-wide instead of dropping
	// the query without a coverage note.
	fset := token.NewFileSet()
	pkg, err := parsePackage(fset, []goFile{{
		Path: "query.go",
		Src:  []byte("package p\n\nvar query = \"SELECT id FROM t WHERE x = 1\"\n"),
	}, {
		Path: "foo.go",
		Src:  []byte("package p\n\nfunc f() {\n\tdb.Exec(query)\n}\n"),
	}})
	c.Assert(err, tc.ErrorIsNil)
	next := 1
	stmts, skips := extractPackage(pkg, &next)
	c.Assert(skips, tc.HasLen, 0)
	c.Assert(stmts, tc.HasLen, 1)
	c.Check(stmts[0].File, tc.Equals, "query.go")
	c.Check(stmts[0].Eval.Content, tc.Equals, "SELECT id FROM t WHERE x = 1")
}

func (s *extractSuite) TestPR23696StyleExtraction(c *tc.C) {
	stmts, skips := extractSrc(c, `
package p

func f() {
	st.Prepare(`+"`"+`
SELECT &Secret.*
FROM   secret
WHERE  uuid = $M.uuid AND name = "double-quoted"
`+"`"+`)
}
`)
	c.Assert(skips, tc.HasLen, 0)
	c.Assert(stmts, tc.HasLen, 1)
	c.Check(strings.Contains(stmts[0].Eval.Content, `"double-quoted"`), tc.IsTrue)
	c.Check(strings.Contains(stmts[0].Eval.Content, "&Secret"), tc.IsFalse)
}
