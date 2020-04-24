// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apicaller_test

import (
	"errors"

	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	apiagent "github.com/juju/juju/api/agent"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/apicaller"
)

// ScaryConnectSuite should cover all the *lines* where we get a connection
// without triggering the checkProvisionedStrategy ugliness. It tests the
// various conditions in isolation; it's possible that some real scenarios
// may trigger more than one of these, but it's impractical to test *every*
// possible *path*.
type ScaryConnectSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ScaryConnectSuite{})

func (*ScaryConnectSuite) TestEntityAlive(c *gc.C) {
	testEntityFine(c, apiagent.Alive)
}

func (*ScaryConnectSuite) TestEntityDying(c *gc.C) {
	testEntityFine(c, apiagent.Dying)
}

func testEntityFine(c *gc.C, life apiagent.Life) {
	stub := &testing.Stub{}
	expectConn := &mockConn{stub: stub}
	apiOpen := func(info *api.Info, opts api.DialOpts) (api.Connection, error) {
		// no apiOpen stub calls necessary in this suite; covered
		// by RetrySuite, just an extra complication here.
		return expectConn, nil
	}

	// to make the point that this code should be entity-agnostic,
	// use an entity that doesn't correspond to an agent at all.
	entity := names.NewApplicationTag("omg")
	connect := func() (api.Connection, error) {
		return apicaller.ScaryConnect(&mockAgent{
			stub:   stub,
			model:  coretesting.ModelTag,
			entity: entity,
		}, apiOpen, loggo.GetLogger("test"))
	}

	conn, err := lifeTest(c, stub, apiagent.Alive, connect)
	c.Check(conn, gc.Equals, expectConn)
	c.Check(err, jc.ErrorIsNil)
	stub.CheckCalls(c, []testing.StubCall{{
		FuncName: "Life",
		Args:     []interface{}{entity},
	}, {
		FuncName: "SetPassword",
		Args:     []interface{}{entity, "new"},
	}})
}

func (*ScaryConnectSuite) TestEntityDead(c *gc.C) {
	// permanent failure case
	stub := &testing.Stub{}
	expectConn := &mockConn{stub: stub}
	apiOpen := func(info *api.Info, opts api.DialOpts) (api.Connection, error) {
		return expectConn, nil
	}

	entity := names.NewApplicationTag("omg")
	connect := func() (api.Connection, error) {
		return apicaller.ScaryConnect(&mockAgent{
			stub:   stub,
			model:  coretesting.ModelTag,
			entity: entity,
		}, apiOpen, loggo.GetLogger("test"))
	}

	conn, err := lifeTest(c, stub, apiagent.Dead, connect)
	c.Check(conn, gc.IsNil)
	c.Check(err, gc.Equals, apicaller.ErrConnectImpossible)
	stub.CheckCalls(c, []testing.StubCall{{
		FuncName: "Life",
		Args:     []interface{}{entity},
	}, {
		FuncName: "Close",
	}})
}

func (*ScaryConnectSuite) TestEntityDenied(c *gc.C) {
	// permanent failure case
	stub := &testing.Stub{}
	stub.SetErrors(apiagent.ErrDenied)
	expectConn := &mockConn{stub: stub}
	apiOpen := func(info *api.Info, opts api.DialOpts) (api.Connection, error) {
		return expectConn, nil
	}

	entity := names.NewApplicationTag("omg")
	connect := func() (api.Connection, error) {
		return apicaller.ScaryConnect(&mockAgent{
			stub:   stub,
			model:  coretesting.ModelTag,
			entity: entity,
		}, apiOpen, loggo.GetLogger("test"))
	}

	conn, err := lifeTest(c, stub, apiagent.Dead, connect)
	c.Check(conn, gc.IsNil)
	c.Check(err, gc.Equals, apicaller.ErrConnectImpossible)
	stub.CheckCalls(c, []testing.StubCall{{
		FuncName: "Life",
		Args:     []interface{}{entity},
	}, {
		FuncName: "Close",
	}})
}

