// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors_test

import (
	stderrors "errors"
	"reflect"
	"runtime"
	"testing"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/errors"
	jc "launchpad.net/juju-core/testing/checkers"
)

type errorsSuite struct{}

var _ = gc.Suite(&errorsSuite{})

func Test(t *testing.T) {
	gc.TestingT(t)
}

type errorSatisfier struct {
	f func(error) bool
}

func (s *errorSatisfier) String() string {
	value := reflect.ValueOf(s.f)
	f := runtime.FuncForPC(value.Pointer())
	return f.Name()
}

func (*errorsSuite) TestNotFoundError(c *gc.C) {
	isNotFoundError := &errorSatisfier{errors.IsNotFoundError}
	isUnauthorizedError := &errorSatisfier{errors.IsUnauthorizedError}
	isNotImplementedError := &errorSatisfier{errors.IsNotImplementedError}
	satisfiers := []*errorSatisfier{
		isNotFoundError,
		isUnauthorizedError,
		isNotImplementedError,
	}

	// make some errors, and record the errorSatsifier
	// that should satisfy the error.
	type errorTest struct {
		err       error
		message   string
		satisfier *errorSatisfier
	}
	errorTests := []errorTest{{
		errors.NotFoundf("woop %d", 123),
		"woop 123 not found",
		isNotFoundError,
	}, {
		errors.NewNotFoundError(stderrors.New("woo"), "msg"),
		"msg: woo",
		isNotFoundError,
	}, {
		errors.NewNotFoundError(stderrors.New("woo"), ""),
		"woo",
		isNotFoundError,
	}, {
		errors.NewNotFoundError(nil, "msg"),
		"msg",
		isNotFoundError,
	}, {
		errors.NewNotFoundError(nil, ""),
		"",
		isNotFoundError,
	}, {
		errors.Unauthorizedf("woo %s", "hoo"),
		"woo hoo",
		isUnauthorizedError,
	}, {
		errors.NewUnauthorizedError(stderrors.New("hoo"), "woo"),
		"woo: hoo",
		isUnauthorizedError,
	}, {
		errors.NewNotImplementedError("something"),
		"something not implemented",
		isNotImplementedError,
	}}

	for i, t := range errorTests {
		c.Logf("test #%d: %v", i, t.err)
		c.Assert(t.err, gc.ErrorMatches, t.message)
		c.Assert(t.err, jc.Satisfies, t.satisfier.f)
		for _, satisfier := range satisfiers {
			// Not using jc.Satisfier here, because it doesn't give
			// a nice string representation of the function. Also,
			// you can't take the address of a func, but you can
			// store it and take the address of the struct.
			if satisfier != t.satisfier && satisfier.f(t.err) {
				c.Errorf("%#v satisfies %v", t.err, satisfier)
			}
		}
	}
}
