// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"testing"

	"github.com/juju/tc"
)

type txnCheckSuite struct{}

func TestTxnCheckSuite(t *testing.T) {
	tc.Run(t, &txnCheckSuite{})
}

func (s *txnCheckSuite) TestAllowsCapturedReassignment(c *tc.C) {
	findings := checkSource(c, `
package p

import (
	"context"

	"github.com/canonical/sqlair"
)

type txnRunner interface {
	Txn(context.Context, func(context.Context, *sqlair.TX) error) error
}

func f(ctx context.Context, db txnRunner) error {
	var result []string
	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		local := []string{"foo"}
		result = local
		return nil
	})
}
`[1:])

	c.Check(findings, tc.HasLen, 0)
}

func (s *txnCheckSuite) TestAllowsCapturedAppendAfterReassignment(c *tc.C) {
	findings := checkSource(c, `
package p

import (
	"context"

	"github.com/canonical/sqlair"
)

type txnRunner interface {
	Txn(context.Context, func(context.Context, *sqlair.TX) error) error
}

func f(ctx context.Context, db txnRunner) error {
	var result []string
	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		result = []string{}
		result = append(result, "foo")
		return nil
	})
}
`[1:])

	c.Check(findings, tc.HasLen, 0)
}

func (s *txnCheckSuite) TestAllowsCapturedMapMutationAfterReassignment(c *tc.C) {
	findings := checkSource(c, `
package p

import (
	"context"

	"github.com/canonical/sqlair"
)

type txnRunner interface {
	Txn(context.Context, func(context.Context, *sqlair.TX) error) error
}

func f(ctx context.Context, db txnRunner) error {
	result := map[string]string{}
	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		result = map[string]string{}
		result["foo"] = "bar"
		delete(result, "foo")
		clear(result)
		return nil
	})
}
`[1:])

	c.Check(findings, tc.HasLen, 0)
}

func (s *txnCheckSuite) TestAllowsCapturedFieldAndIncrementAfterReassignment(c *tc.C) {
	findings := checkSource(c, `
package p

import (
	"context"

	"github.com/canonical/sqlair"
)

type txnRunner interface {
	Txn(context.Context, func(context.Context, *sqlair.TX) error) error
}

type value struct {
	name string
}

func f(ctx context.Context, db txnRunner) error {
	result := &value{}
	var count int
	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		result = &value{}
		result.name = "foo"
		count = 0
		count++
		return nil
	})
}
`[1:])

	c.Check(findings, tc.HasLen, 0)
}

func (s *txnCheckSuite) TestAllowsCapturedMutationInsideBranchAfterReassignment(c *tc.C) {
	findings := checkSource(c, `
package p

import (
	"context"

	"github.com/canonical/sqlair"
)

type txnRunner interface {
	Txn(context.Context, func(context.Context, *sqlair.TX) error) error
}

func f(ctx context.Context, db txnRunner, ok bool) error {
	var result []string
	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if ok {
			result = []string{}
			result = append(result, "foo")
		}
		return nil
	})
}
`[1:])

	c.Check(findings, tc.HasLen, 0)
}

func (s *txnCheckSuite) TestRejectsCapturedAppend(c *tc.C) {
	findings := checkSource(c, `
package p

import (
	"context"

	"github.com/canonical/sqlair"
)

type txnRunner interface {
	Txn(context.Context, func(context.Context, *sqlair.TX) error) error
}

func f(ctx context.Context, db txnRunner) error {
	var result []string
	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		result = append(result, "foo")
		return nil
	})
}
`[1:])

	c.Assert(findings, tc.HasLen, 1)
	c.Check(findings[0].message, tc.Equals, `found captured mutation in transaction: "result" is mutated by append`)
}

