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
	for _, errorInfo := range errors.AllErrors {
		errorTests = append(errorTests, []errorTest{{
			errorInfo.ArgsConstructor(""),
			errorInfo.Suffix,
			errorInfo.Satisfier,
		}, {
			errorInfo.ArgsConstructor("pow"),
			"pow" + errorInfo.Suffix,
			errorInfo.Satisfier,
		}, {
			errorInfo.ArgsConstructor("woop %d", 123),
			"woop 123" + errorInfo.Suffix,
			errorInfo.Satisfier,
		}, {
			errorInfo.WrapConstructor(stderrors.New("woo"), "msg"),
			"msg: woo",
			errorInfo.Satisfier,
		}, {
			errorInfo.WrapConstructor(stderrors.New("woo"), ""),
			"woo",
			errorInfo.Satisfier,
		}, {
			errorInfo.WrapConstructor(fmt.Errorf("hey %s", "jude"), ""),
			"hey jude",
			errorInfo.Satisfier,
		}, {
			errorInfo.WrapConstructor(nil, "msg"),
			"msg",
			errorInfo.Satisfier,
		}, {
			errorInfo.WrapConstructor(nil, ""),
			"",
			errorInfo.Satisfier,
		}, {
			contextf(errorInfo.ArgsConstructor("woo %q", "bar"), "msg %d", 42),
			`msg 42: woo "bar"` + errorInfo.Suffix,
			errorInfo.Satisfier,
		}, {
			contextf(errorInfo.ArgsConstructor("something"), "bad"),
			"bad: something" + errorInfo.Suffix,
			errorInfo.Satisfier,
		}, {
			contextf(errorInfo.ArgsConstructor("foo"), ""),
			"foo" + errorInfo.Suffix,
			errorInfo.Satisfier,
		}, {
			contextf(errorInfo.ArgsConstructor(""), ""),
			errorInfo.Suffix,
			errorInfo.Satisfier,
		}, {
			contextf(errorInfo.WrapConstructor(fmt.Errorf("foo %s", "baz"), "bad"), "bar %d", 42),
			"bar 42: bad: foo baz",
			errorInfo.Satisfier,
		}, {
			contextf(errorInfo.WrapConstructor(stderrors.New("foo"), "bar"), "cause"),
			"cause: bar: foo",
			errorInfo.Satisfier,
		}, {
			contextf(errorInfo.WrapConstructor(stderrors.New(""), "bar"), "cause"),
			"cause: bar: ",
			errorInfo.Satisfier,
		}, {
			contextf(errorInfo.WrapConstructor(stderrors.New("woohoo"), ""), "cause"),
			"cause: woohoo",
			errorInfo.Satisfier,
		}, {
			contextf(errorInfo.WrapConstructor(stderrors.New(""), ""), "cause"),
			"cause: ",
			errorInfo.Satisfier,
		}, {
			contextf(errorInfo.WrapConstructor(nil, "woo"), "cause"),
			"cause: woo",
			errorInfo.Satisfier,
		}, {
			contextf(errorInfo.WrapConstructor(nil, ""), ""),
			"",
			errorInfo.Satisfier,
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
		for _, errorInfo := range errors.AllErrors {
			// Not using jc.Satisfies here, because it doesn't give a
			// nice string representation of the function. Also, you
			// can't take the address of a func, but you can store it
			// and take the address of the struct.
			satisfier := errorInfo.Satisfier
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
