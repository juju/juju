// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package relation_test

import (
	stdtesting "testing"
	"time"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/charm/hooks"
	"launchpad.net/juju-core/state/api/params"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/worker/uniter/hook"
	"launchpad.net/juju-core/worker/uniter/relation"
)

func Test(t *stdtesting.T) { coretesting.MgoTestPackage(t) }

type HookQueueSuite struct{}

var _ = gc.Suite(&HookQueueSuite{})

type msi map[string]int64

type hookQueueTest struct {
	summary string
	initial *relation.State
	steps   []checker
}

func fullTest(summary string, steps ...checker) hookQueueTest {
	return hookQueueTest{summary, &relation.State{21345, nil, ""}, steps}
}

func reconcileTest(summary string, members msi, joined string, steps ...checker) hookQueueTest {
	return hookQueueTest{summary, &relation.State{21345, members, joined}, steps}
}

var aliveHookQueueTests = []hookQueueTest{
	fullTest(
		"Empty initial change causes no hooks.",
		send{nil, nil},
	), fullTest(
		"Joined and changed are both run when unit is first detected.",
		send{msi{"u/0": 0}, nil},
		expect{hooks.RelationJoined, "u/0", 0},
		expect{hooks.RelationChanged, "u/0", 0},
	), fullTest(
		"Automatic changed is run with latest settings.",
		send{msi{"u/0": 0}, nil},
		expect{hooks.RelationJoined, "u/0", 0},
		send{msi{"u/0": 7}, nil},
		expect{hooks.RelationChanged, "u/0", 7},
	), fullTest(
		"Joined is also run with latest settings.",
		send{msi{"u/0": 0}, nil},
		send{msi{"u/0": 7}, nil},
		expect{hooks.RelationJoined, "u/0", 7},
		expect{hooks.RelationChanged, "u/0", 7},
	), fullTest(
		"Nothing happens if a unit departs before its joined is run.",
		send{msi{"u/0": 0}, nil},
		send{msi{"u/0": 7}, nil},
		send{nil, []string{"u/0"}},
	), fullTest(
		"A changed is run after a joined, even if a departed is known.",
		send{msi{"u/0": 0}, nil},
		expect{hooks.RelationJoined, "u/0", 0},
		send{nil, []string{"u/0"}},
		expect{hooks.RelationChanged, "u/0", 0},
		expect{hooks.RelationDeparted, "u/0", 0},
	), fullTest(
		"A departed replaces a changed.",
		send{msi{"u/0": 0}, nil},
		advance{2},
		send{msi{"u/0": 7}, nil},
		send{nil, []string{"u/0"}},
		expect{hooks.RelationDeparted, "u/0", 7},
	), fullTest(
		"Changed events are ignored if the version has not changed.",
		send{msi{"u/0": 0}, nil},
		advance{2},
		send{msi{"u/0": 0}, nil},
	), fullTest(
		"Redundant changed events are elided.",
		send{msi{"u/0": 0}, nil},
		advance{2},
		send{msi{"u/0": 3}, nil},
		send{msi{"u/0": 7}, nil},
		send{msi{"u/0": 79}, nil},
		expect{hooks.RelationChanged, "u/0", 79},
	), fullTest(
		"Latest hooks are run in the original unit order.",
		send{msi{"u/0": 0, "u/1": 1}, nil},
		advance{4},
		send{msi{"u/0": 3}, nil},
		send{msi{"u/1": 7}, nil},
		send{nil, []string{"u/0"}},
		expect{hooks.RelationDeparted, "u/0", 3},
		expect{hooks.RelationChanged, "u/1", 7},
	), fullTest(
		"Test everything we can think of at the same time.",
		send{msi{"u/0": 0, "u/1": 0, "u/2": 0, "u/3": 0, "u/4": 0}, nil},
		advance{6},
		// u/0, u/1, u/2 are now up to date; u/3, u/4 are untouched.
		send{msi{"u/0": 1}, nil},
		send{msi{"u/1": 1, "u/2": 1, "u/3": 1, "u/5": 0}, []string{"u/0", "u/4"}},
		send{msi{"u/3": 2}, nil},
		// - Finish off the rest of the initial state, ignoring u/4, but using
		// the latest known settings.
		expect{hooks.RelationJoined, "u/3", 2},
		expect{hooks.RelationChanged, "u/3", 2},
		// - u/0 was queued for change by the first RUC, but this change is
		// no longer relevant; it's departed in the second RUC, so we run
		// that hook instead.
		expect{hooks.RelationDeparted, "u/0", 1},
		// - Handle the remaining changes in the second RUC, still ignoring u/4.
		// We do run new changed hooks for u/1 and u/2, because the latest settings
		// are newer than those used in their original changed events.
		expect{hooks.RelationChanged, "u/1", 1},
		expect{hooks.RelationChanged, "u/2", 1},
		expect{hooks.RelationJoined, "u/5", 0},
		expect{hooks.RelationChanged, "u/5", 0},
		// - Ignore the third RUC, because the original joined/changed on u/3
		// was executed after we got the latest settings version.
	), reconcileTest(
		"Check that matching settings versions cause no changes.",
		msi{"u/0": 0}, "",
		send{msi{"u/0": 0}, nil},
	), reconcileTest(
		"Check that new settings versions cause appropriate changes.",
		msi{"u/0": 0}, "",
		send{msi{"u/0": 1}, nil},
		expect{hooks.RelationChanged, "u/0", 1},
	), reconcileTest(
		"Check that a just-joined unit gets its changed hook run first.",
		msi{"u/0": 0}, "u/0",
		send{msi{"u/0": 0}, nil},
		expect{hooks.RelationChanged, "u/0", 0},
	), reconcileTest(
		"Check that missing units are queued for depart as early as possible.",
		msi{"u/0": 0}, "",
		send{msi{"u/1": 0}, nil},
		expect{hooks.RelationDeparted, "u/0", 0},
		expect{hooks.RelationJoined, "u/1", 0},
		expect{hooks.RelationChanged, "u/1", 0},
	), reconcileTest(
		"Double-check that a pending changed happens before an injected departed.",
		msi{"u/0": 0}, "u/0",
		send{nil, nil},
		expect{hooks.RelationChanged, "u/0", 0},
		expect{hooks.RelationDeparted, "u/0", 0},
	), reconcileTest(
		"Check that missing units don't slip in front of required changed hooks.",
		msi{"u/0": 0}, "u/0",
		send{msi{"u/1": 0}, nil},
		expect{hooks.RelationChanged, "u/0", 0},
		expect{hooks.RelationDeparted, "u/0", 0},
		expect{hooks.RelationJoined, "u/1", 0},
		expect{hooks.RelationChanged, "u/1", 0},
	),
}

