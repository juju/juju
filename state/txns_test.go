// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"errors"

	jc "github.com/juju/testing/checkers"
	jujutxn "github.com/juju/txn"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/testing"
)

type MultiEnvRunnerSuite struct {
	testing.BaseSuite
	multiEnvRunner jujutxn.Runner
	testRunner     *recordingRunner
}

var _ = gc.Suite(&MultiEnvRunnerSuite{})

// A fixed attempt counter value used to verify this is passed through
// in Run()
const (
	testTxnAttempt = 42
	envUUID        = "uuid"
)

func (s *MultiEnvRunnerSuite) SetUpTest(c *gc.C) {
	s.testRunner = &recordingRunner{}
	s.multiEnvRunner = NewMultiEnvRunnerForTesting(envUUID, s.testRunner)
}

// An alternative machine document to test that fields are matched by
// struct tag.
type altMachineDoc struct {
	Identifier  string `bson:"_id"`
	Environment string `bson:"env-uuid"`
}

type multiEnvRunnerTestCase struct {
	label         string
	input         txn.Op
	expected      txn.Op
	needsAliveEnv bool
}

// Test cases are returned by a function because transaction
// operations are modified in place and can't be safely reused by
// multiple tests.
func getTestCases() []multiEnvRunnerTestCase {
	return []multiEnvRunnerTestCase{
		{
			"ops for non-multi env collections are left alone",
			txn.Op{
				C:      "other",
				Id:     "whatever",
				Insert: bson.M{"_id": "whatever"},
			},
			txn.Op{
				C:      "other",
				Id:     "whatever",
				Insert: bson.M{"_id": "whatever"},
			},
			false,
		}, {
			"env UUID added to doc",
			txn.Op{
				C:  machinesC,
				Id: "0",
				Insert: &machineDoc{
					DocID: "0",
				},
			},
			txn.Op{
				C:  machinesC,
				Id: "uuid:0",
				Insert: &machineDoc{
					DocID:   "uuid:0",
					EnvUUID: "uuid",
				},
			},
			true,
		}, {
			"_id added to doc if missing",
			txn.Op{
				C:      machinesC,
				Id:     "1",
				Insert: &machineDoc{},
			},
			txn.Op{
				C:  machinesC,
				Id: "uuid:1",
				Insert: &machineDoc{
					DocID:   "uuid:1",
					EnvUUID: "uuid",
				},
			},
			true,
		}, {
			"fields matched by struct tag, not field name",
			txn.Op{
				C:  machinesC,
				Id: "2",
				Insert: &altMachineDoc{
					Identifier:  "2",
					Environment: "",
				},
			},
			txn.Op{
				C:  machinesC,
				Id: "uuid:2",
				Insert: &altMachineDoc{
					Identifier:  "uuid:2",
					Environment: "uuid",
				},
			},
			true,
		}, {
			"doc passed as struct value", // ok as long as no change to struct required
			txn.Op{
				C:  machinesC,
				Id: "3",
				// Passed by value
				Insert: machineDoc{
					DocID:   "uuid:3",
					EnvUUID: "uuid",
				},
			},
			txn.Op{
				C:  machinesC,
				Id: "uuid:3",
				Insert: machineDoc{
					DocID:   "uuid:3",
					EnvUUID: "uuid",
				},
			},
			true,
		}, {
			"document passed as bson.D",
			txn.Op{
				C:      machinesC,
				Id:     "4",
				Insert: bson.D{},
			},
			txn.Op{
				C:  machinesC,
				Id: "uuid:4",
				Insert: bson.D{
					{"_id", "uuid:4"},
					{"env-uuid", "uuid"},
				},
			},
			true,
		}, {
			"document passed as bson.M",
			txn.Op{
				C:      machinesC,
				Id:     "5",
				Insert: bson.M{},
			},
			txn.Op{
				C:  machinesC,
				Id: "uuid:5",
				Insert: bson.M{
					"_id":      "uuid:5",
					"env-uuid": "uuid",
				},
			},
			true,
		},
	}
}