func (*ScaryConnectSuite) TestEntityUnknownLife(c *gc.C) {
	// "random" failure case
	stub := &testing.Stub{}
	expectConn := &mockConn{stub: stub}
	apiOpen := func(info *api.Info, opts api.DialOpts) (api.Connection, error) {
		return expectConn, nil
	}

	entity := names.NewApplicationTag("omg")
	connect := func() (api.Connection, error) {
		return apicaller.ScaryConnect(&mockAgent{
			stub:   stub,
			model:  coretesting.ModelTag,
			entity: entity,
		}, apiOpen, loggo.GetLogger("test"))
	}

	conn, err := lifeTest(c, stub, apiagent.Life("zombie"), connect)
	c.Check(conn, gc.IsNil)
	c.Check(err, gc.ErrorMatches, `unknown life value "zombie"`)
	stub.CheckCalls(c, []testing.StubCall{{
		FuncName: "Life",
		Args:     []interface{}{entity},
	}, {
		FuncName: "Close",
	}})
}

func (*ScaryConnectSuite) TestChangePasswordConfigError(c *gc.C) {
	// "random" failure case
	stub := createUnauthorisedStub(nil, errors.New("zap"))
	err := checkChangePassword(c, stub)
	c.Check(err, gc.ErrorMatches, "zap")
	stub.CheckCallNames(c,
		"Life", "ChangeConfig",
		"Close",
	)
}

func (*ScaryConnectSuite) TestChangePasswordRemoteError(c *gc.C) {
	// "random" failure case
	stub := createUnauthorisedStub(nil, nil, nil, nil, errors.New("pow"))
	err := checkChangePassword(c, stub)
	c.Check(err, gc.ErrorMatches, "pow")
	stub.CheckCallNames(c,
		"Life", "ChangeConfig",
		// Be careful, these are two different SetPassword receivers.
		"SetPassword", "SetOldPassword", "SetPassword",
		"Close",
	)
	checkSaneChange(c, stub.Calls()[2:5])
}

func (*ScaryConnectSuite) TestChangePasswordRemoteDenied(c *gc.C) {
	// permanent failure case
	stub := createUnauthorisedStub(nil, nil, nil, nil, apiagent.ErrDenied)
	err := checkChangePassword(c, stub)
	c.Check(err, gc.Equals, apicaller.ErrConnectImpossible)
	stub.CheckCallNames(c,
		"Life", "ChangeConfig",
		// Be careful, these are two different SetPassword receivers.
		"SetPassword", "SetOldPassword", "SetPassword",
		"Close",
	)
	checkSaneChange(c, stub.Calls()[2:5])
}

func (s *ScaryConnectSuite) TestChangePasswordSuccessAfterUnauthorisedError(c *gc.C) {
	// This will try to login with old password if current one fails.
	stub := createUnauthorisedStub()
	s.assertChangePasswordSuccess(c, stub)
}

func (s *ScaryConnectSuite) TestChangePasswordSuccessAfterBadCurrentPasswordError(c *gc.C) {
	// This will try to login with old password if current one fails.
	stub := createPasswordCheckStub(common.ErrBadCreds)
	s.assertChangePasswordSuccess(c, stub)
}

func (*ScaryConnectSuite) assertChangePasswordSuccess(c *gc.C, stub *testing.Stub) {
	err := checkChangePassword(c, stub)
	c.Check(err, gc.Equals, apicaller.ErrChangedPassword)
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

func checkChangePassword(c *gc.C, stub *testing.Stub) error {
	// We prepend the unauth/success pair that triggers password
	// change, and consume them in apiOpen below...
	//errUnauth := &params.Error{Code: params.CodeUnauthorized}
	//allErrs := append([]error{errUnauth, nil}, errs...)
	//
	//stub := &testing.Stub{}
	//stub.SetErrors(allErrs...)
	expectConn := &mockConn{stub: stub}
	apiOpen := func(info *api.Info, opts api.DialOpts) (api.Connection, error) {
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
		return apicaller.ScaryConnect(&mockAgent{
			stub:   stub,
			model:  coretesting.ModelTag,
			entity: entity,
		}, apiOpen, loggo.GetLogger("test"))
	}

	conn, err := lifeTest(c, stub, apiagent.Alive, connect)
	c.Check(conn, gc.IsNil)
	return err
}

func checkSaneChange(c *gc.C, calls []testing.StubCall) {
	c.Assert(calls, gc.HasLen, 3)
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
