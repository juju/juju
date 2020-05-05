// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apicaller_test

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/worker/apicaller"
)

// RetryStrategySuite exercises the cases where we need to connect
// repeatedly, either to wait for provisioning errors or to fall
// back to other possible passwords. It covers OnlyConnect in detail,
// checking success and failure behaviour, but only checks suitable
// error paths for ScaryConnect (which does extra complex things like
// make api calls and rewrite passwords in config).
//
// Would be best of all to test all the ScaryConnect success/failure
// paths explicitly, but the combinatorial explosion makes it tricky;
// in the absence of a further decomposition of responsibilities, it
// seems best to at least decompose the testing. Which is more detailed
// than it was before, anyway.
type RetryStrategySuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&RetryStrategySuite{})

var testEntity = names.NewMachineTag("42")

func (s *RetryStrategySuite) TestOnlyConnectSuccess(c *gc.C) {
	stub := &testing.Stub{}
	stub.SetErrors(
		errNotProvisioned, // initial attempt, outside strategy
		errNotProvisioned, // first strategy attempt
		nil,               // success on second strategy attempt
	)
	// TODO(katco): 2016-08-09: lp:1611427
	strategy := utils.AttemptStrategy{Min: 3}
	conn, err := strategyTest(stub, strategy, func(apiOpen api.OpenFunc) (api.Connection, error) {
		return apicaller.OnlyConnect(&mockAgent{stub: stub, entity: testEntity}, apiOpen, loggo.GetLogger("test"))
	})
	checkOpenCalls(c, stub, "new", "new", "new")
	c.Check(conn, gc.NotNil)
	c.Check(err, jc.ErrorIsNil)
}

func (s *RetryStrategySuite) TestOnlyConnectOldPasswordSuccess(c *gc.C) {
	stub := &testing.Stub{}
	stub.SetErrors(
		errNotAuthorized,  // initial attempt, outside strategy
		errNotProvisioned, // fallback attempt, outside strategy
		errNotProvisioned, // first strategy attempt
		nil,               // second strategy attempt
	)
	// TODO(katco): 2016-08-09: lp:1611427
	strategy := utils.AttemptStrategy{Min: 3}
	conn, err := strategyTest(stub, strategy, func(apiOpen api.OpenFunc) (api.Connection, error) {
		return apicaller.OnlyConnect(&mockAgent{stub: stub, entity: testEntity}, apiOpen, loggo.GetLogger("test"))
	})
	checkOpenCalls(c, stub, "new", "old", "old", "old")
	c.Check(err, jc.ErrorIsNil)
	c.Check(conn, gc.NotNil)
}

func (s *RetryStrategySuite) TestOnlyConnectEventualError(c *gc.C) {
	conn, err := checkWaitProvisionedError(c, apicaller.OnlyConnect)
	c.Check(conn, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "splat pow")
}

func (s *RetryStrategySuite) TestScaryConnectEventualError(c *gc.C) {
	conn, err := checkWaitProvisionedError(c, apicaller.ScaryConnect)
	c.Check(conn, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "splat pow")
}

func checkWaitProvisionedError(c *gc.C, connect apicaller.ConnectFunc) (api.Connection, error) {
	stub := &testing.Stub{}
	stub.SetErrors(
		errNotProvisioned,       // initial attempt, outside strategy
		errNotProvisioned,       // first strategy attempt
		errNotProvisioned,       // second strategy attempt
		errors.New("splat pow"), // third strategy attempt
	)
	// TODO(katco): 2016-08-09: lp:1611427
	strategy := utils.AttemptStrategy{Min: 3}
	conn, err := strategyTest(stub, strategy, func(apiOpen api.OpenFunc) (api.Connection, error) {
		return connect(&mockAgent{stub: stub, entity: testEntity}, apiOpen, loggo.GetLogger("test"))
	})
	checkOpenCalls(c, stub, "new", "new", "new", "new")
	return conn, err
}

func (s *RetryStrategySuite) TestOnlyConnectNeverProvisioned(c *gc.C) {
	conn, err := checkWaitNeverProvisioned(c, apicaller.OnlyConnect)
	c.Check(conn, gc.IsNil)
	c.Check(errors.Cause(err), gc.DeepEquals, errNotProvisioned)
}

func (s *RetryStrategySuite) TestScaryConnectNeverProvisioned(c *gc.C) {
	conn, err := checkWaitNeverProvisioned(c, apicaller.ScaryConnect)
	c.Check(conn, gc.IsNil)
	c.Check(err, gc.Equals, apicaller.ErrConnectImpossible)
}

func checkWaitNeverProvisioned(c *gc.C, connect apicaller.ConnectFunc) (api.Connection, error) {
	stub := &testing.Stub{}
	stub.SetErrors(
		errNotProvisioned, // initial attempt, outside strategy
		errNotProvisioned, // first strategy attempt
		errNotProvisioned, // second strategy attempt
		errNotProvisioned, // third strategy attempt
	)
	// TODO(katco): 2016-08-09: lp:1611427
	strategy := utils.AttemptStrategy{Min: 3}
	conn, err := strategyTest(stub, strategy, func(apiOpen api.OpenFunc) (api.Connection, error) {
		return connect(&mockAgent{stub: stub, entity: testEntity}, apiOpen, loggo.GetLogger("test"))
	})
	checkOpenCalls(c, stub, "new", "new", "new", "new")
	return conn, err
}
