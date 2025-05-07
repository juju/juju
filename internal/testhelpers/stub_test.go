// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package testhelpers_test

import (
	"github.com/juju/errors"
	"github.com/juju/tc"

	testing "github.com/juju/juju/internal/testhelpers"
)

type stubA struct {
	*testing.Stub
}

func (f *stubA) aMethod(a, b, c int) error {
	f.MethodCall(f, "aMethod", a, b, c)
	return f.NextErr()
}

func (f *stubA) otherMethod(values ...string) error {
	f.MethodCall(f, "otherMethod", values)
	return f.NextErr()
}

type stubB struct {
	*testing.Stub
}

func (f *stubB) aMethod() error {
	f.MethodCall(f, "aMethod")
	return f.NextErr()
}

func (f *stubB) aFunc(value string) error {
	f.AddCall("aFunc", value)
	return f.NextErr()
}

type stubSuite struct {
	stub *testing.Stub
}

var _ = tc.Suite(&stubSuite{})

func (s *stubSuite) SetUpTest(c *tc.C) {
	s.stub = &testing.Stub{}
}

func (s *stubSuite) TestNextErrSequence(c *tc.C) {
	exp1 := errors.New("<failure 1>")
	exp2 := errors.New("<failure 2>")
	s.stub.SetErrors(exp1, exp2)

	err1 := s.stub.NextErr()
	err2 := s.stub.NextErr()

	c.Check(err1, tc.Equals, exp1)
	c.Check(err2, tc.Equals, exp2)
}

func (s *stubSuite) TestNextErrPops(c *tc.C) {
	exp1 := errors.New("<failure 1>")
	exp2 := errors.New("<failure 2>")
	s.stub.SetErrors(exp1, exp2)

	s.stub.NextErr()

	s.stub.CheckErrors(c, exp2)
}

func (s *stubSuite) TestNextErrEmptyNil(c *tc.C) {
	err1 := s.stub.NextErr()
	err2 := s.stub.NextErr()

	c.Check(err1, tc.ErrorIsNil)
	c.Check(err2, tc.ErrorIsNil)
}

func (s *stubSuite) TestNextErrSkip(c *tc.C) {
	expected := errors.New("<failure>")
	s.stub.SetErrors(nil, nil, expected)

	err1 := s.stub.NextErr()
	err2 := s.stub.NextErr()
	err3 := s.stub.NextErr()

	c.Check(err1, tc.ErrorIsNil)
	c.Check(err2, tc.ErrorIsNil)
	c.Check(err3, tc.Equals, expected)
}

func (s *stubSuite) TestNextErrEmbeddedMixed(c *tc.C) {
	exp1 := errors.New("<failure 1>")
	exp2 := errors.New("<failure 2>")
	s.stub.SetErrors(exp1, nil, nil, exp2)

	stub1 := &stubA{s.stub}
	stub2 := &stubB{s.stub}
	err1 := stub1.aMethod(1, 2, 3)
	err2 := stub2.aFunc("arg")
	err3 := stub1.otherMethod("arg1", "arg2")
	err4 := stub2.aMethod()

	c.Check(err1, tc.Equals, exp1)
	c.Check(err2, tc.ErrorIsNil)
	c.Check(err3, tc.ErrorIsNil)
	c.Check(err4, tc.Equals, exp2)
}

func (s *stubSuite) TestPopNoErrOkay(c *tc.C) {
	exp1 := errors.New("<failure 1>")
	exp2 := errors.New("<failure 2>")
	s.stub.SetErrors(exp1, nil, exp2)

	err1 := s.stub.NextErr()
	s.stub.PopNoErr()
	err2 := s.stub.NextErr()

	c.Check(err1, tc.Equals, exp1)
	c.Check(err2, tc.Equals, exp2)
}

func (s *stubSuite) TestPopNoErrEmpty(c *tc.C) {
	s.stub.PopNoErr()
	err := s.stub.NextErr()

	c.Check(err, tc.ErrorIsNil)
}

func (s *stubSuite) TestPopNoErrPanic(c *tc.C) {
	failure := errors.New("<failure>")
	s.stub.SetErrors(failure)

	f := func() {
		s.stub.PopNoErr()
	}
	c.Check(f, tc.PanicMatches, `expected a nil error, got .*`)
}

