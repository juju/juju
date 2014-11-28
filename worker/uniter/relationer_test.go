// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"strconv"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	ft "github.com/juju/testing/filetesting"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v4/hooks"

	"github.com/juju/juju/api"
	apiuniter "github.com/juju/juju/api/uniter"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/uniter"
	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/relation"
)

type RelationerSuite struct {
	jujutesting.JujuConnSuite
	hooks   chan hook.Info
	svc     *state.Service
	rel     *state.Relation
	dir     *relation.StateDir
	dirPath string

	st         *api.State
	uniter     *apiuniter.State
	apiRelUnit *apiuniter.RelationUnit
}

var _ = gc.Suite(&RelationerSuite{})

func (s *RelationerSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	var err error
	s.svc = s.AddTestingService(c, "u", s.AddTestingCharm(c, "riak"))
	c.Assert(err, jc.ErrorIsNil)
	rels, err := s.svc.Relations()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rels, gc.HasLen, 1)
	s.rel = rels[0]
	_, unit := s.AddRelationUnit(c, "u/0")
	s.dirPath = c.MkDir()
	s.dir, err = relation.ReadStateDir(s.dirPath, s.rel.Id())
	c.Assert(err, jc.ErrorIsNil)
	s.hooks = make(chan hook.Info)

	password, err := utils.RandomPassword()
	c.Assert(err, jc.ErrorIsNil)
	err = unit.SetPassword(password)
	c.Assert(err, jc.ErrorIsNil)
	s.st = s.OpenAPIAs(c, unit.Tag(), password)
	s.uniter, err = s.st.Uniter()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.uniter, gc.NotNil)

	apiUnit, err := s.uniter.Unit(unit.Tag().(names.UnitTag))
	c.Assert(err, jc.ErrorIsNil)
	apiRel, err := s.uniter.Relation(s.rel.Tag().(names.RelationTag))
	c.Assert(err, jc.ErrorIsNil)
	s.apiRelUnit, err = apiRel.Unit(apiUnit)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *RelationerSuite) AddRelationUnit(c *gc.C, name string) (*state.RelationUnit, *state.Unit) {
	u, err := s.svc.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(u.Name(), gc.Equals, name)
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = u.AssignToMachine(machine)
	c.Assert(err, jc.ErrorIsNil)
	privateAddr := network.NewAddress(
		strings.Replace(name, "/", "-", 1)+".testing.invalid", network.ScopeCloudLocal)
	err = machine.SetAddresses(privateAddr)
	c.Assert(err, jc.ErrorIsNil)
	ru, err := s.rel.Unit(u)
	c.Assert(err, jc.ErrorIsNil)
	return ru, u
}

func (s *RelationerSuite) TestStateDir(c *gc.C) {
	// Create the relationer; check its state dir is not created.
	r := uniter.NewRelationer(s.apiRelUnit, s.dir, s.hooks)
	path := strconv.Itoa(s.rel.Id())
	ft.Removed{path}.Check(c, s.dirPath)

	// Join the relation; check the dir was created.
	err := r.Join()
	c.Assert(err, jc.ErrorIsNil)
	ft.Dir{path, 0755}.Check(c, s.dirPath)

	// Prepare to depart the relation; check the dir is still there.
	hi := hook.Info{Kind: hooks.RelationBroken}
	_, err = r.PrepareHook(hi)
	c.Assert(err, jc.ErrorIsNil)
	ft.Dir{path, 0755}.Check(c, s.dirPath)

	// Actually depart it; check the dir is removed.
	err = r.CommitHook(hi)
	c.Assert(err, jc.ErrorIsNil)
	ft.Removed{path}.Check(c, s.dirPath)
}

