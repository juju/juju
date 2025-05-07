// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apicaller_test

import (
	"context"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/retry"
	"github.com/juju/tc"
	"github.com/juju/testing"

	"github.com/juju/juju/api"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/worker/apicaller"
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

var _ = tc.Suite(&RetryStrategySuite{})

var testEntity = names.NewMachineTag("42")

var strategy = retry.CallArgs{
	Clock:    clock.WallClock,
	Delay:    time.Millisecond,
	Attempts: 3,
}

func (s *RetryStrategySuite) TestOnlyConnectSuccess(c *tc.C) {
	stub := &testing.Stub{}
	stub.SetErrors(
		errNotProvisioned, // initial attempt, outside strategy
		errNotProvisioned, // first strategy attempt
		nil,               // success on second strategy attempt
	)
	conn, err := strategyTest(stub, strategy, func(apiOpen api.OpenFunc) (api.Connection, error) {
		return apicaller.OnlyConnect(context.Background(), &mockAgent{stub: stub, entity: testEntity}, apiOpen, loggertesting.WrapCheckLog(c))
	})
	checkOpenCalls(c, stub, "new", "new", "new")
	c.Check(conn, tc.NotNil)
	c.Check(err, tc.ErrorIsNil)
}

func (s *RetryStrategySuite) TestOnlyConnectOldPasswordSuccess(c *tc.C) {
	stub := &testing.Stub{}
	stub.SetErrors(
		errNotAuthorized,  // initial attempt, outside strategy
		errNotProvisioned, // fallback attempt, outside strategy
		errNotProvisioned, // first strategy attempt
		nil,               // second strategy attempt
	)
	conn, err := strategyTest(stub, strategy, func(apiOpen api.OpenFunc) (api.Connection, error) {
		return apicaller.OnlyConnect(context.Background(), &mockAgent{stub: stub, entity: testEntity}, apiOpen, loggertesting.WrapCheckLog(c))
	})
	checkOpenCalls(c, stub, "new", "old", "old", "old")
	c.Check(err, tc.ErrorIsNil)
	c.Check(conn, tc.NotNil)
}

func (s *RetryStrategySuite) TestOnlyConnectEventualError(c *tc.C) {
	conn, err := checkWaitProvisionedError(c, apicaller.OnlyConnect)
	c.Check(conn, tc.IsNil)
	c.Check(err, tc.ErrorMatches, "splat pow")
}

func (s *RetryStrategySuite) TestScaryConnectEventualError(c *tc.C) {
	conn, err := checkWaitProvisionedError(c, apicaller.ScaryConnect)
	c.Check(conn, tc.IsNil)
	c.Check(err, tc.ErrorMatches, "splat pow")
}

func checkWaitProvisionedError(c *tc.C, connect apicaller.ConnectFunc) (api.Connection, error) {
	stub := &testing.Stub{}
	stub.SetErrors(
		errNotProvisioned,       // initial attempt, outside strategy
		errNotProvisioned,       // first strategy attempt
		errNotProvisioned,       // second strategy attempt
		errors.New("splat pow"), // third strategy attempt
	)
	conn, err := strategyTest(stub, strategy, func(apiOpen api.OpenFunc) (api.Connection, error) {
		return connect(context.Background(), &mockAgent{stub: stub, entity: testEntity}, apiOpen, loggertesting.WrapCheckLog(c))
	})
	checkOpenCalls(c, stub, "new", "new", "new", "new")
	return conn, err
}

func (s *RetryStrategySuite) TestOnlyConnectNeverProvisioned(c *tc.C) {
	conn, err := checkWaitNeverProvisioned(c, apicaller.OnlyConnect)
	c.Check(conn, tc.IsNil)
	c.Check(errors.Cause(err), tc.DeepEquals, errNotProvisioned)
}

func (s *RetryStrategySuite) TestScaryConnectNeverProvisioned(c *tc.C) {
	conn, err := checkWaitNeverProvisioned(c, apicaller.ScaryConnect)
	c.Check(conn, tc.IsNil)
	c.Check(err, tc.Equals, apicaller.ErrConnectImpossible)
}

func checkWaitNeverProvisioned(c *tc.C, connect apicaller.ConnectFunc) (api.Connection, error) {
	stub := &testing.Stub{}
	stub.SetErrors(
		errNotProvisioned, // initial attempt, outside strategy
		errNotProvisioned, // first strategy attempt
		errNotProvisioned, // second strategy attempt
		errNotProvisioned, // third strategy attempt
	)
	conn, err := strategyTest(stub, strategy, func(apiOpen api.OpenFunc) (api.Connection, error) {
		return connect(context.Background(), &mockAgent{stub: stub, entity: testEntity}, apiOpen, loggertesting.WrapCheckLog(c))
	})
	checkOpenCalls(c, stub, "new", "new", "new", "new")
	return conn, err
}