func (s *MultiEnvRunnerSuite) TestRunTransaction(c *gc.C) {
	for i, t := range getTestCases() {
		c.Logf("TestRunTransaction %d: %s", i, t.label)

		inOps := []txn.Op{t.input}
		err := s.multiEnvRunner.RunTransaction(inOps)
		c.Assert(err, jc.ErrorIsNil)

		expected := []txn.Op{t.expected}

		// Input should have been modified in-place.
		c.Check(inOps, gc.DeepEquals, expected)

		if t.needsAliveEnv {
			expected = append(expected, assertEnvAliveOp(envUUID))
		}

		// Check ops seen by underlying runner.
		c.Check(s.testRunner.seenOps, gc.DeepEquals, expected)
	}
}

func (s *MultiEnvRunnerSuite) TestMultipleOps(c *gc.C) {
	var inOps []txn.Op
	var expectedOps []txn.Op
	var needsAliveEnv bool
	for _, t := range getTestCases() {
		inOps = append(inOps, t.input)
		expectedOps = append(expectedOps, t.expected)
		if !needsAliveEnv && t.needsAliveEnv {
			needsAliveEnv = true
		}
	}

	err := s.multiEnvRunner.RunTransaction(inOps)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(inOps, gc.DeepEquals, expectedOps)

	if needsAliveEnv {
		expectedOps = append(expectedOps, assertEnvAliveOp(envUUID))
	}
	c.Assert(s.testRunner.seenOps, gc.DeepEquals, expectedOps)
}

func (s *MultiEnvRunnerSuite) TestWithObjectIds(c *gc.C) {
	id := bson.NewObjectId()
	inOps := []txn.Op{{
		C:      networkInterfacesC,
		Id:     id,
		Insert: &networkInterfaceDoc{},
	}}

	err := s.multiEnvRunner.RunTransaction(inOps)
	c.Assert(err, jc.ErrorIsNil)

	expectedOps := []txn.Op{{
		C:  networkInterfacesC,
		Id: id,
		Insert: &networkInterfaceDoc{
			Id:      id,
			EnvUUID: envUUID,
		},
	}}
	c.Assert(inOps, gc.DeepEquals, expectedOps)

	updatedOps := []txn.Op{{
		C:  networkInterfacesC,
		Id: id,
		Insert: &networkInterfaceDoc{
			Id:      id,
			EnvUUID: envUUID,
		},
	},
		assertEnvAliveOp(envUUID),
	}

	c.Assert(s.testRunner.seenOps, gc.DeepEquals, updatedOps)
}

func (s *MultiEnvRunnerSuite) TestPanicWhenStructIsPassedByValueAndNeedsChange(c *gc.C) {
	// When a document struct is passed by reference it can't be
	// changed in place. It's important that the caller sees any
	// modifications made by multiEnvRunner in case the document
	// struct is used to create an object after insertion into
	// MongoDB.
	attempt := func() {
		s.multiEnvRunner.RunTransaction([]txn.Op{{
			C:      machinesC,
			Id:     "uuid:0",
			Insert: machineDoc{},
		}})
	}
	c.Assert(attempt, gc.PanicMatches,
		"struct for insert into machines requires DocID change but was passed by value")
}

func (s *MultiEnvRunnerSuite) TestPanicWhenStructIsMissingEnvUUIDField(c *gc.C) {
	type someDoc struct {
		DocID string `bson:"_id"`
	}
	attempt := func() {
		s.multiEnvRunner.RunTransaction([]txn.Op{{
			C:      machinesC,
			Id:     "uuid:0",
			Insert: &someDoc{DocID: "uuid:0"},
		}})
	}
	c.Assert(attempt, gc.PanicMatches,
		"struct for insert into machines is missing an env-uuid field")
}

func (s *MultiEnvRunnerSuite) TestPanicWhenStructEnvUUIDMismatch(c *gc.C) {
	attempt := func() {
		s.multiEnvRunner.RunTransaction([]txn.Op{{
			C:  machinesC,
			Id: "uuid:0",
			Insert: &machineDoc{
				DocID:   "uuid:0",
				EnvUUID: "somethingelse",
			},
		}})
	}
	c.Assert(attempt, gc.PanicMatches,
		"EnvUUID for insert into machines does not match expected value")
}