func (s *HookQueueSuite) TestAliveHookQueue(c *gc.C) {
	for i, t := range aliveHookQueueTests {
		c.Logf("test %d: %s", i, t.summary)
		out := make(chan hook.Info)
		in := make(chan params.RelationUnitsChange)
		ruw := &RUW{in, false}
		q := relation.NewAliveHookQueue(t.initial, out, ruw)
		for i, step := range t.steps {
			c.Logf("  step %d", i)
			step.check(c, in, out)
		}
		expect{}.check(c, in, out)
		q.Stop()
		c.Assert(ruw.stopped, gc.Equals, true)
	}
}

var dyingHookQueueTests = []hookQueueTest{
	fullTest(
		"Empty state just gets a broken hook.",
		expect{hook: hooks.RelationBroken},
	), reconcileTest(
		"Each current member is departed before broken is sent.",
		msi{"u/1": 7, "u/4": 33}, "",
		expect{hooks.RelationDeparted, "u/1", 7},
		expect{hooks.RelationDeparted, "u/4", 33},
		expect{hook: hooks.RelationBroken},
	), reconcileTest(
		"If there's a pending changed, that must still be respected.",
		msi{"u/0": 3}, "u/0",
		expect{hooks.RelationChanged, "u/0", 3},
		expect{hooks.RelationDeparted, "u/0", 3},
		expect{hook: hooks.RelationBroken},
	),
}

func (s *HookQueueSuite) TestDyingHookQueue(c *gc.C) {
	for i, t := range dyingHookQueueTests {
		c.Logf("test %d: %s", i, t.summary)
		out := make(chan hook.Info)
		q := relation.NewDyingHookQueue(t.initial, out)
		for i, step := range t.steps {
			c.Logf("  step %d", i)
			step.check(c, nil, out)
		}
		expect{}.check(c, nil, out)
		q.Stop()
	}
}

// RUW exists entirely to send RelationUnitsChanged events to a tested
// HookQueue in a synchronous and predictable fashion.
type RUW struct {
	in      chan params.RelationUnitsChange
	stopped bool
}

func (w *RUW) Changes() <-chan params.RelationUnitsChange {
	return w.in
}

func (w *RUW) Stop() error {
	close(w.in)
	w.stopped = true
	return nil
}

func (w *RUW) Err() error {
	return nil
}

type checker interface {
	check(c *gc.C, in chan params.RelationUnitsChange, out chan hook.Info)
}

type send struct {
	changed  msi
	departed []string
}

func (d send) check(c *gc.C, in chan params.RelationUnitsChange, out chan hook.Info) {
	ruc := params.RelationUnitsChange{Changed: map[string]params.UnitSettings{}}
	for name, version := range d.changed {
		ruc.Changed[name] = params.UnitSettings{Version: version}
	}
	for _, name := range d.departed {
		ruc.Departed = append(ruc.Departed, name)
	}
	in <- ruc
}

type advance struct {
	count int
}

func (d advance) check(c *gc.C, in chan params.RelationUnitsChange, out chan hook.Info) {
	for i := 0; i < d.count; i++ {
		select {
		case <-out:
		case <-time.After(coretesting.LongWait):
			c.Fatalf("timed out waiting for event %d", i)
		}
	}
}

type expect struct {
	hook    hooks.Kind
	unit    string
	version int64
}

func (d expect) check(c *gc.C, in chan params.RelationUnitsChange, out chan hook.Info) {
	if d.hook == "" {
		select {
		case unexpected := <-out:
			c.Fatalf("got %#v", unexpected)
		case <-time.After(coretesting.ShortWait):
		}
		return
	}
	expect := hook.Info{
		Kind:          d.hook,
		RelationId:    21345,
		RemoteUnit:    d.unit,
		ChangeVersion: d.version,
	}
	select {
	case actual := <-out:
		c.Assert(actual, gc.DeepEquals, expect)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for %#v", expect)
	}
}
