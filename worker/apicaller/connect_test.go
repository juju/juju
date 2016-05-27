// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apicaller_test

import (
	"errors"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	apiagent "github.com/juju/juju/api/agent"
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
		}, apiOpen)
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

func (*ScaryConnectSuite) TestModelTagCannotChangeConfig(c *gc.C) {
	stub := checkModelTagUpdate(c, errors.New("oh noes"))
	stub.CheckCallNames(c,
		"ChangeConfig",
		"Life", "SetPassword",
	)
}

func (*ScaryConnectSuite) TestModelTagCannotGetTag(c *gc.C) {
	stub := checkModelTagUpdate(c, nil, errors.New("oh noes"))
	stub.CheckCallNames(c,
		"ChangeConfig", "ModelTag",
		"Life", "SetPassword",
	)
}

func (*ScaryConnectSuite) TestModelTagCannotMigrate(c *gc.C) {
	stub := checkModelTagUpdate(c, nil, nil, errors.New("oh noes"))
	stub.CheckCallNames(c,
		"ChangeConfig", "ModelTag", "Migrate",
		"Life", "SetPassword",
	)
	c.Check(stub.Calls()[2].Args, jc.DeepEquals, []interface{}{
		agent.MigrateParams{Model: coretesting.ModelTag},
	})
}

func (*ScaryConnectSuite) TestModelTagSuccess(c *gc.C) {
	stub := checkModelTagUpdate(c)
	stub.CheckCallNames(c,
		"ChangeConfig", "ModelTag", "Migrate",
		"Life", "SetPassword",
	)
	c.Check(stub.Calls()[2].Args, jc.DeepEquals, []interface{}{
		agent.MigrateParams{Model: coretesting.ModelTag},
	})
}

func checkModelTagUpdate(c *gc.C, errs ...error) *testing.Stub {
	// success case; just a little failure we don't mind, otherwise
	// equivalent to testEntityFine.
	stub := &testing.Stub{}
	stub.SetErrors(errs...) // from ChangeConfig
	expectConn := &mockConn{stub: stub}
	apiOpen := func(info *api.Info, opts api.DialOpts) (api.Connection, error) {
		return expectConn, nil
	}

	entity := names.NewApplicationTag("omg")
	connect := func() (api.Connection, error) {
		return apicaller.ScaryConnect(&mockAgent{
			stub: stub,
			// no model set; triggers ChangeConfig
			entity: entity,
		}, apiOpen)
	}
	conn, err := lifeTest(c, stub, apiagent.Alive, connect)
	c.Check(conn, gc.Equals, expectConn)
	c.Check(err, jc.ErrorIsNil)
	return stub
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
		}, apiOpen)
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
		}, apiOpen)
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
		}, apiOpen)
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
	stub, err := checkChangePassword(c, nil, errors.New("zap"))
	c.Check(err, gc.ErrorMatches, "zap")
	stub.CheckCallNames(c,
		"Life", "ChangeConfig",
		"Close",
	)
}

func (*ScaryConnectSuite) TestChangePasswordRemoteError(c *gc.C) {
	// "random" failure case
	stub, err := checkChangePassword(c,
		nil, nil, nil, nil, errors.New("pow"),
	)
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
	stub, err := checkChangePassword(c,
		nil, nil, nil, nil, apiagent.ErrDenied,
	)
	c.Check(err, gc.Equals, apicaller.ErrConnectImpossible)
	stub.CheckCallNames(c,
		"Life", "ChangeConfig",
		// Be careful, these are two different SetPassword receivers.
		"SetPassword", "SetOldPassword", "SetPassword",
		"Close",
	)
	checkSaneChange(c, stub.Calls()[2:5])
}

func (*ScaryConnectSuite) TestChangePasswordSuccess(c *gc.C) {
	// retry-please failure case
	stub, err := checkChangePassword(c)
	c.Check(err, gc.Equals, apicaller.ErrChangedPassword)
	stub.CheckCallNames(c,
		"Life", "ChangeConfig",
		// Be careful, these are two different SetPassword receivers.
		"SetPassword", "SetOldPassword", "SetPassword",
		"Close",
	)
	checkSaneChange(c, stub.Calls()[2:5])
}

func checkChangePassword(c *gc.C, errs ...error) (*testing.Stub, error) {
	// We prepend the unauth/success pair that triggers password
	// change, and consume them in apiOpen below...
	errUnauth := &params.Error{Code: params.CodeUnauthorized}
	allErrs := append([]error{errUnauth, nil}, errs...)

	stub := &testing.Stub{}
	stub.SetErrors(allErrs...)
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
		}, apiOpen)
	}

	conn, err := lifeTest(c, stub, apiagent.Alive, connect)
	c.Check(conn, gc.IsNil)
	return stub, err
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