func (s *txnCheckSuite) TestRejectsCapturedAppendAfterConditionalReassignment(c *tc.C) {
	findings := checkSource(c, `
package p

import (
	"context"

	"github.com/canonical/sqlair"
)

type txnRunner interface {
	Txn(context.Context, func(context.Context, *sqlair.TX) error) error
}

func f(ctx context.Context, db txnRunner, ok bool) error {
	var result []string
	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if ok {
			result = []string{}
		}
		result = append(result, "foo")
		return nil
	})
}
`[1:])

	c.Assert(findings, tc.HasLen, 1)
	c.Check(findings[0].message, tc.Equals, `found captured mutation in transaction: "result" is mutated by append`)
}

func (s *txnCheckSuite) TestRejectsCapturedAppendBeforeReassignment(c *tc.C) {
	findings := checkSource(c, `
package p

import (
	"context"

	"github.com/canonical/sqlair"
)

type txnRunner interface {
	Txn(context.Context, func(context.Context, *sqlair.TX) error) error
}

func f(ctx context.Context, db txnRunner) error {
	var result []string
	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		result = append(result, "foo")
		result = []string{}
		return nil
	})
}
`[1:])

	c.Assert(findings, tc.HasLen, 1)
	c.Check(findings[0].message, tc.Equals, `found captured mutation in transaction: "result" is mutated by append`)
}

func (s *txnCheckSuite) TestRejectsCapturedAppendEvenWhenAssignedToLocal(c *tc.C) {
	findings := checkSource(c, `
package p

import (
	"context"

	"github.com/canonical/sqlair"
)

type txnRunner interface {
	Txn(context.Context, func(context.Context, *sqlair.TX) error) error
}

func f(ctx context.Context, db txnRunner) error {
	var result []string
	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		local := append(result, "foo")
		_ = local
		return nil
	})
}
`[1:])

	c.Assert(findings, tc.HasLen, 1)
	c.Check(findings[0].message, tc.Equals, `found captured mutation in transaction: "result" is mutated by append`)
}

func (s *txnCheckSuite) TestRejectsCapturedIndexAssignment(c *tc.C) {
	findings := checkSource(c, `
package p

import (
	"context"

	"github.com/canonical/sqlair"
)

type txnRunner interface {
	Txn(context.Context, func(context.Context, *sqlair.TX) error) error
}

func f(ctx context.Context, db txnRunner) error {
	result := map[string]string{}
	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		result["foo"] = "bar"
		return nil
	})
}
`[1:])

	c.Assert(findings, tc.HasLen, 1)
	c.Check(findings[0].message, tc.Equals, `found captured mutation in transaction: "result" is mutated by assignment`)
}

func (s *txnCheckSuite) TestRejectsCapturedFieldAssignment(c *tc.C) {
	findings := checkSource(c, `
package p

import (
	"context"

	"github.com/canonical/sqlair"
)

type txnRunner interface {
	Txn(context.Context, func(context.Context, *sqlair.TX) error) error
}

type result struct {
	value string
}

func f(ctx context.Context, db txnRunner) error {
	var result result
	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		result.value = "foo"
		return nil
	})
}
`[1:])

	c.Assert(findings, tc.HasLen, 1)
	c.Check(findings[0].message, tc.Equals, `found captured mutation in transaction: "result" is mutated by assignment`)
}

func (s *txnCheckSuite) TestRejectsCapturedPointerAssignment(c *tc.C) {
	findings := checkSource(c, `
package p

import (
	"context"

	"github.com/canonical/sqlair"
)

type txnRunner interface {
	Txn(context.Context, func(context.Context, *sqlair.TX) error) error
}

func f(ctx context.Context, db txnRunner) error {
	result := new(string)
	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		*result = "foo"
		return nil
	})
}
`[1:])

	c.Assert(findings, tc.HasLen, 1)
	c.Check(findings[0].message, tc.Equals, `found captured mutation in transaction: "result" is mutated by assignment`)
}

func (s *txnCheckSuite) TestRejectsCapturedCompoundAssignment(c *tc.C) {
	findings := checkSource(c, `
package p

import (
	"context"

	"github.com/canonical/sqlair"
)

type txnRunner interface {
	Txn(context.Context, func(context.Context, *sqlair.TX) error) error
}

func f(ctx context.Context, db txnRunner) error {
	var result int
	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		result += 1
		return nil
	})
}
`[1:])

	c.Assert(findings, tc.HasLen, 1)
	c.Check(findings[0].message, tc.Equals, `found captured mutation in transaction: "result" is mutated by compound assignment`)
}

