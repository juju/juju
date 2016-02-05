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

type MultiModelRunnerSuite struct {
	testing.BaseSuite
	multiModelRunner jujutxn.Runner
	testRunner       *recordingRunner
}

var _ = gc.Suite(&MultiModelRunnerSuite{})

// A fixed attempt counter value used to verify this is passed through
// in Run()
const (
	testTxnAttempt = 42
	modelUUID      = "uuid"
)

func (s *MultiModelRunnerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.testRunner = &recordingRunner{}
	s.multiModelRunner = &multiModelRunner{
		rawRunner: s.testRunner,
		modelUUID: modelUUID,
		schema: collectionSchema{
			machinesC:          {},
			networkInterfacesC: {},
			modelsC:            {global: true},
			"other":            {global: true},
			"raw":              {rawAccess: true},
		},
	}
}

type testDoc struct {
	DocID     string `bson:"_id"`
	Id        string `bson:"thingid"`
	ModelUUID string `bson:"model-uuid"`
}

// An alternative machine document to test that fields are matched by
// struct tag.
type altTestDoc struct {
	Identifier string `bson:"_id"`
	Model      string `bson:"model-uuid"`
}

type multiModelRunnerTestCase struct {
	label    string
	input    txn.Op
	expected txn.Op
}

// Test cases are returned by a function because transaction
// operations are modified in place and can't be safely reused by
// multiple tests.
func getTestCases() []multiModelRunnerTestCase {
	return []multiModelRunnerTestCase{
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
				Insert: &testDoc{
					DocID: "0",
					Id:    "0",
				},
			},
			txn.Op{
				C:  machinesC,
				Id: "uuid:0",
				Insert: bson.D{
					{"_id", "uuid:0"},
					{"thingid", "0"},
					{"model-uuid", "uuid"},
				},
			},
		}, {
			"fields matched by struct tag, not field name",
			txn.Op{
				C:  machinesC,
				Id: "2",
				Insert: &altTestDoc{
					Identifier: "2",
					Model:      "",
				},
			},
			txn.Op{
				C:  machinesC,
				Id: "uuid:2",
				Insert: bson.D{
					{"_id", "uuid:2"},
					{"model-uuid", "uuid"},
				},
			},
		}, {
			"doc passed as struct value",
			txn.Op{
				C:  machinesC,
				Id: "3",
				// Passed by value
				Insert: testDoc{
					DocID: "3",
					Id:    "3",
				},
			},
			txn.Op{
				C:  machinesC,
				Id: "uuid:3",
				Insert: bson.D{
					{"_id", "uuid:3"},
					{"thingid", "3"},
					{"model-uuid", "uuid"},
				},
			},
		}, {
			"document passed as bson.D",
			txn.Op{
				C:      machinesC,
				Id:     "4",
				Insert: bson.D{{"_id", "4"}},
			},
			txn.Op{
				C:  machinesC,
				Id: "uuid:4",
				Insert: bson.D{
					{"_id", "uuid:4"},
					{"model-uuid", "uuid"},
				},
			},
		}, {
			"document passed as bson.M",
			txn.Op{
				C:      machinesC,
				Id:     "5",
				Insert: bson.M{"_id": "5"},
			},
			txn.Op{
				C:  machinesC,
				Id: "uuid:5",
				Insert: bson.D{
					{"_id", "uuid:5"},
					{"model-uuid", "uuid"},
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
				Insert: bson.D{
					{"model-uuid", "uuid"},
				},
			},
		}, {
			"bson.D $set with struct update",
			txn.Op{
				C:  machinesC,
				Id: "1",
				Update: bson.D{{"$set", &testDoc{
					DocID: "1",
					Id:    "1",
				}}},
			},
			txn.Op{
				C:  machinesC,
				Id: "uuid:1",
				Update: bson.D{{"$set",
					bson.D{
						{"_id", "uuid:1"},
						{"thingid", "1"},
						{"model-uuid", "uuid"},
					},
				}},
			},
		}, {
			"bson.D $set with bson.D update",
			txn.Op{
				C:  machinesC,
				Id: "1",
				Update: bson.D{
					{"$set", bson.D{
						{"_id", "1"},
						{"foo", "bar"},
					}},
					{"$other", "op"},
				},
			},
			txn.Op{
				C:  machinesC,
				Id: "uuid:1",
				Update: bson.D{
					{"$set", bson.D{
						{"_id", "uuid:1"},
						{"foo", "bar"},
					}},
					{"$other", "op"},
				},
			},
		}, {
			"bson.M $set",
			txn.Op{
				C:  machinesC,
				Id: "1",
				Update: bson.M{
					"$set": bson.M{"_id": "1"},
					"$foo": "bar",
				},
			},
			txn.Op{
				C:  machinesC,
				Id: "uuid:1",
				Update: bson.M{
					"$set": bson.D{{"_id", "uuid:1"}},
					"$foo": "bar",
				},
			},
		},
	}
}

