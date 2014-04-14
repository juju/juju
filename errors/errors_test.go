// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors_test

import (
	stderrors "errors"
	"fmt"
	"reflect"
	"runtime"
	"testing"

	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/errors"
)

// allErrors holds information for all defined errors: a satisfier
// function, wrapping and variable arguments constructors and message
// suffix. When adding new errors, add them here as well to include
// them in tests.
var allErrors = []struct {
	satisfier       func(error) bool
	argsConstructor func(string, ...interface{}) error
	wrapConstructor func(error, string) error
	suffix          string
}{
	{errors.IsNotFound, errors.NotFoundf, errors.NewNotFound, " not found"},
	{errors.IsUnauthorized, errors.Unauthorizedf, errors.NewUnauthorized, ""},
	{errors.IsNotImplemented, errors.NotImplementedf, errors.NewNotImplemented, " not implemented"},
	{errors.IsAlreadyExists, errors.AlreadyExistsf, errors.NewAlreadyExists, " already exists"},
	{errors.IsNotSupported, errors.NotSupportedf, errors.NewNotSupported, " not supported"},
}

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

func contextf(err error, format string, args ...interface{}) error {
	errors.Contextf(&err, format, args...)
	return err
}

type errorTest struct {
	err       error
	message   string
	satisfier func(error) bool
}

func (*errorsSuite) TestErrors(c *gc.C) {
	errorTests := []errorTest{{
		contextf(stderrors.New("generic error"), "some%s", "what"),
		"somewhat: generic error",
		nil,
	}, {
		nil,
		"<nil>",
		nil,
	}, {
		contextf(nil, "ignored"),
		"<nil>",
		nil,
	}}
	for _, errorInfo := range allErrors {
		errorTests = append(errorTests, []errorTest{{
			errorInfo.argsConstructor(""),
			errorInfo.suffix,
			errorInfo.satisfier,
		}, {
			errorInfo.argsConstructor("pow"),
			"pow" + errorInfo.suffix,
			errorInfo.satisfier,
		}, {
			errorInfo.argsConstructor("woop %d", 123),
			"woop 123" + errorInfo.suffix,
			errorInfo.satisfier,
		}, {
			errorInfo.wrapConstructor(stderrors.New("woo"), "msg"),
			"msg: woo",
			errorInfo.satisfier,
		}, {
			errorInfo.wrapConstructor(stderrors.New("woo"), ""),
			"woo",
			errorInfo.satisfier,
		}, {
			errorInfo.wrapConstructor(fmt.Errorf("hey %s", "jude"), ""),
			"hey jude",
			errorInfo.satisfier,
		}, {
			errorInfo.wrapConstructor(nil, "msg"),
			"msg",
			errorInfo.satisfier,
		}, {
			errorInfo.wrapConstructor(nil, ""),
			"",
			errorInfo.satisfier,
		}, {
			contextf(errorInfo.argsConstructor("woo %q", "bar"), "msg %d", 42),
			`msg 42: woo "bar"` + errorInfo.suffix,
			errorInfo.satisfier,
		}, {
			contextf(errorInfo.argsConstructor("something"), "bad"),
			"bad: something" + errorInfo.suffix,
			errorInfo.satisfier,
		}, {
			contextf(errorInfo.argsConstructor("foo"), ""),
			"foo" + errorInfo.suffix,
			errorInfo.satisfier,
		}, {
			contextf(errorInfo.argsConstructor(""), ""),
			errorInfo.suffix,
			errorInfo.satisfier,
		}, {
			contextf(errorInfo.wrapConstructor(fmt.Errorf("foo %s", "baz"), "bad"), "bar %d", 42),
			"bar 42: bad: foo baz",
			errorInfo.satisfier,
		}, {
			contextf(errorInfo.wrapConstructor(stderrors.New("foo"), "bar"), "cause"),
			"cause: bar: foo",
			errorInfo.satisfier,
		}, {
			contextf(errorInfo.wrapConstructor(stderrors.New(""), "bar"), "cause"),
			"cause: bar: ",
			errorInfo.satisfier,
		}, {
			contextf(errorInfo.wrapConstructor(stderrors.New("woohoo"), ""), "cause"),
			"cause: woohoo",
			errorInfo.satisfier,
		}, {
			contextf(errorInfo.wrapConstructor(stderrors.New(""), ""), "cause"),
			"cause: ",
			errorInfo.satisfier,
		}, {
			contextf(errorInfo.wrapConstructor(nil, "woo"), "cause"),
			"cause: woo",
			errorInfo.satisfier,
		}, {
			contextf(errorInfo.wrapConstructor(nil, ""), ""),
			"",
			errorInfo.satisfier,
		}}...)
	}

	for i, t := range errorTests {
		if t.err == nil {
			c.Logf("test #%d: nil: %v", i, t.err)
			c.Check(t.message, gc.Equals, "<nil>")
			c.Check(t.satisfier, gc.IsNil)
			continue
		}
		c.Logf("test #%d: %T: %v", i, t.err, t.err)
		c.Check(t.err, gc.ErrorMatches, t.message)
		if t.satisfier != nil {
			c.Check(t.err, jc.Satisfies, t.satisfier)
		}
		// Make sure only t.satifier satisfies t.err.
		for _, errorInfo := range allErrors {
			// Not using jc.Satisfies here, because it doesn't give a
			// nice string representation of the function. Also, you
			// can't take the address of a func, but you can store it
			// and take the address of the struct.
			satisfier := errorInfo.satisfier
			errSatisfier := &errorSatisfier{satisfier}
			testSatisfier := &errorSatisfier{t.satisfier}
			if t.satisfier == nil && satisfier(t.err) {
				c.Errorf("%#v satisfies %v (but should not)", t.err, errSatisfier)
			} else if t.satisfier != nil {
				if !t.satisfier(t.err) {
					c.Errorf("%#v does not satisfy %v (but it should)", t.err, testSatisfier)
				}
				if t.satisfier(t.err) && satisfier(t.err) &&
					testSatisfier.String() != errSatisfier.String() {
					c.Errorf("%#v satisfies both %v (good) and %v (bad)", t.err, testSatisfier, errSatisfier)
				}
			}
		}
	}
}