func (s *RelationerSuite) TestEnterLeaveScope(c *gc.C) {
	ru1, _ := s.AddRelationUnit(c, "u/1")
	r := uniter.NewRelationer(s.apiRelUnit, s.dir, s.hooks)

	// u/1 does not consider u/0 to be alive.
	w := ru1.Watch()
	defer stop(c, w)
	s.State.StartSync()
	ch, ok := <-w.Changes()
	c.Assert(ok, jc.IsTrue)
	c.Assert(ch.Changed, gc.HasLen, 0)
	c.Assert(ch.Departed, gc.HasLen, 0)

	// u/0 enters scope; u/1 observes it.
	err := r.Join()
	c.Assert(err, jc.ErrorIsNil)
	s.State.StartSync()
	select {
	case ch, ok := <-w.Changes():
		c.Assert(ok, jc.IsTrue)
		c.Assert(ch.Changed, gc.HasLen, 1)
		_, found := ch.Changed["u/0"]
		c.Assert(found, jc.IsTrue)
		c.Assert(ch.Departed, gc.HasLen, 0)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for presence detection")
	}

	// re-Join is no-op.
	err = r.Join()
	c.Assert(err, jc.ErrorIsNil)
	// TODO(jam): This would be a great to replace with statetesting.NotifyWatcherC
	s.State.StartSync()
	select {
	case ch, ok := <-w.Changes():
		c.Fatalf("got unexpected change: %#v, %#v", ch, ok)
	case <-time.After(coretesting.ShortWait):
	}

	// u/0 leaves scope; u/1 observes it.
	hi := hook.Info{Kind: hooks.RelationBroken}
	_, err = r.PrepareHook(hi)
	c.Assert(err, jc.ErrorIsNil)

	err = r.CommitHook(hi)
	c.Assert(err, jc.ErrorIsNil)
	s.State.StartSync()
	select {
	case ch, ok := <-w.Changes():
		c.Assert(ok, jc.IsTrue)
		c.Assert(ch.Changed, gc.HasLen, 0)
		c.Assert(ch.Departed, gc.DeepEquals, []string{"u/0"})
	case <-time.After(worstCase):
		c.Fatalf("timed out waiting for absence detection")
	}
}

func (s *RelationerSuite) TestStartStopHooks(c *gc.C) {
	ru1, _ := s.AddRelationUnit(c, "u/1")
	ru2, _ := s.AddRelationUnit(c, "u/2")
	r := uniter.NewRelationer(s.apiRelUnit, s.dir, s.hooks)
	c.Assert(r.IsImplicit(), jc.IsFalse)
	err := r.Join()
	c.Assert(err, jc.ErrorIsNil)

	// Check no hooks are being sent.
	s.assertNoHook(c)

	// Start hooks, and check that still no changes are sent.
	r.StartHooks()
	defer stopHooks(c, r)
	s.assertNoHook(c)

	// Check we can't start hooks again.
	f := func() { r.StartHooks() }
	c.Assert(f, gc.PanicMatches, "hooks already started!")

	// Join u/1 to the relation, and check that we receive the expected hooks.
	settings := map[string]interface{}{"unit": "settings"}
	err = ru1.EnterScope(settings)
	c.Assert(err, jc.ErrorIsNil)
	s.assertHook(c, hook.Info{
		Kind:       hooks.RelationJoined,
		RemoteUnit: "u/1",
	})
	s.assertHook(c, hook.Info{
		Kind:       hooks.RelationChanged,
		RemoteUnit: "u/1",
	})
	s.assertNoHook(c)

	// Stop hooks, make more changes, check no events.
	err = r.StopHooks()
	c.Assert(err, jc.ErrorIsNil)
	err = ru1.LeaveScope()
	c.Assert(err, jc.ErrorIsNil)
	err = ru2.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	node, err := ru2.Settings()
	c.Assert(err, jc.ErrorIsNil)
	node.Set("private-address", "roehampton")
	_, err = node.Write()
	c.Assert(err, jc.ErrorIsNil)
	s.assertNoHook(c)

	// Stop hooks again to verify safety.
	err = r.StopHooks()
	c.Assert(err, jc.ErrorIsNil)
	s.assertNoHook(c)

	// Start them again, and check we get the expected events sent.
	r.StartHooks()
	defer stopHooks(c, r)
	s.assertHook(c, hook.Info{
		Kind:       hooks.RelationDeparted,
		RemoteUnit: "u/1",
	})
	s.assertHook(c, hook.Info{
		Kind:          hooks.RelationJoined,
		ChangeVersion: 1,
		RemoteUnit:    "u/2",
	})
	s.assertHook(c, hook.Info{
		Kind:          hooks.RelationChanged,
		ChangeVersion: 1,
		RemoteUnit:    "u/2",
	})
	s.assertNoHook(c)

	// Stop them again, just to be sure.
	err = r.StopHooks()
	c.Assert(err, jc.ErrorIsNil)
	s.assertNoHook(c)
}