func (s *txnCheckSuite) TestRejectsCapturedIncrement(c *tc.C) {
	findings := checkSource(c, `
package p

import (
	"context"

	"github.com/canonical/sqlair"
)

type txnRunner interface {
	Txn(context.Context, func(context.Context, *sqlair.TX) error) error
}

func f(ctx context.Context, db txnRunner) error {
	var result int
	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		result++
		return nil
	})
}
`[1:])

	c.Assert(findings, tc.HasLen, 1)
	c.Check(findings[0].message, tc.Equals, `found captured mutation in transaction: "result" is mutated by increment/decrement`)
}

func (s *txnCheckSuite) TestRejectsCapturedBuiltinMutations(c *tc.C) {
	findings := checkSource(c, `
package p

import (
	"context"

	"github.com/canonical/sqlair"
)

type txnRunner interface {
	Txn(context.Context, func(context.Context, *sqlair.TX) error) error
}

func f(ctx context.Context, db txnRunner) error {
	dst := []string{"foo"}
	values := map[string]string{"foo": "bar"}
	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		copy(dst, []string{"bar"})
		delete(values, "foo")
		clear(values)
		return nil
	})
}
`[1:])

	c.Assert(findings, tc.HasLen, 3)
	c.Check(findingMessages(findings), tc.SameContents, []string{
		`found captured mutation in transaction: "dst" is mutated by copy`,
		`found captured mutation in transaction: "values" is mutated by delete`,
		`found captured mutation in transaction: "values" is mutated by clear`,
	})
}

func (s *txnCheckSuite) TestRejectsCapturedMutationInNestedFunction(c *tc.C) {
	findings := checkSource(c, `
package p

import (
	"context"

	"github.com/canonical/sqlair"
)

type txnRunner interface {
	Txn(context.Context, func(context.Context, *sqlair.TX) error) error
}

func f(ctx context.Context, db txnRunner) error {
	var result []string
	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		collect := func() {
			result = append(result, "foo")
		}
		collect()
		return nil
	})
}
`[1:])

	c.Assert(findings, tc.HasLen, 1)
	c.Check(findings[0].message, tc.Equals, `found captured mutation in transaction: "result" is mutated by append`)
}

func (s *txnCheckSuite) TestAllowsCapturedMutationInNestedFunctionAfterReassignment(c *tc.C) {
	findings := checkSource(c, `
package p

import (
	"context"

	"github.com/canonical/sqlair"
)

type txnRunner interface {
	Txn(context.Context, func(context.Context, *sqlair.TX) error) error
}

func f(ctx context.Context, db txnRunner) error {
	var result []string
	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		result = []string{}
		collect := func() {
			result = append(result, "foo")
		}
		collect()
		return nil
	})
}
`[1:])

	c.Check(findings, tc.HasLen, 0)
}

func (s *txnCheckSuite) TestReassignmentInNestedFunctionDoesNotPermitOuterMutation(c *tc.C) {
	findings := checkSource(c, `
package p

import (
	"context"

	"github.com/canonical/sqlair"
)

type txnRunner interface {
	Txn(context.Context, func(context.Context, *sqlair.TX) error) error
}

func f(ctx context.Context, db txnRunner) error {
	var result []string
	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		reset := func() {
			result = []string{}
		}
		reset()
		result = append(result, "foo")
		return nil
	})
}
`[1:])

	c.Assert(findings, tc.HasLen, 1)
	c.Check(findings[0].message, tc.Equals, `found captured mutation in transaction: "result" is mutated by append`)
}