func (s *MultiModelRunnerSuite) TestRunTransaction(c *gc.C) {
	for i, t := range getTestCases() {
		c.Logf("TestRunTransaction %d: %s", i, t.label)

		inOps := []txn.Op{t.input}
		err := s.multiModelRunner.RunTransaction(inOps)
		c.Assert(err, jc.ErrorIsNil)

		expected := []txn.Op{t.expected}

		// Check ops seen by underlying runner.
		c.Check(s.testRunner.seenOps, gc.DeepEquals, expected)
	}
}

func (s *MultiModelRunnerSuite) TestMultipleOps(c *gc.C) {
	var inOps []txn.Op
	var expectedOps []txn.Op
	for _, t := range getTestCases() {
		inOps = append(inOps, t.input)
		expectedOps = append(expectedOps, t.expected)
	}

	err := s.multiModelRunner.RunTransaction(inOps)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(s.testRunner.seenOps, gc.DeepEquals, expectedOps)
}

type objIdDoc struct {
	Id        bson.ObjectId `bson:"_id"`
	ModelUUID string        `bson:"model-uuid"`
}

func (s *MultiModelRunnerSuite) TestWithObjectIds(c *gc.C) {
	id := bson.NewObjectId()
	inOps := []txn.Op{{
		C:      networkInterfacesC,
		Id:     id,
		Insert: &objIdDoc{Id: id},
	}}

	err := s.multiModelRunner.RunTransaction(inOps)
	c.Assert(err, jc.ErrorIsNil)

	expectedOps := []txn.Op{{
		C:  networkInterfacesC,
		Id: id,
		Insert: bson.D{
			{"_id", id},
			{"model-uuid", "uuid"},
		},
	}}
	c.Assert(s.testRunner.seenOps, gc.DeepEquals, expectedOps)
}

func (s *MultiModelRunnerSuite) TestRejectsAttemptToInsertWrongModelUUID(c *gc.C) {
	ops := []txn.Op{{
		C:      machinesC,
		Id:     "1",
		Insert: &machineDoc{},
	}}
	err := s.multiModelRunner.RunTransaction(ops)
	c.Assert(err, jc.ErrorIsNil)

	ops = []txn.Op{{
		C:  machinesC,
		Id: "1",
		Insert: &machineDoc{
			ModelUUID: "wrong",
		},
	}}
	err = s.multiModelRunner.RunTransaction(ops)
	c.Assert(err, gc.ErrorMatches, `cannot insert into "machines": bad "model-uuid" value.+`)
}

func (s *MultiModelRunnerSuite) TestRejectsAttemptToChangeModelUUID(c *gc.C) {
	// Setting to same env UUID is ok.
	ops := []txn.Op{{
		C:      machinesC,
		Id:     "1",
		Update: bson.M{"$set": &machineDoc{ModelUUID: modelUUID}},
	}}
	err := s.multiModelRunner.RunTransaction(ops)
	c.Assert(err, jc.ErrorIsNil)

	// Using the wrong env UUID isn't allowed.
	ops = []txn.Op{{
		C:      machinesC,
		Id:     "1",
		Update: bson.M{"$set": &machineDoc{ModelUUID: "wrong"}},
	}}
	err = s.multiModelRunner.RunTransaction(ops)
	c.Assert(err, gc.ErrorMatches, `cannot update "machines": bad "model-uuid" value.+`)
}

func (s *MultiModelRunnerSuite) TestDoesNotAssertReferencedEnv(c *gc.C) {
	err := s.multiModelRunner.RunTransaction([]txn.Op{{
		C:      modelsC,
		Id:     modelUUID,
		Insert: bson.M{},
	}})
	c.Check(err, jc.ErrorIsNil)
	c.Check(s.testRunner.seenOps, jc.DeepEquals, []txn.Op{{
		C:      modelsC,
		Id:     modelUUID,
		Insert: bson.M{},
	}})
}

