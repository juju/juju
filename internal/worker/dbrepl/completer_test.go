// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dbrepl

import (
	"strings"
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/domain/schema"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/internal/database/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
)

type completerSuite struct {
	testing.DqliteSuite
}

func TestCompleterSuite(t *stdtesting.T) {
	testhelpers.PrintGoroutineLeaks(t, func(t *stdtesting.T) {
		tc.Run(t, &completerSuite{})
	})
}

func (s *completerSuite) SetUpTest(c *tc.C) {
	s.DqliteSuite.SetUpTest(c)

	s.ApplyDDL(c, &schematesting.SchemaApplier{
		Schema: schema.ModelDDL(),
	})
}

// complete returns the reconstructed candidate lines offered for the given
// input line, with the cursor at the end of the line.
func (s *completerSuite) complete(c *tc.C, line string) []string {
	comp := newSQLCompleter(
		func() database.TxnRunner { return s.DqliteSuite.TxnRunner() },
		c.Context(),
		loggertesting.WrapCheckLog(c),
	)
	cands, _ := comp.Do([]rune(line), len(line))

	out := make([]string, 0, len(cands))
	for _, r := range cands {
		out = append(out, line+string(r))
	}
	return out
}

func joined(cands []string) string {
	return strings.Join(cands, "\n")
}

func containsCandidate(cands [][]rune, target string) bool {
	for _, candidate := range cands {
		if string(candidate) == target {
			return true
		}
	}
	return false
}

func (s *completerSuite) TestCompleteKeywords(c *tc.C) {
	cands := s.complete(c, "SEL")
	c.Check(joined(cands), tc.Contains, "SELECT")
}

func (s *completerSuite) TestCompleteKeywordsEmptyLine(c *tc.C) {
	cands := s.complete(c, "")
	c.Check(joined(cands), tc.Contains, "SELECT")
	c.Check(joined(cands), tc.Contains, "DELETE")
}

func (s *completerSuite) TestCompleteTablesAfterFrom(c *tc.C) {
	cands := s.complete(c, "SELECT * FROM ")
	c.Check(joined(cands), tc.Contains, "unit")
	c.Check(joined(cands), tc.Contains, "machine")
	c.Check(joined(cands), tc.Contains, "application")
}

func (s *completerSuite) TestCompleteTablesAfterDeleteFrom(c *tc.C) {
	cands := s.complete(c, "DELETE FROM ")
	c.Check(joined(cands), tc.Contains, "unit")
}

func (s *completerSuite) TestCompleteTablesAfterJoin(c *tc.C) {
	cands := s.complete(c, "SELECT * FROM unit JOIN ")
	c.Check(joined(cands), tc.Contains, "machine")
}

func (s *completerSuite) TestCompleteClauseKeywordsAfterTable(c *tc.C) {
	cands := s.complete(c, "SELECT * FROM unit ")
	c.Check(joined(cands), tc.Contains, "WHERE")
	c.Check(cands, tc.HasLen, len(clauseKeywordCandidates))
}

func (s *completerSuite) TestCompleteMultiWordKeyword(c *tc.C) {
	comp := newSQLCompleter(
		func() database.TxnRunner { return s.DqliteSuite.TxnRunner() },
		c.Context(),
		loggertesting.WrapCheckLog(c),
	)
	cands, _ := comp.Do([]rune("INNER "), len("INNER "))
	c.Check(containsCandidate(cands, "JOIN"), tc.IsTrue)
	c.Check(containsCandidate(cands, "INNER JOIN"), tc.IsFalse)
}

func (s *completerSuite) TestCompleteTablesPrefix(c *tc.C) {
	cands := s.complete(c, "FROM un")
	c.Check(joined(cands), tc.Contains, "unit")
	c.Check(joined(cands), tc.Contains, "unit_principal")
	c.Check(joined(cands), tc.Contains, "unit_state")
}

func (s *completerSuite) TestCompleteColumnsDotPrefix(c *tc.C) {
	cands := s.complete(c, "SELECT unit.")
	c.Check(joined(cands), tc.Contains, "uuid")
	c.Check(joined(cands), tc.Contains, "name")
	c.Check(joined(cands), tc.Contains, "life_id")
}

func (s *completerSuite) TestCompleteColumnsDotPrefixPartial(c *tc.C) {
	cands := s.complete(c, "SELECT unit.u")
	c.Check(joined(cands), tc.Contains, "uuid")
}

func (s *completerSuite) TestCompleteColumnsDotPrefixUnknownTable(c *tc.C) {
	cands := s.complete(c, "SELECT nonexistent.")
	c.Check(cands, tc.HasLen, 0)
}

func (s *completerSuite) TestCompleteNilDatabase(c *tc.C) {
	comp := newSQLCompleter(
		func() database.TxnRunner { return nil },
		c.Context(),
		loggertesting.WrapCheckLog(c),
	)
	cands, _ := comp.Do([]rune("SELECT * FROM "), len("SELECT * FROM "))
	c.Check(cands, tc.IsNil)
}

func (s *completerSuite) TestCompleteColumnsInClause(c *tc.C) {
	cands := s.complete(c, "SELECT name FROM unit WHERE ")
	c.Check(joined(cands), tc.Contains, "uuid")
	c.Check(joined(cands), tc.Contains, "life_id")
	// clause keywords are still offered alongside columns
	c.Check(joined(cands), tc.Contains, "WHERE")
}
