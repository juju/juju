// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apicaller_test

import (
	"context"
	"errors"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/api"
	apiagent "github.com/juju/juju/api/agent/agent"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/apicaller"
	"github.com/juju/juju/rpc/params"
)

// ScaryConnectSuite should cover all the *lines* where we get a connection
// without triggering the checkProvisionedStrategy ugliness. It tests the
// various conditions in isolation; it's possible that some real scenarios
// may trigger more than one of these, but it's impractical to test *every*
// possible *path*.
type ScaryConnectSuite struct {
	testing.IsolationSuite
}

var _ = tc.Suite(&ScaryConnectSuite{})

func (*ScaryConnectSuite) TestEntityAlive(c *tc.C) {
	testEntityFine(c, apiagent.Alive)
}

func (*ScaryConnectSuite) TestEntityDying(c *tc.C) {
	testEntityFine(c, apiagent.Dying)
}

func testEntityFine(c *tc.C, life apiagent.Life) {
	stub := &testing.Stub{}
	expectConn := &mockConn{stub: stub}
	apiOpen := func(ctx context.Context, info *api.Info, opts api.DialOpts) (api.Connection, error) {
		// no apiOpen stub calls necessary in this suite; covered
		// by RetrySuite, just an extra complication here.
		return expectConn, nil
	}

	// to make the point that this code should be entity-agnostic,
	// use an entity that doesn't correspond to an agent at all.
	entity := names.NewApplicationTag("omg")
	connect := func() (api.Connection, error) {
		return apicaller.ScaryConnect(context.Background(), &mockAgent{
			stub:   stub,
			model:  coretesting.ModelTag,
			entity: entity,
		}, apiOpen, loggertesting.WrapCheckLog(c))
	}

	conn, err := lifeTest(c, stub, apiagent.Alive, connect)
	c.Check(conn, tc.Equals, expectConn)
	c.Check(err, jc.ErrorIsNil)
	stub.CheckCalls(c, []testing.StubCall{{
		FuncName: "Life",
		Args:     []interface{}{entity},
	}, {
		FuncName: "SetPassword",
		Args:     []interface{}{entity, "new"},
	}})
}

func (*ScaryConnectSuite) TestEntityDead(c *tc.C) {
	// permanent failure case
	stub := &testing.Stub{}
	expectConn := &mockConn{stub: stub}
	apiOpen := func(ctx context.Context, info *api.Info, opts api.DialOpts) (api.Connection, error) {
		return expectConn, nil
	}

	entity := names.NewApplicationTag("omg")
	connect := func() (api.Connection, error) {
		return apicaller.ScaryConnect(context.Background(), &mockAgent{
			stub:   stub,
			model:  coretesting.ModelTag,
			entity: entity,
		}, apiOpen, loggertesting.WrapCheckLog(c))
	}

	conn, err := lifeTest(c, stub, apiagent.Dead, connect)
	c.Check(conn, tc.IsNil)
	c.Check(err, tc.Equals, apicaller.ErrConnectImpossible)
	stub.CheckCalls(c, []testing.StubCall{{
		FuncName: "Life",
		Args:     []interface{}{entity},
	}, {
		FuncName: "Close",
	}})
}

func (*ScaryConnectSuite) TestEntityDenied(c *tc.C) {
	// permanent failure case
	stub := &testing.Stub{}
	stub.SetErrors(apiagent.ErrDenied)
	expectConn := &mockConn{stub: stub}
	apiOpen := func(ctx context.Context, info *api.Info, opts api.DialOpts) (api.Connection, error) {
		return expectConn, nil
	}

	entity := names.NewApplicationTag("omg")
	connect := func() (api.Connection, error) {
		return apicaller.ScaryConnect(context.Background(), &mockAgent{
			stub:   stub,
			model:  coretesting.ModelTag,
			entity: entity,
		}, apiOpen, loggertesting.WrapCheckLog(c))
	}

	conn, err := lifeTest(c, stub, apiagent.Dead, connect)
	c.Check(conn, tc.IsNil)
	c.Check(err, tc.Equals, apicaller.ErrConnectImpossible)
	stub.CheckCalls(c, []testing.StubCall{{
		FuncName: "Life",
		Args:     []interface{}{entity},
	}, {
		FuncName: "Close",
	}})
}

func (*ScaryConnectSuite) TestEntityUnknownLife(c *tc.C) {
	// "random" failure case
	stub := &testing.Stub{}
	expectConn := &mockConn{stub: stub}
	apiOpen := func(ctx context.Context, info *api.Info, opts api.DialOpts) (api.Connection, error) {
		return expectConn, nil
	}

	entity := names.NewApplicationTag("omg")
	connect := func() (api.Connection, error) {
		return apicaller.ScaryConnect(context.Background(), &mockAgent{
			stub:   stub,
			model:  coretesting.ModelTag,
			entity: entity,
		}, apiOpen, loggertesting.WrapCheckLog(c))
	}

	conn, err := lifeTest(c, stub, apiagent.Life("zombie"), connect)
	c.Check(conn, tc.IsNil)
	c.Check(err, tc.ErrorMatches, `unknown life value "zombie"`)
	stub.CheckCalls(c, []testing.StubCall{{
		FuncName: "Life",
		Args:     []interface{}{entity},
	}, {
		FuncName: "Close",
	}})
}

