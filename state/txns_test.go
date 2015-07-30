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
	s.BaseSuite.SetUpTest(c)
	s.testRunner = &recordingRunner{}
	s.multiEnvRunner = &multiEnvRunner{
		rawRunner: s.testRunner,
		envUUID:   envUUID,
		schema: collectionSchema{
			machinesC:          {},
			networkInterfacesC: {},
			environmentsC:      {global: true},
			"other":            {global: true},
			"raw":              {rawAccess: true},
		},
	}
}

// An alternative machine document to test that fields are matched by
// struct tag.
type altMachineDoc struct {
	Identifier  string `bson:"_id"`
	Environment string `bson:"env-uuid"`
}

type multiEnvRunnerTestCase struct {
	label    string
	input    txn.Op
	expected txn.Op
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
		}, {
			"document passed as map[string]interface{}",
			txn.Op{
				C:      machinesC,
				Id:     "5",
				Insert: map[string]interface{}{},
			},
			txn.Op{
				C:  machinesC,
				Id: "uuid:5",
				Insert: map[string]interface{}{
					"_id":      "uuid:5",
					"env-uuid": "uuid",
				},
			},
		}, {
			"bson.D $set with struct update",
			txn.Op{
				C:      machinesC,
				Id:     "1",
				Update: bson.D{{"$set", &machineDoc{}}},
			},
			txn.Op{
				C:  machinesC,
				Id: "uuid:1",
				Update: bson.D{{
					"$set",
					&machineDoc{
						DocID:   "uuid:1",
						EnvUUID: "uuid",
					},
				}},
			},
		}, {
			"bson.D $set with bson.D update",
			txn.Op{
				C:      machinesC,
				Id:     "1",
				Update: bson.D{{"$set", bson.D{{"foo", "bar"}}}},
			},
			txn.Op{
				C:  machinesC,
				Id: "uuid:1",
				// Only structs get touched for $set updates.
				Update: bson.D{{
					"$set",
					bson.D{
						{"foo", "bar"},
					},
				}},
			},
		}, {
			"bson.M $set",
			txn.Op{
				C:      machinesC,
				Id:     "1",
				Update: bson.M{"$set": &machineDoc{}},
			},
			txn.Op{
				C:  machinesC,
				Id: "uuid:1",
				Update: bson.M{
					"$set": &machineDoc{
						DocID:   "uuid:1",
						EnvUUID: "uuid",
					},
				},
			},
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

		// Check ops seen by underlying runner.
		c.Check(s.testRunner.seenOps, gc.DeepEquals, expected)
	}
}

func (s *MultiEnvRunnerSuite) TestMultipleOps(c *gc.C) {
	var inOps []txn.Op
	var expectedOps []txn.Op
	for _, t := range getTestCases() {
		inOps = append(inOps, t.input)
		expectedOps = append(expectedOps, t.expected)
	}

	err := s.multiEnvRunner.RunTransaction(inOps)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(inOps, gc.DeepEquals, expectedOps)
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
	}}

	c.Assert(s.testRunner.seenOps, gc.DeepEquals, updatedOps)
}

func (s *MultiEnvRunnerSuite) TestRejectsAttemptToChangeEnvUUID(c *gc.C) {
	ops := []txn.Op{{
		C:      machinesC,
		Id:     "1",
		Insert: &machineDoc{},
	}}
	err := s.multiEnvRunner.RunTransaction(ops)
	c.Assert(err, jc.ErrorIsNil)

	ops = []txn.Op{{
		C:  machinesC,
		Id: "1",
		Insert: &machineDoc{
			EnvUUID: "wrong",
		},
	}}
	err = s.multiEnvRunner.RunTransaction(ops)
	c.Assert(err, gc.ErrorMatches, `cannot insert into "machines": bad "EnvUUID" field value.+`)
}

func (s *MultiEnvRunnerSuite) TestDoesNotAssertReferencedEnv(c *gc.C) {
	err := s.multiEnvRunner.RunTransaction([]txn.Op{{
		C:      environmentsC,
		Id:     envUUID,
		Insert: bson.M{},
	}})
	c.Check(err, jc.ErrorIsNil)
	c.Check(s.testRunner.seenOps, jc.DeepEquals, []txn.Op{{
		C:      environmentsC,
		Id:     envUUID,
		Insert: bson.M{},
	}})
}

func (s *MultiEnvRunnerSuite) TestRejectRawAccessCollection(c *gc.C) {
	err := s.multiEnvRunner.RunTransaction([]txn.Op{{
		C:      "raw",
		Id:     "whatever",
		Assert: bson.D{{"any", "thing"}},
	}})
	c.Check(err, gc.ErrorMatches, `forbidden transaction: references raw-access collection "raw"`)
	c.Check(s.testRunner.seenOps, gc.IsNil)
}

func (s *MultiEnvRunnerSuite) TestRejectUnknownCollection(c *gc.C) {
	err := s.multiEnvRunner.RunTransaction([]txn.Op{{
		C:      "unknown",
		Id:     "whatever",
		Assert: bson.D{{"any", "thing"}},
	}})
	c.Check(err, gc.ErrorMatches, `forbidden transaction: references unknown collection "unknown"`)
	c.Check(s.testRunner.seenOps, gc.IsNil)
}