func (s *txnCheckSuite) TestAllowsLocalMutations(c *tc.C) {
	findings := checkSource(c, `
package p

import (
	"context"

	"github.com/canonical/sqlair"
)

type txnRunner interface {
	Txn(context.Context, func(context.Context, *sqlair.TX) error) error
}

func f(ctx context.Context, db txnRunner) error {
	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		result := []string{}
		result = append(result, "foo")
		values := map[string]string{}
		values["foo"] = "bar"
		count := 0
		count++
		return nil
	})
}
`[1:])

	c.Check(findings, tc.HasLen, 0)
}

func (s *txnCheckSuite) TestRejectsAssertAndCheck(c *tc.C) {
	findings := checkSource(c, `
package p

import (
	"context"

	"github.com/canonical/sqlair"
)

type C struct{}

func (C) Assert(...any) {}
func (C) Check(...any)  {}

type txnRunner interface {
	Txn(context.Context, func(context.Context, *sqlair.TX) error) error
}

func f(ctx context.Context, db txnRunner, c C) error {
	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		c.Assert(nil)
		c.Check(nil)
		return nil
	})
}
`[1:])

	c.Assert(findings, tc.HasLen, 2)
	c.Check(findingMessages(findings), tc.SameContents, []string{
		"found assert in transaction",
		"found check in transaction",
	})
}

func (s *txnCheckSuite) TestCanRunOnlyAssertCheck(c *tc.C) {
	findings := checkSourceWithChecks(c, `
package p

import (
	"context"

	"github.com/canonical/sqlair"
)

type C struct{}

func (C) Assert(...any) {}

type txnRunner interface {
	Txn(context.Context, func(context.Context, *sqlair.TX) error) error
}

func f(ctx context.Context, db txnRunner, c C) error {
	var result []string
	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		result = append(result, "foo")
		c.Assert(nil)
		return nil
	})
}
`[1:], []nodeCheck{assertCheck{}})

	c.Assert(findings, tc.HasLen, 1)
	c.Check(findings[0].message, tc.Equals, "found assert in transaction")
}

func (s *txnCheckSuite) TestCanRunOnlyMutationChecks(c *tc.C) {
	findings := checkSourceWithChecks(c, `
package p

import (
	"context"

	"github.com/canonical/sqlair"
)

type C struct{}

func (C) Assert(...any) {}

type txnRunner interface {
	Txn(context.Context, func(context.Context, *sqlair.TX) error) error
}

func f(ctx context.Context, db txnRunner, c C) error {
	var result []string
	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		result = append(result, "foo")
		c.Assert(nil)
		return nil
	})
}
`[1:], []nodeCheck{
		capturedAssignmentCheck{},
		capturedBuiltinMutationCheck{},
	})

	c.Assert(findings, tc.HasLen, 1)
	c.Check(findings[0].message, tc.Equals, `found captured mutation in transaction: "result" is mutated by append`)
}

func (s *txnCheckSuite) TestRegisteredChecksRunTogether(c *tc.C) {
	findings := checkSourceWithChecks(c, `
package p

import (
	"context"

	"github.com/canonical/sqlair"
)

type C struct{}

func (C) Assert(...any) {}

type txnRunner interface {
	Txn(context.Context, func(context.Context, *sqlair.TX) error) error
}

func f(ctx context.Context, db txnRunner, c C) error {
	var result []string
	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		result = append(result, "foo")
		c.Assert(nil)
		return nil
	})
}
`[1:], []nodeCheck{
		assertCheck{},
		capturedBuiltinMutationCheck{},
	})

	c.Assert(findings, tc.HasLen, 2)
	c.Check(findingMessages(findings), tc.SameContents, []string{
		`found captured mutation in transaction: "result" is mutated by append`,
		"found assert in transaction",
	})
}

func checkSource(c *tc.C, source string) []finding {
	return checkSourceWithChecks(c, source, defaultChecks())
}

func checkSourceWithChecks(c *tc.C, source string, checks []nodeCheck) []finding {
	findings, err := checkFileWithChecks("test.go", []byte(source), checks)
	c.Assert(err, tc.ErrorIsNil)
	return findings
}

func findingMessages(findings []finding) []string {
	messages := make([]string, 0, len(findings))
	for _, finding := range findings {
		messages = append(messages, finding.message)
	}
	return messages
}