func (s *MultiEnvRunnerSuite) TestPanicWhenBsonDEnvUUIDMismatch(c *gc.C) {
	attempt := func() {
		s.multiEnvRunner.RunTransaction([]txn.Op{{
			C:      machinesC,
			Id:     "uuid:0",
			Insert: bson.D{{"env-uuid", "wtf"}},
		}})
	}
	c.Assert(attempt, gc.PanicMatches,
		"environment UUID for document to insert into machines does not match state")
}

func (s *MultiEnvRunnerSuite) TestPanicWhenBsonMEnvUUIDMismatch(c *gc.C) {
	attempt := func() {
		s.multiEnvRunner.RunTransaction([]txn.Op{{
			C:      machinesC,
			Id:     "uuid:0",
			Insert: bson.M{"env-uuid": "wtf"},
		}})
	}
	c.Assert(attempt, gc.PanicMatches,
		"environment UUID for document to insert into machines does not match state")
}

func (s *MultiEnvRunnerSuite) TestRun(c *gc.C) {
	for i, t := range getTestCases() {
		c.Logf("TestRun %d: %s", i, t.label)

		var seenAttempt int
		err := s.multiEnvRunner.Run(func(attempt int) ([]txn.Op, error) {
			seenAttempt = attempt
			return []txn.Op{t.input}, nil
		})
		c.Assert(err, jc.ErrorIsNil)

		expected := []txn.Op{t.expected}
		if t.needsAliveEnv {
			expected = append(expected, assertEnvAliveOp(envUUID))
		}

		c.Check(seenAttempt, gc.Equals, testTxnAttempt)
		c.Check(s.testRunner.seenOps, gc.DeepEquals, expected)
	}
}

func (s *MultiEnvRunnerSuite) TestRunWithError(c *gc.C) {
	err := s.multiEnvRunner.Run(func(attempt int) ([]txn.Op, error) {
		return nil, errors.New("boom")
	})
	c.Assert(err, gc.ErrorMatches, "boom")
	c.Assert(s.testRunner.seenOps, gc.IsNil)
}

func (s *MultiEnvRunnerSuite) TestResumeTransactions(c *gc.C) {
	err := s.multiEnvRunner.ResumeTransactions()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.testRunner.resumeTransactionsCalled, jc.IsTrue)
}

func (s *MultiEnvRunnerSuite) TestResumeTransactionsWithError(c *gc.C) {
	s.testRunner.resumeTransactionsErr = errors.New("boom")
	err := s.multiEnvRunner.ResumeTransactions()
	c.Assert(err, gc.ErrorMatches, "boom")
}

func (s *MultiEnvRunnerSuite) TestMaybePruneTransactions(c *gc.C) {
	err := s.multiEnvRunner.MaybePruneTransactions(2.0)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.testRunner.pruneTransactionsCalled, jc.IsTrue)
}

func (s *MultiEnvRunnerSuite) TestMaybePruneTransactionsWithError(c *gc.C) {
	s.testRunner.pruneTransactionsErr = errors.New("boom")
	err := s.multiEnvRunner.MaybePruneTransactions(2.0)
	c.Assert(err, gc.ErrorMatches, "boom")
}

// recordingRunner is fake transaction running that implements the
// jujutxn.Runner interface. Instead of doing anything with a database
// it simply records the transaction operations passed to it for later
// inspection.
//
// Note that a recordingRunner is only good for a single test because
// seenOps is overwritten for each call to RunTransaction and Run. A
// fresh instance should be created for each test.
type recordingRunner struct {
	seenOps                  []txn.Op
	resumeTransactionsCalled bool
	resumeTransactionsErr    error
	pruneTransactionsCalled  bool
	pruneTransactionsErr     error
}

func (r *recordingRunner) RunTransaction(ops []txn.Op) error {
	r.seenOps = ops
	return nil
}

func (r *recordingRunner) Run(transactions jujutxn.TransactionSource) (err error) {
	r.seenOps, err = transactions(testTxnAttempt)
	return
}

func (r *recordingRunner) ResumeTransactions() error {
	r.resumeTransactionsCalled = true
	return r.resumeTransactionsErr
}

func (r *recordingRunner) MaybePruneTransactions(float32) error {
	r.pruneTransactionsCalled = true
	return r.pruneTransactionsErr
}