func (s *MultiModelRunnerSuite) TestRejectRawAccessCollection(c *gc.C) {
	err := s.multiModelRunner.RunTransaction([]txn.Op{{
		C:      "raw",
		Id:     "whatever",
		Assert: bson.D{{"any", "thing"}},
	}})
	c.Check(err, gc.ErrorMatches, `forbidden transaction: references raw-access collection "raw"`)
	c.Check(s.testRunner.seenOps, gc.IsNil)
}

func (s *MultiModelRunnerSuite) TestRejectUnknownCollection(c *gc.C) {
	err := s.multiModelRunner.RunTransaction([]txn.Op{{
		C:      "unknown",
		Id:     "whatever",
		Assert: bson.D{{"any", "thing"}},
	}})
	c.Check(err, gc.ErrorMatches, `forbidden transaction: references unknown collection "unknown"`)
	c.Check(s.testRunner.seenOps, gc.IsNil)
}

func (s *MultiModelRunnerSuite) TestRejectStructModelUUIDMismatch(c *gc.C) {
	err := s.multiModelRunner.RunTransaction([]txn.Op{{
		C:  machinesC,
		Id: "uuid:0",
		Insert: &machineDoc{
			DocID:     "uuid:0",
			ModelUUID: "somethingelse",
		},
	}})
	c.Check(err, gc.ErrorMatches,
		`cannot insert into "machines": bad "model-uuid" value: expected uuid, got somethingelse`)
	c.Check(s.testRunner.seenOps, gc.IsNil)
}

func (s *MultiModelRunnerSuite) TestRejectBsonDModelUUIDMismatch(c *gc.C) {
	err := s.multiModelRunner.RunTransaction([]txn.Op{{
		C:      machinesC,
		Id:     "uuid:0",
		Insert: bson.D{{"model-uuid", "wtf"}},
	}})
	c.Check(err, gc.ErrorMatches,
		`cannot insert into "machines": bad "model-uuid" value: expected uuid, got wtf`)
	c.Check(s.testRunner.seenOps, gc.IsNil)
}

func (s *MultiModelRunnerSuite) TestRejectBsonMModelUUIDMismatch(c *gc.C) {
	err := s.multiModelRunner.RunTransaction([]txn.Op{{
		C:      machinesC,
		Id:     "uuid:0",
		Insert: bson.M{"model-uuid": "wtf"},
	}})
	c.Check(err, gc.ErrorMatches,
		`cannot insert into "machines": bad "model-uuid" value: expected uuid, got wtf`)
	c.Check(s.testRunner.seenOps, gc.IsNil)
}

func (s *MultiModelRunnerSuite) TestRun(c *gc.C) {
	for i, t := range getTestCases() {
		c.Logf("TestRun %d: %s", i, t.label)

		var seenAttempt int
		err := s.multiModelRunner.Run(func(attempt int) ([]txn.Op, error) {
			seenAttempt = attempt
			return []txn.Op{t.input}, nil
		})
		c.Assert(err, jc.ErrorIsNil)

		c.Check(seenAttempt, gc.Equals, testTxnAttempt)
		c.Check(s.testRunner.seenOps, gc.DeepEquals, []txn.Op{t.expected})
	}
}

func (s *MultiModelRunnerSuite) TestRunWithError(c *gc.C) {
	err := s.multiModelRunner.Run(func(attempt int) ([]txn.Op, error) {
		return nil, errors.New("boom")
	})
	c.Check(err, gc.ErrorMatches, "boom")
	c.Check(s.testRunner.seenOps, gc.IsNil)
}

func (s *MultiModelRunnerSuite) TestResumeTransactions(c *gc.C) {
	err := s.multiModelRunner.ResumeTransactions()
	c.Check(err, jc.ErrorIsNil)
	c.Check(s.testRunner.resumeTransactionsCalled, jc.IsTrue)
}

func (s *MultiModelRunnerSuite) TestResumeTransactionsWithError(c *gc.C) {
	s.testRunner.resumeTransactionsErr = errors.New("boom")
	err := s.multiModelRunner.ResumeTransactions()
	c.Check(err, gc.ErrorMatches, "boom")
}

func (s *MultiModelRunnerSuite) TestMaybePruneTransactions(c *gc.C) {
	err := s.multiModelRunner.MaybePruneTransactions(2.0)
	c.Check(err, jc.ErrorIsNil)
	c.Check(s.testRunner.pruneTransactionsCalled, jc.IsTrue)
}

func (s *MultiModelRunnerSuite) TestMaybePruneTransactionsWithError(c *gc.C) {
	s.testRunner.pruneTransactionsErr = errors.New("boom")
	err := s.multiModelRunner.MaybePruneTransactions(2.0)
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
