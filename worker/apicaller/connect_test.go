// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apicaller_test

import (
	"github.com/juju/names"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
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

func (s *ScaryConnectSuite) TestCannotSetModelTag(c *gc.C) {
	// success case; just a little failure we don't mind
	c.Fatalf("xxx")
}

func (s *ScaryConnectSuite) TestEntityAlive(c *gc.C) {
	stub := &testing.Stub{}
	stub.SetErrors(
		nil, // apiOpen
		nil, // Entity
		nil, // remote SetPassword to orig value
	)

	expectConn := &mockConn{stub: stub}
	apiOpen := func(info *api.Info, opts api.DialOpts) (api.Connection, error) {
		stub.AddCall("apiOpen", *info, opts)
		if err := stub.NextErr(); err != nil {
			return nil, err
		}
		return expectConn, nil
	}

	// to make the point that this code should be entity-agnostic,
	// use an entity that doesn't correspond to an agent at all.
	entity := names.NewServiceTag("omg")
	model := coretesting.ModelTag
	conn, err := apicaller.ScaryConnect(&mockAgent{
		stub:   stub,
		model:  model,
		entity: entity,
	}, apiOpen)
	c.Check(conn, gc.Equals, expectConn)
	c.Check(err, jc.ErrorIsNil)

	connCalls := []testing.StubCall{{
		FuncName: "Entity",
		Args:     []interface{}{entity},
	}, {
		FuncName: "SetPassword",
		Args:     []interface{}{"new"},
	}}
	calls := append(openCalls(model, entity, "new"), connCalls...)
	stub.CheckCalls(c, calls)
}

func (s *ScaryConnectSuite) TestEntityDying(c *gc.C) {
	// success case
	c.Fatalf("xxx")
}

func (s *ScaryConnectSuite) TestEntityDead(c *gc.C) {
	// permanent failure case
	c.Fatalf("xxx")
}

func (s *ScaryConnectSuite) TestEntityRemoved(c *gc.C) {
	// permanent failure case
	c.Fatalf("xxx")
}

func (s *ScaryConnectSuite) TestEntityUnknownLife(c *gc.C) {
	// "random" failure case
	c.Fatalf("xxx")
}

func (s *ScaryConnectSuite) TestChangePasswordLocalError(c *gc.C) {
	// "random" failure case
	c.Fatalf("xxx")
}

func (s *ScaryConnectSuite) TestChangePasswordRemoteError(c *gc.C) {
	// "random" failure case
	c.Fatalf("xxx")
}

func (s *ScaryConnectSuite) TestChangePasswordSuccess(c *gc.C) {
	// retry-please failure case
	c.Fatalf("xxx")
}