func (s *RelationerSuite) TestPrepareCommitHooks(c *gc.C) {
	r := uniter.NewRelationer(s.apiRelUnit, s.dir, s.hooks)
	err := r.Join()
	c.Assert(err, jc.ErrorIsNil)

	assertMembers := func(expect map[string]int64) {
		c.Assert(s.dir.State().Members, jc.DeepEquals, expect)
		expectNames := make([]string, 0, len(expect))
		for name := range expect {
			expectNames = append(expectNames, name)
		}
		c.Assert(r.ContextInfo().MemberNames, jc.SameContents, expectNames)
	}
	assertMembers(map[string]int64{})

	// Check preparing an invalid hook changes nothing.
	changed := hook.Info{
		Kind:          hooks.RelationChanged,
		RemoteUnit:    "u/1",
		ChangeVersion: 7,
	}
	_, err = r.PrepareHook(changed)
	c.Assert(err, gc.ErrorMatches, `inappropriate "relation-changed" for "u/1": unit has not joined`)
	assertMembers(map[string]int64{})

	// Check preparing a valid hook updates neither the context nor persistent
	// relation state.
	joined := hook.Info{
		Kind:       hooks.RelationJoined,
		RemoteUnit: "u/1",
	}
	name, err := r.PrepareHook(joined)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(name, gc.Equals, "ring-relation-joined")
	assertMembers(map[string]int64{})

	// Check that preparing the following hook fails as before...
	_, err = r.PrepareHook(changed)
	c.Assert(err, gc.ErrorMatches, `inappropriate "relation-changed" for "u/1": unit has not joined`)
	assertMembers(map[string]int64{})

	// ...but that committing the previous hook updates the persistent
	// relation state...
	err = r.CommitHook(joined)
	c.Assert(err, jc.ErrorIsNil)
	assertMembers(map[string]int64{"u/1": 0})

	// ...and allows us to prepare the next hook...
	name, err = r.PrepareHook(changed)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(name, gc.Equals, "ring-relation-changed")
	assertMembers(map[string]int64{"u/1": 0})

	// ...and commit it.
	err = r.CommitHook(changed)
	c.Assert(err, jc.ErrorIsNil)
	assertMembers(map[string]int64{"u/1": 7})

	// To verify implied behaviour above, prepare a new joined hook with
	// missing membership information, and check relation context
	// membership is stil not updated...
	joined.RemoteUnit = "u/2"
	joined.ChangeVersion = 3
	name, err = r.PrepareHook(joined)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(name, gc.Equals, "ring-relation-joined")
	assertMembers(map[string]int64{"u/1": 7})

	// ...until commit, at which point so is relation state.
	err = r.CommitHook(joined)
	c.Assert(err, jc.ErrorIsNil)
	assertMembers(map[string]int64{"u/1": 7, "u/2": 3})
}

func (s *RelationerSuite) TestSetDying(c *gc.C) {
	ru1, _ := s.AddRelationUnit(c, "u/1")
	settings := map[string]interface{}{"unit": "settings"}
	err := ru1.EnterScope(settings)
	c.Assert(err, jc.ErrorIsNil)
	r := uniter.NewRelationer(s.apiRelUnit, s.dir, s.hooks)
	err = r.Join()
	c.Assert(err, jc.ErrorIsNil)
	r.StartHooks()
	defer stopHooks(c, r)
	s.assertHook(c, hook.Info{
		Kind:       hooks.RelationJoined,
		RemoteUnit: "u/1",
	})

	// While a changed hook is still pending, the relation (or possibly the unit,
	// pending lifecycle work), changes Life to Dying, and the relationer is
	// informed.
	err = r.SetDying()
	c.Assert(err, jc.ErrorIsNil)

	// Check that we cannot rejoin the relation.
	f := func() { r.Join() }
	c.Assert(f, gc.PanicMatches, "dying relationer must not join!")

	// ...but the hook stream continues, sending the required changed hook for
	// u/1 before moving on to a departed, despite the fact that its pinger is
	// still running, and closing with a broken.
	s.assertHook(c, hook.Info{Kind: hooks.RelationChanged, RemoteUnit: "u/1"})
	s.assertHook(c, hook.Info{Kind: hooks.RelationDeparted, RemoteUnit: "u/1"})
	s.assertHook(c, hook.Info{Kind: hooks.RelationBroken})

	// Check that the relation state has been broken.
	err = s.dir.State().Validate(hook.Info{Kind: hooks.RelationBroken})
	c.Assert(err, gc.ErrorMatches, ".*: relation is broken and cannot be changed further")
}

