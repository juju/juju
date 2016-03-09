// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"net/url"

	"github.com/juju/errors"
	"github.com/juju/httprequest"
	"github.com/juju/names"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

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
	return 0
}

func (APICallerFunc) ModelTag() (names.ModelTag, error) {
	return coretesting.ModelTag, nil
}

func (APICallerFunc) Close() error {
	return nil
}

func (APICallerFunc) HTTPClient() (*httprequest.Client, error) {
	return nil, errors.New("no HTTP client available in this test")
}

func (APICallerFunc) ConnectStream(path string, attrs url.Values) (base.Stream, error) {
	return nil, errors.New("stream connection unimplemented")
}

// CheckArgs holds the possible arguments to CheckingAPICaller(). Any
// fields non empty fields will be checked to match the arguments
// recieved by the APICall() method of the returned APICallerFunc. If
// Id is empty, but IdIsEmpty is true, the id argument is checked to
// be empty. The same applies to Version being empty, but if
// VersionIsZero set to true the version is checked to be 0.
type CheckArgs struct {
	Facade  string
	Version int
	Id      string
	Method  string
	Args    interface{}
	Results interface{}

	IdIsEmpty     bool
	VersionIsZero bool
}

func checkArgs(c *gc.C, args *CheckArgs, facade string, version int, id, method string, inArgs, outResults interface{}) {
	if args == nil {
		c.Logf("checkArgs: args is nil!")
		return
	} else {
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
	}
}

// CheckingAPICaller returns an APICallerFunc which can report the
// number of times its APICall() method was called (if numCalls is not
// nil), as well as check if any of the arguments passed to the
// APICall() method match the values given in args (if args itself is
// not nil, otherwise no arguments are checked). The final error
// result of the APICall() will be set to err.
func CheckingAPICaller(c *gc.C, args *CheckArgs, numCalls *int, err error) base.APICallCloser {
	return APICallerFunc(
		func(facade string, version int, id, method string, inArgs, outResults interface{}) error {
			if numCalls != nil {
				*numCalls++
			}
			if args != nil {
				checkArgs(c, args, facade, version, id, method, inArgs, outResults)
			}
			return err
		},
	)
}

// NotifyingCheckingAPICaller returns an APICallerFunc which sends a message on the channel "called" every
// time it recives a call, as well as check if any of the arguments passed to the APICall() method match
// the values given in args (if args itself is not nil, otherwise no arguments are checked). The final
// error result of the APICall() will be set to err.
func NotifyingCheckingAPICaller(c *gc.C, args *CheckArgs, called chan struct{}, err error) base.APICaller {
	return APICallerFunc(
		func(facade string, version int, id, method string, inArgs, outResults interface{}) error {
			called <- struct{}{}
			if args != nil {
				checkArgs(c, args, facade, version, id, method, inArgs, outResults)
			}
			return err
		},
	)
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