func (s *stubSuite) TestAddCallRecorded(c *tc.C) {
	s.stub.AddCall("aFunc", 1, 2, 3)

	c.Check(s.stub.Calls(), tc.DeepEquals, []testing.StubCall{{
		FuncName: "aFunc",
		Args:     []interface{}{1, 2, 3},
	}})
	s.stub.CheckReceivers(c, nil)
}

func (s *stubSuite) TestAddCallRepeated(c *tc.C) {
	s.stub.AddCall("before", "arg")
	s.stub.AddCall("aFunc", 1, 2, 3)
	s.stub.AddCall("aFunc", 4, 5, 6)
	s.stub.AddCall("after", "arg")

	c.Check(s.stub.Calls(), tc.DeepEquals, []testing.StubCall{{
		FuncName: "before",
		Args:     []interface{}{"arg"},
	}, {
		FuncName: "aFunc",
		Args:     []interface{}{1, 2, 3},
	}, {
		FuncName: "aFunc",
		Args:     []interface{}{4, 5, 6},
	}, {
		FuncName: "after",
		Args:     []interface{}{"arg"},
	}})
	s.stub.CheckReceivers(c, nil, nil, nil, nil)
}

func (s *stubSuite) TestAddCallNoArgs(c *tc.C) {
	s.stub.AddCall("aFunc")

	c.Check(s.stub.Calls(), tc.DeepEquals, []testing.StubCall{{
		FuncName: "aFunc",
	}})
}

func (s *stubSuite) TestResetCalls(c *tc.C) {
	s.stub.AddCall("aFunc")
	s.stub.CheckCalls(c, []testing.StubCall{{FuncName: "aFunc"}})

	s.stub.ResetCalls()
	s.stub.CheckCalls(c, nil)
}

func (s *stubSuite) TestAddCallSequence(c *tc.C) {
	s.stub.AddCall("first")
	s.stub.AddCall("second")
	s.stub.AddCall("third")

	c.Check(s.stub.Calls(), tc.DeepEquals, []testing.StubCall{{
		FuncName: "first",
	}, {
		FuncName: "second",
	}, {
		FuncName: "third",
	}})
}

func (s *stubSuite) TestMethodCallRecorded(c *tc.C) {
	s.stub.MethodCall(s.stub, "aMethod", 1, 2, 3)

	c.Check(s.stub.Calls(), tc.DeepEquals, []testing.StubCall{{
		FuncName: "aMethod",
		Args:     []interface{}{1, 2, 3},
	}})
	s.stub.CheckReceivers(c, s.stub)
}

func (s *stubSuite) TestMethodCallMixed(c *tc.C) {
	s.stub.MethodCall(s.stub, "Method1", 1, 2, 3)
	s.stub.AddCall("aFunc", "arg")
	s.stub.MethodCall(s.stub, "Method2")

	s.stub.CheckCalls(c, []testing.StubCall{{
		FuncName: "Method1",
		Args:     []interface{}{1, 2, 3},
	}, {
		FuncName: "aFunc",
		Args:     []interface{}{"arg"},
	}, {
		FuncName: "Method2",
	}})
	s.stub.CheckReceivers(c, s.stub, nil, s.stub)
}

func (s *stubSuite) TestMethodCallEmbeddedMixed(c *tc.C) {
	stub1 := &stubA{s.stub}
	stub2 := &stubB{s.stub}
	err := stub1.aMethod(1, 2, 3)
	c.Assert(err, tc.ErrorIsNil)
	err = stub2.aFunc("arg")
	c.Assert(err, tc.ErrorIsNil)
	err = stub1.otherMethod("arg1", "arg2")
	c.Assert(err, tc.ErrorIsNil)
	err = stub2.aMethod()
	c.Assert(err, tc.ErrorIsNil)

	c.Check(s.stub.Calls(), tc.DeepEquals, []testing.StubCall{{
		FuncName: "aMethod",
		Args:     []interface{}{1, 2, 3},
	}, {
		FuncName: "aFunc",
		Args:     []interface{}{"arg"},
	}, {
		FuncName: "otherMethod",
		Args:     []interface{}{[]string{"arg1", "arg2"}},
	}, {
		FuncName: "aMethod",
	}})
	s.stub.CheckReceivers(c, stub1, nil, stub1, stub2)
}

