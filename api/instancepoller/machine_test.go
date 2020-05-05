// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancepoller_test

import (
	"reflect"
	"time"

	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	apitesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/instancepoller"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	coretesting "github.com/juju/juju/testing"
)

type MachineSuite struct {
	coretesting.BaseSuite

	tag names.MachineTag
}

var _ = gc.Suite(&MachineSuite{})

func (s *MachineSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.tag = names.NewMachineTag("42")
}

func (s *MachineSuite) TestNonFacadeMethods(c *gc.C) {
	nopCaller := apitesting.APICallerFunc(
		func(_ string, _ int, _, _ string, _, _ interface{}) error {
			c.Fatalf("facade call was not expected")
			return nil
		},
	)
	machine := instancepoller.NewMachine(nopCaller, s.tag, life.Dying)

	c.Assert(machine.Id(), gc.Equals, "42")
	c.Assert(machine.Tag(), jc.DeepEquals, s.tag)
	c.Assert(machine.String(), gc.Equals, "42")
	c.Assert(machine.Life(), gc.Equals, life.Dying)
}

// methodWrapper wraps a Machine method call and returns the error,
// ignoring the result (if any).
type methodWrapper func(*instancepoller.Machine) error

// machineErrorTests contains all the necessary information to test
// how each Machine method handles client- and server-side API errors,
// as well as the case when the server-side API returns more results
// than expected.
var machineErrorTests = []struct {
	method     string // only for logging
	wrapper    methodWrapper
	resultsRef interface{} // an instance of the server-side method's result type
}{{
	method:     "Refresh",
	wrapper:    (*instancepoller.Machine).Refresh,
	resultsRef: params.LifeResults{},
}, {
	method: "IsManual",
	wrapper: func(m *instancepoller.Machine) error {
		_, err := m.IsManual()
		return err
	},
	resultsRef: params.BoolResults{},
}, {
	method: "InstanceId",
	wrapper: func(m *instancepoller.Machine) error {
		_, err := m.InstanceId()
		return err
	},
	resultsRef: params.StringResults{},
}, {
	method: "Status",
	wrapper: func(m *instancepoller.Machine) error {
		_, err := m.Status()
		return err
	},
	resultsRef: params.StatusResults{},
}, {
	method: "InstanceStatus",
	wrapper: func(m *instancepoller.Machine) error {
		_, err := m.InstanceStatus()
		return err
	},
	resultsRef: params.StatusResults{},
}, {
	method: "SetInstanceStatus",
	wrapper: func(m *instancepoller.Machine) error {
		return m.SetInstanceStatus("", "", nil)
	},
	resultsRef: params.ErrorResults{},
}, {
	method: "SetProviderNetworkConfig",
	wrapper: func(m *instancepoller.Machine) error {
		_, _, err := m.SetProviderNetworkConfig(nil)
		return err
	},
	resultsRef: params.SetProviderNetworkConfigResults{},
}}

func (s *MachineSuite) TestClientError(c *gc.C) {
	for i, test := range machineErrorTests {
		c.Logf("test #%d: %s", i, test.method)
		s.CheckClientError(c, test.wrapper)
	}
}

func (s *MachineSuite) TestServerError(c *gc.C) {
	err := apiservertesting.ServerError("server error!")
	expected := err.Error()
	for i, test := range machineErrorTests {
		c.Logf("test #%d: %s", i, test.method)
		results := MakeResultsWithErrors(test.resultsRef, err, 1)
		s.CheckServerError(c, test.wrapper, expected, results)
	}
}

func (s *MachineSuite) TestTooManyResultsServerError(c *gc.C) {
	err := apiservertesting.ServerError("some error")
	expected := "expected 1 result, got 2"
	for i, test := range machineErrorTests {
		c.Logf("test #%d: %s", i, test.method)
		results := MakeResultsWithErrors(test.resultsRef, err, 2)
		s.CheckServerError(c, test.wrapper, expected, results)
	}
}

