// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"net/http"
	"net/url"

	"github.com/juju/errors"
	"github.com/juju/httprequest"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/base"
	coretesting "github.com/juju/juju/testing"
)

// APICallerFunc is a function type that implements APICaller.
// The only method that actually does anything is APICall itself
// which calls the function. The other methods are just stubs.
type APICallerFunc func(objType string, version int, id, request string, params, response interface{}) error

func (f APICallerFunc) APICall(objType string, version int, id, request string, params, response interface{}) error {
	return f(objType, version, id, request, params, response)
}

func (APICallerFunc) BestFacadeVersion(facade string) int {
	// TODO(fwereade): this should return something arbitrary (e.g. 37)
	// so that it can't be confused with mere uninitialized data.
	return 0
}

func (APICallerFunc) ModelTag() (names.ModelTag, bool) {
	return coretesting.ModelTag, true
}

func (APICallerFunc) Close() error {
	return nil
}

func (APICallerFunc) HTTPClient() (*httprequest.Client, error) {
	return nil, errors.New("no HTTP client available in this test")
}

func (APICallerFunc) ConnectStream(path string, attrs url.Values) (base.Stream, error) {
	return nil, errors.NotImplementedf("stream connection")
}

func (APICallerFunc) ConnectControllerStream(path string, attrs url.Values, headers http.Header) (base.Stream, error) {
	return nil, errors.NotImplementedf("controller stream connection")
}

// BestVersionCaller is an APICallerFunc that has a particular best version.
type BestVersionCaller struct {
	APICallerFunc
	BestVersion int
}

func (c BestVersionCaller) BestFacadeVersion(facade string) int {
	return c.BestVersion
}

// CallChecker is an APICaller implementation that checks
// calls as they are made.
type CallChecker struct {
	APICallerFunc

	// CallCount records the current call count.
	CallCount int
}

// APICall describes an expected API call.
type APICall struct {
	// If Check is non-nil, all other fields will be ignored and Check
	// will be called to check the call.
	Check func(objType string, version int, id, request string, params, response interface{}) error

	// Facade holds the expected call facade. If it's empty,
	// any facade will be accepted.
	Facade string

	// Version holds the expected call version. If it's zero,
	// any version will be accepted unless VersionIsZero is true.
	Version int

	// VersionIsZero holds whether the version is expected to be zero.
	VersionIsZero bool

	// Id holds the expected call id. If it's empty, any id will be
	// accepted unless IdIsEmpty is true.
	Id string

	// IdIsEmpty holds whether the call id is expected to be empty.
	IdIsEmpty bool

	// Method holds the expected method.
	Method string

	// Args holds the expected value of the call's argument.
	Args interface{}

	// Results is assigned to the result parameter of the call on return.
	Results interface{}

	// Error is returned from the call.
	Error error
}

// APICallChecker returns an APICaller implementation that checks
// API calls. Each element of calls corresponds to an expected
// API call. If more calls are made than there are elements, they
// will not be checked - check the value of the Count field
// to ensure that the expected number of calls have been made.
//
// Note that the returned value is not thread-safe - do not
// use it if the client is making concurrent calls.
func APICallChecker(c *gc.C, calls ...APICall) *CallChecker {
	var checker CallChecker
	checker.APICallerFunc = func(facade string, version int, id, method string, inArgs, outResults interface{}) error {
		call := checker.CallCount
		checker.CallCount++
		if call >= len(calls) {
			return nil
		}
		return checkArgs(c, calls[call], facade, version, id, method, inArgs, outResults)
	}
	return &checker
}

func checkArgs(c *gc.C, args APICall, facade string, version int, id, method string, inArgs, outResults interface{}) error {
	if args.Facade != "" {
		c.Check(facade, gc.Equals, args.Facade)
	}
	if args.Version != 0 {
		c.Check(version, gc.Equals, args.Version)
	} else if args.VersionIsZero {
		c.Check(version, gc.Equals, 0)
	}
	if args.Id != "" {
		c.Check(id, gc.Equals, args.Id)
	} else if args.IdIsEmpty {
		c.Check(id, gc.Equals, "")
	}
	if args.Method != "" {
		c.Check(method, gc.Equals, args.Method)
	}
	if args.Args != nil {
		c.Check(inArgs, jc.DeepEquals, args.Args)
	}
	if args.Results != nil {
		c.Check(outResults, gc.NotNil)
		testing.PatchValue(outResults, args.Results)
	}
	return args.Error
}

type notifyingAPICaller struct {
	base.APICaller
	called chan<- struct{}
}

func (c notifyingAPICaller) APICall(objType string, version int, id, request string, params, response interface{}) error {
	c.called <- struct{}{}
	return c.APICaller.APICall(objType, version, id, request, params, response)
}

// NotifyingAPICaller returns an APICaller implementation which sends a
// message on the given channel every time it receives a call.
func NotifyingAPICaller(c *gc.C, called chan<- struct{}, caller base.APICaller) base.APICaller {
	return notifyingAPICaller{
		APICaller: caller,
		called:    called,
	}
}

// StubFacadeCaller is a testing stub implementation of api/base.FacadeCaller.
type StubFacadeCaller struct {
	// Stub is the raw stub used to track calls and errors.
	Stub *testing.Stub
	// These control the values returned by the stub's methods.
	FacadeCallFn         func(name string, params, response interface{}) error
	ReturnName           string
	ReturnBestAPIVersion int
	ReturnRawAPICaller   base.APICaller
}

// FacadeCall implements api/base.FacadeCaller.
func (s *StubFacadeCaller) FacadeCall(request string, params, response interface{}) error {
	s.Stub.AddCall("FacadeCall", request, params, response)
	if err := s.Stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	if s.FacadeCallFn != nil {
		return s.FacadeCallFn(request, params, response)
	}
	return nil
}

// Name implements api/base.FacadeCaller.
func (s *StubFacadeCaller) Name() string {
	s.Stub.AddCall("Name")
	s.Stub.PopNoErr()

	return s.ReturnName
}

// BestAPIVersion implements api/base.FacadeCaller.
func (s *StubFacadeCaller) BestAPIVersion() int {
	s.Stub.AddCall("BestAPIVersion")
	s.Stub.PopNoErr()

	return s.ReturnBestAPIVersion
}

// RawAPICaller implements api/base.FacadeCaller.
func (s *StubFacadeCaller) RawAPICaller() base.APICaller {
	s.Stub.AddCall("RawAPICaller")
	s.Stub.PopNoErr()

	return s.ReturnRawAPICaller
}