func (s *MultiEnvRunnerSuite) TestRejectStructPassedByValueAndNeedsChange(c *gc.C) {
	// When a document struct is passed by reference it can't be
	// changed in place, it's important that the caller sees any
	// modifications made by multiEnvRunner in case the document
	// struct is used to create an object after insertion into
	// MongoDB (but see comments in multiEnvRunner.updateOps).
	err := s.multiEnvRunner.RunTransaction([]txn.Op{{
		C:      machinesC,
		Id:     "uuid:0",
		Insert: machineDoc{},
	}})
	c.Check(err, gc.ErrorMatches,
		`cannot insert into "machines": cannot set "DocID" field: struct passed by value`)
	c.Check(s.testRunner.seenOps, gc.IsNil)
}

func (s *MultiEnvRunnerSuite) TestRejectStructMissingEnvUUIDField(c *gc.C) {
	type someDoc struct {
		DocID string `bson:"_id"`
	}
	err := s.multiEnvRunner.RunTransaction([]txn.Op{{
		C:      machinesC,
		Id:     "uuid:0",
		Insert: &someDoc{DocID: "uuid:0"},
	}})
	c.Check(err, gc.ErrorMatches,
		`cannot insert into "machines": struct lacks field with bson:"env-uuid" tag`)
	c.Check(s.testRunner.seenOps, gc.IsNil)
}

func (s *MultiEnvRunnerSuite) TestRejectStructEnvUUIDMismatch(c *gc.C) {
	err := s.multiEnvRunner.RunTransaction([]txn.Op{{
		C:  machinesC,
		Id: "uuid:0",
		Insert: &machineDoc{
			DocID:   "uuid:0",
			EnvUUID: "somethingelse",
		},
	}})
	c.Check(err, gc.ErrorMatches,
		`cannot insert into "machines": bad "EnvUUID" field value: expected uuid, got somethingelse`)
	c.Check(s.testRunner.seenOps, gc.IsNil)
}

func (s *MultiEnvRunnerSuite) TestRejectBsonDEnvUUIDMismatch(c *gc.C) {
	err := s.multiEnvRunner.RunTransaction([]txn.Op{{
		C:      machinesC,
		Id:     "uuid:0",
		Insert: bson.D{{"env-uuid", "wtf"}},
	}})
	c.Check(err, gc.ErrorMatches,
		`cannot insert into "machines": bad "env-uuid" value: expected uuid, got wtf`)
	c.Check(s.testRunner.seenOps, gc.IsNil)
}

func (s *MultiEnvRunnerSuite) TestRejectBsonMEnvUUIDMismatch(c *gc.C) {
	err := s.multiEnvRunner.RunTransaction([]txn.Op{{
		C:      machinesC,
		Id:     "uuid:0",
		Insert: bson.M{"env-uuid": "wtf"},
	}})
	c.Check(err, gc.ErrorMatches,
		`cannot insert into "machines": bad "env-uuid" value: expected uuid, got wtf`)
	c.Check(s.testRunner.seenOps, gc.IsNil)
}

func (s *MultiEnvRunnerSuite) TestRejectUnsupportedDocType(c *gc.C) {
	err := s.multiEnvRunner.RunTransaction([]txn.Op{{
		C:      machinesC,
		Id:     "uuid:0",
		Insert: make(map[int]int),
	}})
	c.Check(err, gc.ErrorMatches, `cannot insert into "machines": unknown type map\[int\]int`)
	c.Check(s.testRunner.seenOps, gc.IsNil)
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

		c.Check(seenAttempt, gc.Equals, testTxnAttempt)
		c.Check(s.testRunner.seenOps, gc.DeepEquals, []txn.Op{t.expected})
	}
}

func (s *MultiEnvRunnerSuite) TestRunWithError(c *gc.C) {
	err := s.multiEnvRunner.Run(func(attempt int) ([]txn.Op, error) {
		return nil, errors.New("boom")
	})
	c.Check(err, gc.ErrorMatches, "boom")
	c.Check(s.testRunner.seenOps, gc.IsNil)
}

func (s *MultiEnvRunnerSuite) TestResumeTransactions(c *gc.C) {
	err := s.multiEnvRunner.ResumeTransactions()
	c.Check(err, jc.ErrorIsNil)
	c.Check(s.testRunner.resumeTransactionsCalled, jc.IsTrue)
}

func (s *MultiEnvRunnerSuite) TestResumeTransactionsWithError(c *gc.C) {
	s.testRunner.resumeTransactionsErr = errors.New("boom")
	err := s.multiEnvRunner.ResumeTransactions()
	c.Check(err, gc.ErrorMatches, "boom")
}

func (s *MultiEnvRunnerSuite) TestMaybePruneTransactions(c *gc.C) {
	err := s.multiEnvRunner.MaybePruneTransactions(2.0)
	c.Check(err, jc.ErrorIsNil)
	c.Check(s.testRunner.pruneTransactionsCalled, jc.IsTrue)
}

func (s *MultiEnvRunnerSuite) TestMaybePruneTransactionsWithError(c *gc.C) {
	s.testRunner.pruneTransactionsErr = errors.New("boom")
	err := s.multiEnvRunner.MaybePruneTransactions(2.0)
	c.Check(err, gc.ErrorMatches, "boom")
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