func (s *MachineSuite) TestRefreshSuccess(c *gc.C) {
	results := params.LifeResults{
		Results: []params.LifeResult{{Life: life.Dying}},
	}
	apiCaller := successAPICaller(c, "Life", entitiesArgs, results)
	machine := instancepoller.NewMachine(apiCaller, s.tag, life.Alive)
	c.Check(machine.Refresh(), jc.ErrorIsNil)
	c.Check(machine.Life(), gc.Equals, life.Dying)
	c.Check(apiCaller.CallCount, gc.Equals, 1)
}

func (s *MachineSuite) TestStatusSuccess(c *gc.C) {
	now := time.Now()
	expectStatus := params.StatusResult{
		Status: "foo",
		Info:   "bar",
		Data: map[string]interface{}{
			"int":    42,
			"bool":   true,
			"float":  3.14,
			"slice":  []string{"a", "b"},
			"map":    map[int]string{5: "five"},
			"string": "argh",
		},
		Since: &now,
	}
	results := params.StatusResults{Results: []params.StatusResult{expectStatus}}
	apiCaller := successAPICaller(c, "Status", entitiesArgs, results)
	machine := instancepoller.NewMachine(apiCaller, s.tag, life.Alive)
	status, err := machine.Status()
	c.Check(err, jc.ErrorIsNil)
	c.Check(status, jc.DeepEquals, expectStatus)
	c.Check(apiCaller.CallCount, gc.Equals, 1)
}

func (s *MachineSuite) TestIsManualSuccess(c *gc.C) {
	results := params.BoolResults{
		Results: []params.BoolResult{{Result: true}},
	}
	apiCaller := successAPICaller(c, "AreManuallyProvisioned", entitiesArgs, results)
	machine := instancepoller.NewMachine(apiCaller, s.tag, life.Alive)
	isManual, err := machine.IsManual()
	c.Check(err, jc.ErrorIsNil)
	c.Check(isManual, jc.IsTrue)
	c.Check(apiCaller.CallCount, gc.Equals, 1)
}

func (s *MachineSuite) TestInstanceIdSuccess(c *gc.C) {
	results := params.StringResults{
		Results: []params.StringResult{{Result: "i-foo"}},
	}
	apiCaller := successAPICaller(c, "InstanceId", entitiesArgs, results)
	machine := instancepoller.NewMachine(apiCaller, s.tag, life.Alive)
	instId, err := machine.InstanceId()
	c.Check(err, jc.ErrorIsNil)
	c.Check(instId, gc.Equals, instance.Id("i-foo"))
	c.Check(apiCaller.CallCount, gc.Equals, 1)
}

func (s *MachineSuite) TestInstanceStatusSuccess(c *gc.C) {
	results := params.StatusResults{
		Results: []params.StatusResult{{
			Status: status.Provisioning.String(),
		}},
	}
	apiCaller := successAPICaller(c, "InstanceStatus", entitiesArgs, results)
	machine := instancepoller.NewMachine(apiCaller, s.tag, life.Alive)
	statusResult, err := machine.InstanceStatus()
	c.Check(err, jc.ErrorIsNil)
	c.Check(statusResult.Status, gc.DeepEquals, status.Provisioning.String())
	c.Check(apiCaller.CallCount, gc.Equals, 1)
}

func (s *MachineSuite) TestSetInstanceStatusSuccess(c *gc.C) {
	expectArgs := params.SetStatus{
		Entities: []params.EntityStatusArgs{{
			Tag:    "machine-42",
			Status: "RUNNING",
		}}}
	results := params.ErrorResults{
		Results: []params.ErrorResult{{Error: nil}},
	}
	apiCaller := successAPICaller(c, "SetInstanceStatus", expectArgs, results)
	machine := instancepoller.NewMachine(apiCaller, s.tag, life.Alive)
	err := machine.SetInstanceStatus("RUNNING", "", nil)
	c.Check(err, jc.ErrorIsNil)
	c.Check(apiCaller.CallCount, gc.Equals, 1)
}