func (*ScaryConnectSuite) TestChangePasswordConfigError(c *tc.C) {
	// "random" failure case
	stub := createUnauthorisedStub(nil, errors.New("zap"))
	err := checkChangePassword(c, stub)
	c.Check(err, tc.ErrorMatches, "zap")
	stub.CheckCallNames(c,
		"Life", "ChangeConfig",
		"Close",
	)
}

func (*ScaryConnectSuite) TestChangePasswordRemoteError(c *tc.C) {
	// "random" failure case
	stub := createUnauthorisedStub(nil, nil, nil, nil, errors.New("pow"))
	err := checkChangePassword(c, stub)
	c.Check(err, tc.ErrorMatches, "pow")
	stub.CheckCallNames(c,
		"Life", "ChangeConfig",
		// Be careful, these are two different SetPassword receivers.
		"SetPassword", "SetOldPassword", "SetPassword",
		"Close",
	)
	checkSaneChange(c, stub.Calls()[2:5])
}

func (*ScaryConnectSuite) TestChangePasswordRemoteDenied(c *tc.C) {
	// permanent failure case
	stub := createUnauthorisedStub(nil, nil, nil, nil, apiagent.ErrDenied)
	err := checkChangePassword(c, stub)
	c.Check(err, tc.Equals, apicaller.ErrConnectImpossible)
	stub.CheckCallNames(c,
		"Life", "ChangeConfig",
		// Be careful, these are two different SetPassword receivers.
		"SetPassword", "SetOldPassword", "SetPassword",
		"Close",
	)
	checkSaneChange(c, stub.Calls()[2:5])
}

func (s *ScaryConnectSuite) TestChangePasswordSuccessAfterUnauthorisedError(c *tc.C) {
	// This will try to login with old password if current one fails.
	stub := createUnauthorisedStub()
	s.assertChangePasswordSuccess(c, stub)
}

func (s *ScaryConnectSuite) TestChangePasswordSuccessAfterBadCurrentPasswordError(c *tc.C) {
	// This will try to login with old password if current one fails.
	stub := createPasswordCheckStub(apiservererrors.ErrUnauthorized)
	s.assertChangePasswordSuccess(c, stub)
}

func (*ScaryConnectSuite) assertChangePasswordSuccess(c *tc.C, stub *testing.Stub) {
	err := checkChangePassword(c, stub)
	c.Check(err, tc.Equals, apicaller.ErrChangedPassword)
	stub.CheckCallNames(c,
		"Life", "ChangeConfig",
		// Be careful, these are two different SetPassword receivers.
		"SetPassword", "SetOldPassword", "SetPassword",
		"Close",
	)
	checkSaneChange(c, stub.Calls()[2:5])
}

func createUnauthorisedStub(errs ...error) *testing.Stub {
	return createPasswordCheckStub(&params.Error{Code: params.CodeUnauthorized}, errs...)
}

func createPasswordCheckStub(currentPwdLoginErr error, errs ...error) *testing.Stub {
	allErrs := append([]error{currentPwdLoginErr, nil}, errs...)

	stub := &testing.Stub{}
	stub.SetErrors(allErrs...)
	return stub
}

func checkChangePassword(c *tc.C, stub *testing.Stub) error {
	// We prepend the unauth/success pair that triggers password
	// change, and consume them in apiOpen below...
	//errUnauth := &params.Error{Code: params.CodeUnauthorized}
	//allErrs := append([]error{errUnauth, nil}, errs...)
	//
	//stub := &testing.Stub{}
	//stub.SetErrors(allErrs...)
	expectConn := &mockConn{stub: stub}
	apiOpen := func(ctx context.Context, info *api.Info, opts api.DialOpts) (api.Connection, error) {
		// ...but we *don't* record the calls themselves; they
		// are tested plenty elsewhere, and hiding them makes
		// client code simpler.
		if err := stub.NextErr(); err != nil {
			return nil, err
		}
		return expectConn, nil
	}

	entity := names.NewApplicationTag("omg")
	connect := func() (api.Connection, error) {
		return apicaller.ScaryConnect(context.Background(), &mockAgent{
			stub:   stub,
			model:  coretesting.ModelTag,
			entity: entity,
		}, apiOpen, loggertesting.WrapCheckLog(c))
	}

	conn, err := lifeTest(c, stub, apiagent.Alive, connect)
	c.Check(conn, tc.IsNil)
	return err
}

func checkSaneChange(c *tc.C, calls []testing.StubCall) {
	c.Assert(calls, tc.HasLen, 3)
	localSet := calls[0]
	localSetOld := calls[1]
	remoteSet := calls[2]
	chosePassword := localSet.Args[0].(string)
	switch chosePassword {
	case "", "new", "old":
		c.Fatalf("very bad new password: %q", chosePassword)
	}

	c.Check(localSet, jc.DeepEquals, testing.StubCall{
		FuncName: "SetPassword",
		Args:     []interface{}{chosePassword},
	})
	c.Check(localSetOld, jc.DeepEquals, testing.StubCall{
		FuncName: "SetOldPassword",
		Args:     []interface{}{"old"},
	})
	c.Check(remoteSet, jc.DeepEquals, testing.StubCall{
		FuncName: "SetPassword",
		Args:     []interface{}{names.NewApplicationTag("omg"), chosePassword},
	})
}