func (s *stubSuite) TestSetErrorsMultiple(c *tc.C) {
	err1 := errors.New("<failure 1>")
	err2 := errors.New("<failure 2>")
	s.stub.SetErrors(err1, err2)

	s.stub.CheckErrors(c, err1, err2)
}

func (s *stubSuite) TestSetErrorsEmpty(c *tc.C) {
	s.stub.SetErrors() // pass an empty varargs of errors

	s.stub.CheckErrors(c) // check that it is indeed empty
}

func (s *stubSuite) TestSetErrorMixed(c *tc.C) {
	err1 := errors.New("<failure 1>")
	err2 := errors.New("<failure 2>")
	s.stub.SetErrors(nil, err1, nil, err2)

	s.stub.CheckErrors(c, nil, err1, nil, err2)
}

func (s *stubSuite) TestSetErrorsTrailingNil(c *tc.C) {
	err := errors.New("<failure 1>")
	s.stub.SetErrors(err, nil)

	s.stub.CheckErrors(c, err, nil)
}

func (s *stubSuite) checkCallsStandard(c *tc.C) {
	s.stub.CheckCalls(c, []testing.StubCall{{
		FuncName: "first",
		Args:     []interface{}{"arg"},
	}, {
		FuncName: "second",
		Args:     []interface{}{1, 2, 3},
	}, {
		FuncName: "third",
	}})
}

func (s *stubSuite) TestCheckCallsPass(c *tc.C) {
	s.stub.AddCall("first", "arg")
	s.stub.AddCall("second", 1, 2, 3)
	s.stub.AddCall("third")

	s.checkCallsStandard(c)
}

func (s *stubSuite) TestCheckCallsEmpty(c *tc.C) {
	s.stub.CheckCalls(c, nil)
}

func (s *stubSuite) TestCheckCallsMissingCall(c *tc.C) {
	s.stub.AddCall("first", "arg")
	s.stub.AddCall("third")

	c.ExpectFailure(`the "standard" Stub.CheckCalls call should fail`)
	s.checkCallsStandard(c)
}

func (s *stubSuite) TestCheckCallsWrongName(c *tc.C) {
	s.stub.AddCall("first", "arg")
	s.stub.AddCall("oops", 1, 2, 3)
	s.stub.AddCall("third")

	c.ExpectFailure(`the "standard" Stub.CheckCalls call should fail`)
	s.checkCallsStandard(c)
}

func (s *stubSuite) TestCheckCallsWrongArgs(c *tc.C) {
	s.stub.AddCall("first", "arg")
	s.stub.AddCall("second", 1, 2, 4)
	s.stub.AddCall("third")

	c.ExpectFailure(`the "standard" Stub.CheckCalls call should fail`)
	s.checkCallsStandard(c)
}

func (s *stubSuite) checkCallStandard(c *tc.C) {
	s.stub.CheckCall(c, 0, "first", "arg")
	s.stub.CheckCall(c, 1, "second", 1, 2, 3)
	s.stub.CheckCall(c, 2, "third")
}

func (s *stubSuite) TestCheckCallPass(c *tc.C) {
	s.stub.AddCall("first", "arg")
	s.stub.AddCall("second", 1, 2, 3)
	s.stub.AddCall("third")

	s.checkCallStandard(c)
}

func (s *stubSuite) TestCheckCallEmpty(c *tc.C) {
	c.ExpectFailure(`Stub.CheckCall should fail when no calls have been made`)
	s.stub.CheckCall(c, 0, "aMethod")
}

func (s *stubSuite) TestCheckCallMissingCall(c *tc.C) {
	s.stub.AddCall("first", "arg")
	s.stub.AddCall("third")

	c.ExpectFailure(`the "standard" Stub.CheckCall call should fail here`)
	s.checkCallStandard(c)
}