func (s *MachineSuite) TestSetProviderNetworkConfigSuccess(c *gc.C) {
	cfg := []network.InterfaceInfo{{
		DeviceIndex: 0,
		CIDR:        "10.0.0.0/24",
		Addresses: []network.ProviderAddress{
			network.NewProviderAddress("10.0.0.42"),
		},
	}}
	expectArgs := params.SetProviderNetworkConfig{
		Args: []params.ProviderNetworkConfig{{
			Tag:     "machine-42",
			Configs: params.NetworkConfigFromInterfaceInfo(cfg),
		}},
	}
	results := params.SetProviderNetworkConfigResults{
		Results: []params.SetProviderNetworkConfigResult{{Error: nil}},
	}
	apiCaller := successAPICaller(c, "SetProviderNetworkConfig", expectArgs, results)
	machine := instancepoller.NewMachine(apiCaller, s.tag, life.Alive)
	_, _, err := machine.SetProviderNetworkConfig(cfg)
	c.Check(err, jc.ErrorIsNil)
	c.Check(apiCaller.CallCount, gc.Equals, 1)
}

func (s *MachineSuite) CheckClientError(c *gc.C, wf methodWrapper) {
	apiCaller := clientErrorAPICaller(c, "", nil)
	machine := instancepoller.NewMachine(apiCaller, s.tag, life.Alive)
	c.Check(wf(machine), gc.ErrorMatches, "client error!")
	c.Check(apiCaller.CallCount, gc.Equals, 1)
}

func (s *MachineSuite) CheckServerError(c *gc.C, wf methodWrapper, expectErr string, serverResults interface{}) {
	apiCaller := successAPICaller(c, "", nil, serverResults)
	machine := instancepoller.NewMachine(apiCaller, s.tag, life.Alive)
	c.Check(wf(machine), gc.ErrorMatches, expectErr)
	c.Check(apiCaller.CallCount, gc.Equals, 1)
}

var entitiesArgs = params.Entities{
	Entities: []params.Entity{{Tag: "machine-42"}},
}

// MakeResultsWithErrors constructs a new instance of the results type
// (from apiserver/params), matching the given resultsRef, finds its
// first field (expected to be a slice, usually "Results") and adds
// howMany elements to it, setting the Error field of each element to
// err.
//
// This helper makes a few assumptions:
// - resultsRef's type is a struct and has a single field (commonly - "Results")
// - that field is a slice of structs, which have an Error field
// - the Error field is of type *params.Error
//
// Example:
//   err := apiservertesting.ServerError("foo")
//   r := MakeResultsWithErrors(params.LifeResults{}, err, 2)
// is equvalent to:
//   r := params.LifeResults{Results: []params.LifeResult{{Error: err}, {Error: err}}}
//
func MakeResultsWithErrors(resultsRef interface{}, err *params.Error, howMany int) interface{} {
	// Make a new instance of the same type as resultsRef.
	resultsType := reflect.TypeOf(resultsRef)
	newResults := reflect.New(resultsType).Elem()

	// Make a new empty slice for the results.
	sliceField := newResults.Field(0)
	newSlice := reflect.New(sliceField.Type()).Elem()

	// Make a new result of the slice's element type and set it to err.
	newResult := reflect.New(newSlice.Type().Elem()).Elem()
	newResult.FieldByName("Error").Set(reflect.ValueOf(err))

	// Append howMany copies of newResult to the slice.
	for howMany > 0 {
		sliceField.Set(reflect.Append(sliceField, newResult))
		howMany--
	}

	return newResults.Interface()
}

// TODO(dimitern): Move this and MakeResultsWithErrors in params/testing ?
func (MachineSuite) TestMakeResultsWithErrors(c *gc.C) {
	err := apiservertesting.ServerError("foo")
	r1 := MakeResultsWithErrors(params.LifeResults{}, err, 2)
	r2 := params.LifeResults{Results: []params.LifeResult{{Error: err}, {Error: err}}}
	c.Assert(r1, jc.DeepEquals, r2)
}