func (s *RelationerSuite) assertNoHook(c *gc.C) {
	s.BackingState.StartSync()
	select {
	case hi, ok := <-s.hooks:
		c.Fatalf("got unexpected hook info %#v (%t)", hi, ok)
	case <-time.After(coretesting.ShortWait):
	}
}

func (s *RelationerSuite) assertHook(c *gc.C, expect hook.Info) {
	s.BackingState.StartSync()
	// We must ensure the local state dir exists first.
	c.Assert(s.dir.Ensure(), gc.IsNil)
	select {
	case hi, ok := <-s.hooks:
		c.Assert(ok, jc.IsTrue)
		expect.ChangeVersion = hi.ChangeVersion
		c.Assert(hi, gc.DeepEquals, expect)
		c.Assert(s.dir.Write(hi), gc.Equals, nil)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for %#v", expect)
	}
}

func (s *RelationerSuite) TestInferRemoteUnitInvalidInput(c *gc.C) {
	s.AddRelationUnit(c, "u/1")
	r := uniter.NewRelationer(s.apiRelUnit, s.dir, s.hooks)
	c.Assert(r.IsImplicit(), jc.IsFalse)
	err := r.Join()
	c.Assert(err, jc.ErrorIsNil)

	err = r.CommitHook(hook.Info{
		RelationId: 0,
		Kind:       hooks.RelationJoined,
		RemoteUnit: "u/0",
	})
	c.Assert(err, jc.ErrorIsNil)

	args := uniter.RunCommandsArgs{
		Commands:       "some-command",
		RelationId:     0,
		RemoteUnitName: "my-bad-remote-unit",
	}

	relationers := map[int]*uniter.Relationer{}
	relationers[0] = r

	// Bad remote unit
	remoteUnit, err := uniter.InferRemoteUnit(relationers, args)
	c.Assert(err, gc.ErrorMatches, "no remote unit found:.*, override to execute command")

	// Good remote unit
	args.RemoteUnitName = "u/0"
	remoteUnit, err = uniter.InferRemoteUnit(relationers, args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(remoteUnit, gc.Equals, "u/0")
}

func (s *RelationerSuite) TestInferRemoteUnitAmbiguous(c *gc.C) {
	s.AddRelationUnit(c, "u/1")
	s.AddRelationUnit(c, "u/2")
	r := uniter.NewRelationer(s.apiRelUnit, s.dir, s.hooks)
	c.Assert(r.IsImplicit(), jc.IsFalse)
	err := r.Join()
	c.Assert(err, jc.ErrorIsNil)

	err = r.CommitHook(hook.Info{
		RelationId: 0,
		Kind:       hooks.RelationJoined,
		RemoteUnit: "u/0",
	})
	c.Assert(err, jc.ErrorIsNil)

	err = r.CommitHook(hook.Info{
		RelationId: 0,
		Kind:       hooks.RelationJoined,
		RemoteUnit: "u/1",
	})
	c.Assert(err, jc.ErrorIsNil)

	relationers := map[int]*uniter.Relationer{}
	relationers[0] = r

	args := uniter.RunCommandsArgs{
		Commands:       "some-command",
		RelationId:     0,
		RemoteUnitName: "",
	}

	// Ambiguous
	_, err = uniter.InferRemoteUnit(relationers, args)
	c.Assert(err, gc.ErrorMatches, `unable to determine remote-unit, please disambiguate:.*`)

	// Disambiguate
	args.RemoteUnitName = "u/0"
	remoteUnit, err := uniter.InferRemoteUnit(relationers, args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(remoteUnit, gc.Equals, "u/0")
}

func (s *RelationerSuite) TestInferRemoteUnitMissingRelation(c *gc.C) {
	relationers := map[int]*uniter.Relationer{}
	args := uniter.RunCommandsArgs{
		Commands:       "some-command",
		RelationId:     -1,
		RemoteUnitName: "remote/0",
	}

	_, err := uniter.InferRemoteUnit(relationers, args)
	c.Assert(err, gc.ErrorMatches, "remote unit: remote/0, provided without a relation")
}

func (s *RelationerSuite) TestInferRemoteValidEmptyForce(c *gc.C) {
	relationers := map[int]*uniter.Relationer{}

	r := uniter.NewRelationer(s.apiRelUnit, s.dir, s.hooks)
	err := r.Join()
	c.Assert(err, jc.ErrorIsNil)

	relationers[0] = r

	args := uniter.RunCommandsArgs{
		Commands:        "some-command",
		RelationId:      0,
		RemoteUnitName:  "",
		ForceRemoteUnit: true,
	}

	// Test with valid RemoteUnit
	joined := hook.Info{
		Kind:       hooks.RelationJoined,
		RemoteUnit: "u/1",
	}
	name, err := r.PrepareHook(joined)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(name, gc.Equals, "ring-relation-joined")

	err = r.CommitHook(joined)
	c.Assert(err, jc.ErrorIsNil)

	remoteUnit, err := uniter.InferRemoteUnit(relationers, args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(remoteUnit, gc.Equals, "")

	// Invalid remote-unit should still fail even when forcing
	args.RemoteUnitName = "invalid-remote-unit"
	_, err = uniter.InferRemoteUnit(relationers, args)
	c.Assert(err, gc.ErrorMatches, `"invalid-remote-unit" is not a valid remote unit name`)
}

func (s *RelationerSuite) TestInferRemoteValidDepartedForce(c *gc.C) {
	relationers := map[int]*uniter.Relationer{}

	r := uniter.NewRelationer(s.apiRelUnit, s.dir, s.hooks)
	err := r.Join()
	c.Assert(err, jc.ErrorIsNil)

	relationers[0] = r

	args := uniter.RunCommandsArgs{
		Commands:        "some-command",
		RelationId:      0,
		RemoteUnitName:  "departed/0",
		ForceRemoteUnit: true,
	}

	// Test with valid RemoteUnit
	joined := hook.Info{
		Kind:       hooks.RelationJoined,
		RemoteUnit: "u/1",
	}
	name, err := r.PrepareHook(joined)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(name, gc.Equals, "ring-relation-joined")

	err = r.CommitHook(joined)
	c.Assert(err, jc.ErrorIsNil)

	remoteUnit, err := uniter.InferRemoteUnit(relationers, args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(remoteUnit, gc.Equals, "departed/0")
}

func (s *RelationerSuite) TestInferRemoteUnit(c *gc.C) {
	relationers := map[int]*uniter.Relationer{}

	r := uniter.NewRelationer(s.apiRelUnit, s.dir, s.hooks)
	err := r.Join()
	c.Assert(err, jc.ErrorIsNil)

	relationers[0] = r

	args := uniter.RunCommandsArgs{
		Commands:       "some-command",
		RelationId:     0,
		RemoteUnitName: "",
	}

	// Test with valid RemoteUnit
	joined := hook.Info{
		Kind:       hooks.RelationJoined,
		RemoteUnit: "u/1",
	}
	name, err := r.PrepareHook(joined)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(name, gc.Equals, "ring-relation-joined")

	err = r.CommitHook(joined)
	c.Assert(err, jc.ErrorIsNil)

	remoteUnit, err := uniter.InferRemoteUnit(relationers, args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(remoteUnit, gc.Equals, "u/1")

	// Test with valid departed RemoteUnit
	// We commit the RelationChanged and RelationDeparted hooks
	// this gives us a valid relation with no remote unit.
	changed := hook.Info{
		Kind:       hooks.RelationChanged,
		RemoteUnit: "u/1",
	}
	name, err = r.PrepareHook(changed)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(name, gc.Equals, "ring-relation-changed")

	err = r.CommitHook(changed)
	c.Assert(err, jc.ErrorIsNil)

	departed := hook.Info{
		Kind:       hooks.RelationDeparted,
		RemoteUnit: "u/1",
	}
	name, err = r.PrepareHook(departed)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(name, gc.Equals, "ring-relation-departed")

	err = r.CommitHook(departed)
	c.Assert(err, jc.ErrorIsNil)

	// Ensure the call with the same args fails, explicitly warning
	// the user about the lack of a remote unit.
	_, err = uniter.InferRemoteUnit(relationers, args)
	c.Assert(err, gc.ErrorMatches, "no remote unit found for relation id: 0, override to execute commands")

	// Now with ForceRemoteUnitCheck flag and a manual remoteUnit set
	// we validate the remoteUnit is well formed, but skip membership / existence validation.
	args.ForceRemoteUnit = true
	args.RemoteUnitName = "u/1"
	remoteUnit, err = uniter.InferRemoteUnit(relationers, args)
	c.Assert(remoteUnit, gc.Equals, "u/1")
}

type stopper interface {
	Stop() error
}

func stop(c *gc.C, s stopper) {
	c.Assert(s.Stop(), gc.IsNil)
}

func stopHooks(c *gc.C, r *uniter.Relationer) {
	c.Assert(r.StopHooks(), gc.IsNil)
}

type RelationerImplicitSuite struct {
	jujutesting.JujuConnSuite
}

var _ = gc.Suite(&RelationerImplicitSuite{})

func (s *RelationerImplicitSuite) TestImplicitRelationer(c *gc.C) {
	// Create a relationer for an implicit endpoint (mysql:juju-info).
	mysql := s.AddTestingService(c, "mysql", s.AddTestingCharm(c, "mysql"))
	u, err := mysql.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = u.AssignToMachine(machine)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetAddresses(network.NewAddress("blah", network.ScopeCloudLocal))
	c.Assert(err, jc.ErrorIsNil)
	s.AddTestingService(c, "logging", s.AddTestingCharm(c, "logging"))
	eps, err := s.State.InferEndpoints("logging", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	relsDir := c.MkDir()
	dir, err := relation.ReadStateDir(relsDir, rel.Id())
	c.Assert(err, jc.ErrorIsNil)
	hooks := make(chan hook.Info)

	password, err := utils.RandomPassword()
	c.Assert(err, jc.ErrorIsNil)
	err = u.SetPassword(password)
	c.Assert(err, jc.ErrorIsNil)
	st := s.OpenAPIAs(c, u.Tag(), password)
	uniterState, err := st.Uniter()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(uniterState, gc.NotNil)

	apiUnit, err := uniterState.Unit(u.Tag().(names.UnitTag))
	c.Assert(err, jc.ErrorIsNil)
	apiRel, err := uniterState.Relation(rel.Tag().(names.RelationTag))
	c.Assert(err, jc.ErrorIsNil)
	apiRelUnit, err := apiRel.Unit(apiUnit)
	c.Assert(err, jc.ErrorIsNil)

	r := uniter.NewRelationer(apiRelUnit, dir, hooks)
	c.Assert(r, jc.Satisfies, (*uniter.Relationer).IsImplicit)

	// Join the relation.
	err = r.Join()
	c.Assert(err, jc.ErrorIsNil)
	sub, err := s.State.Unit("logging/0")
	c.Assert(err, jc.ErrorIsNil)

	// Join the other side; check no hooks are sent.
	r.StartHooks()
	defer func() { c.Assert(r.StopHooks(), gc.IsNil) }()
	subru, err := rel.Unit(sub)
	c.Assert(err, jc.ErrorIsNil)
	err = subru.EnterScope(map[string]interface{}{"some": "data"})
	c.Assert(err, jc.ErrorIsNil)
	s.State.StartSync()
	select {
	case <-time.After(coretesting.ShortWait):
	case <-hooks:
		c.Fatalf("unexpected hook generated")
	}

	// Set it to Dying; check that the dir is removed immediately.
	err = r.SetDying()
	c.Assert(err, jc.ErrorIsNil)
	path := strconv.Itoa(rel.Id())
	ft.Removed{path}.Check(c, relsDir)

	// Check that it left scope, by leaving scope on the other side and destroying
	// the relation.
	err = subru.LeaveScope()
	c.Assert(err, jc.ErrorIsNil)
	err = rel.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = rel.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	// Verify that no other hooks were sent at any stage.
	select {
	case <-hooks:
		c.Fatalf("unexpected hook generated")
	default:
	}
}