func (s *stubSuite) TestCheckCallWrongName(c *tc.C) {
	s.stub.AddCall("first", "arg")
	s.stub.AddCall("oops", 1, 2, 3)
	s.stub.AddCall("third")

	c.ExpectFailure(`the "standard" Stub.CheckCall call should fail here`)
	s.checkCallStandard(c)
}

func (s *stubSuite) TestCheckCallWrongArgs(c *tc.C) {
	s.stub.AddCall("first", "arg")
	s.stub.AddCall("second", 1, 2, 4)
	s.stub.AddCall("third")

	c.ExpectFailure(`the "standard" Stub.CheckCall call should fail here`)
	s.checkCallStandard(c)
}

func (s *stubSuite) TestCheckCallNamesPass(c *tc.C) {
	s.stub.AddCall("first", "arg")
	s.stub.AddCall("second", 1, 2, 4)
	s.stub.AddCall("third")

	s.stub.CheckCallNames(c, "first", "second", "third")
}

func (s *stubSuite) TestCheckCallNamesUnexpected(c *tc.C) {
	s.stub.AddCall("first", "arg")
	s.stub.AddCall("second", 1, 2, 4)
	s.stub.AddCall("third")

	c.ExpectFailure(`Stub.CheckCall should fail when no calls have been made`)
	s.stub.CheckCallNames(c)
}

func (s *stubSuite) TestCheckCallNamesEmptyPass(c *tc.C) {
	s.stub.CheckCallNames(c)
}

func (s *stubSuite) TestCheckCallNamesEmptyFail(c *tc.C) {
	c.ExpectFailure(`Stub.CheckCall should fail when no calls have been made`)
	s.stub.CheckCallNames(c, "aMethod")
}

func (s *stubSuite) TestCheckCallNamesMissingCall(c *tc.C) {
	s.stub.AddCall("first", "arg")
	s.stub.AddCall("third")

	c.ExpectFailure(`the "standard" Stub.CheckCallNames call should fail here`)
	s.stub.CheckCallNames(c, "first", "second", "third")
}

func (s *stubSuite) TestCheckCallNamesWrongName(c *tc.C) {
	s.stub.AddCall("first", "arg")
	s.stub.AddCall("oops", 1, 2, 4)
	s.stub.AddCall("third")

	c.ExpectFailure(`the "standard" Stub.CheckCallNames call should fail here`)
	s.stub.CheckCallNames(c, "first", "second", "third")
}

func (s *stubSuite) TestCheckNoCalls(c *tc.C) {
	s.stub.CheckNoCalls(c)

	s.stub.AddCall("method", "arg")
	c.ExpectFailure(`the "standard" Stub.CheckNoCalls call should fail here`)
	s.stub.CheckNoCalls(c)
}

func (s *stubSuite) TestMethodCallsUnordered(c *tc.C) {
	s.stub.MethodCall(s.stub, "Method1", 1, 2, 3)
	s.stub.AddCall("aFunc", "arg")
	s.stub.MethodCall(s.stub, "Method2")

	s.stub.CheckCallsUnordered(c, []testing.StubCall{{
		FuncName: "aFunc",
		Args:     []interface{}{"arg"},
	}, {
		FuncName: "Method1",
		Args:     []interface{}{1, 2, 3},
	}, {
		FuncName: "Method2",
	}})
}

// This case checks that in the scenario when expected calls are
// [a,b,c,c] but the calls made are actually [a,b,b,c], we fail correctly.
func (s *stubSuite) TestMethodCallsUnorderedDuplicateFail(c *tc.C) {
	s.stub.MethodCall(s.stub, "Method1", 1, 2, 3)
	s.stub.MethodCall(s.stub, "Method1", 1, 2, 3)
	s.stub.AddCall("aFunc", "arg")
	s.stub.MethodCall(s.stub, "Method2")

	s.stub.CheckCallsUnordered(c, []testing.StubCall{{
		FuncName: "aFunc",
		Args:     []interface{}{"arg"},
	}, {
		FuncName: "Method1",
		Args:     []interface{}{1, 2, 3},
	}, {
		FuncName: "Method2",
	}, {
		FuncName: "Method2",
	}})
	c.ExpectFailure("should have failed as expected calls differ from calls made")
}
