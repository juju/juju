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

// errorInfo holds information about a single error type: a satisfier
// function, wrapping and variable arguments constructors and message
// suffix.
type errorInfo struct {
	satisfier       func(error) bool
	argsConstructor func(string, ...interface{}) error
	wrapConstructor func(error, string) error
	suffix          string
}

// allErrors holds information for all defined errors. When adding new
// errors, add them here as well to include them in tests.
var allErrors = []*errorInfo{
	&errorInfo{errors.IsNotFound, errors.NotFoundf, errors.NewNotFound, " not found"},
	&errorInfo{errors.IsUnauthorized, errors.Unauthorizedf, errors.NewUnauthorized, ""},
	&errorInfo{errors.IsNotImplemented, errors.NotImplementedf, errors.NewNotImplemented, " not implemented"},
	&errorInfo{errors.IsAlreadyExists, errors.AlreadyExistsf, errors.NewAlreadyExists, " already exists"},
	&errorInfo{errors.IsNotSupported, errors.NotSupportedf, errors.NewNotSupported, " not supported"},
}

type errorsSuite struct{}

var _ = gc.Suite(&errorsSuite{})

func Test(t *testing.T) {
	gc.TestingT(t)
}

func (t *errorInfo) satisfierName() string {
	value := reflect.ValueOf(t.satisfier)
	f := runtime.FuncForPC(value.Pointer())
	return f.Name()
}

func (t *errorInfo) equal(t0 *errorInfo) bool {
	if t0 == nil {
		return false
	}
	return t.satisfierName() == t0.satisfierName()
}

type errorTest struct {
	err     error
	message string
	errInfo *errorInfo
}

func contextf(err error, format string, args ...interface{}) error {
	errors.Contextf(&err, format, args...)
	return err
}

func maskf(err error, format string, args ...interface{}) error {
	errors.Maskf(&err, format, args...)
	return err
}

func mustSatisfy(c *gc.C, err error, errInfo *errorInfo) {
	if errInfo != nil {
		msg := fmt.Sprintf("%#v must satisfy %v", err, errInfo.satisfierName())
		c.Check(err, jc.Satisfies, errInfo.satisfier, gc.Commentf(msg))
	}
}

func mustNotSatisfy(c *gc.C, err error, errInfo *errorInfo) {
	if errInfo != nil {
		msg := fmt.Sprintf("%#v must not satisfy %v", err, errInfo.satisfierName())
		c.Check(err, gc.Not(jc.Satisfies), errInfo.satisfier, gc.Commentf(msg))
	}
}

func checkErrorMatches(c *gc.C, err error, message string, errInfo *errorInfo) {
	if message == "<nil>" {
		c.Check(err, gc.IsNil)
		c.Check(errInfo, gc.IsNil)
	} else {
		c.Check(err, gc.ErrorMatches, message)
	}
}

func runErrorTests(c *gc.C, errorTests []errorTest, checkMustSatisfy bool) {
	for i, t := range errorTests {
		c.Logf("test %d: %T: %v", i, t.err, t.err)
		checkErrorMatches(c, t.err, t.message, t.errInfo)
		if checkMustSatisfy {
			mustSatisfy(c, t.err, t.errInfo)
		}

		// Check all other satisfiers to make sure none match.
		for _, otherErrInfo := range allErrors {
			if checkMustSatisfy && otherErrInfo.equal(t.errInfo) {
				continue
			}
			mustNotSatisfy(c, t.err, otherErrInfo)
		}
	}
}

func (*errorsSuite) TestMaskf(c *gc.C) {
	// Ensure Maskf masks all known errors and their satisfiers don't
	// succeed.
	errorTests := []errorTest{}
	for _, errInfo := range allErrors {
		errorTests = append(errorTests, []errorTest{{
			maskf(nil, "masked"),
			"<nil>",
			nil,
		}, {
			maskf(stderrors.New("blast"), "masked"),
			"masked: blast",
			nil,
		}, {
			maskf(errInfo.argsConstructor("foo %d", 42), "masked %d", 69),
			"masked 69: foo 42" + errInfo.suffix,
			errInfo,
		}, {
			maskf(errInfo.argsConstructor(""), "masked"),
			"masked: " + errInfo.suffix,
			errInfo,
		}, {
			maskf(errInfo.wrapConstructor(stderrors.New("pow!"), "woo"), "masked"),
			"masked: woo: pow!",
			errInfo,
		}}...)
	}

	runErrorTests(c, errorTests, false)
}

func (*errorsSuite) TestContextf(c *gc.C) {
	// Ensure Contextf masks only unknown error types, but passes through known ones.
	errorTests := []errorTest{}
	for _, errInfo := range allErrors {
		errorTests = append(errorTests, []errorTest{{
			contextf(nil, "prefix"),
			"<nil>",
			nil,
		}, {
			contextf(stderrors.New("blast"), "prefix"),
			"prefix: blast",
			nil,
		}, {
			contextf(errInfo.argsConstructor("foo %d", 42), "prefix %d", 69),
			"prefix 69: foo 42" + errInfo.suffix,
			errInfo,
		}, {
			contextf(errInfo.argsConstructor(""), "prefix"),
			"prefix: " + errInfo.suffix,
			errInfo,
		}, {
			contextf(errInfo.wrapConstructor(stderrors.New("pow!"), "woo"), "prefix"),
			"prefix: woo: pow!",
			errInfo,
		}}...)
	}

	runErrorTests(c, errorTests, true)
}

func (*errorsSuite) TestAllErrors(c *gc.C) {
	errorTests := []errorTest{}
	for _, errInfo := range allErrors {
		errorTests = append(errorTests, []errorTest{{
			nil,
			"<nil>",
			nil,
		}, {
			errInfo.argsConstructor("foo %d", 42),
			"foo 42" + errInfo.suffix,
			errInfo,
		}, {
			errInfo.argsConstructor(""),
			errInfo.suffix,
			errInfo,
		}, {
			errInfo.wrapConstructor(stderrors.New("pow!"), "prefix"),
			"prefix: pow!",
			errInfo,
		}, {
			errInfo.wrapConstructor(stderrors.New("pow!"), ""),
			"pow!",
			errInfo,
		}, {
			errInfo.wrapConstructor(nil, "prefix"),
			"prefix",
			errInfo,
		}}...)
	}

	runErrorTests(c, errorTests, true)
}
