package relation_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/worker/uniter/hook"
	"launchpad.net/juju-core/worker/uniter/relation"
	stdtesting "testing"
	"time"
)

func Test(t *stdtesting.T) { testing.ZkTestPackage(t) }

type HookQueueSuite struct{}

var _ = Suite(&HookQueueSuite{})

type msi map[string]int

type hookQueueTest struct {
	initial *relation.State
	steps   []checker
}

func fullTest(steps ...checker) hookQueueTest {
	return hookQueueTest{&relation.State{21345, nil, ""}, steps}
}

func reconcileTest(members msi, joined string, steps ...checker) hookQueueTest {
	return hookQueueTest{&relation.State{21345, members, joined}, steps}
}

var hookQueueTests = []hookQueueTest{
	fullTest(
		// Empty initial change causes no hooks.
		send{nil, nil},
	), fullTest(
		// Joined and changed are both run when unit is first detected.
		send{msi{"u/0": 0}, nil},
		expect{hook.RelationJoined, "u/0", 0, msi{"u/0": 0}},
		expect{hook.RelationChanged, "u/0", 0, msi{"u/0": 0}},
	), fullTest(
		// Automatic changed is run with latest settings.
		send{msi{"u/0": 0}, nil},
		expect{hook.RelationJoined, "u/0", 0, msi{"u/0": 0}},
		send{msi{"u/0": 7}, nil},
		expect{hook.RelationChanged, "u/0", 7, msi{"u/0": 7}},
	), fullTest(
		// Joined is also run with latest settings.
		send{msi{"u/0": 0}, nil},
		send{msi{"u/0": 7}, nil},
		expect{hook.RelationJoined, "u/0", 7, msi{"u/0": 7}},
		expect{hook.RelationChanged, "u/0", 7, msi{"u/0": 7}},
	), fullTest(
		// Nothing happens if a unit departs before its joined is run.
		send{msi{"u/0": 0}, nil},
		send{msi{"u/0": 7}, nil},
		send{nil, []string{"u/0"}},
	), fullTest(
		// A changed is run after a joined, even if a departed is known.
		send{msi{"u/0": 0}, nil},
		expect{hook.RelationJoined, "u/0", 0, msi{"u/0": 0}},
		send{nil, []string{"u/0"}},
		expect{hook.RelationChanged, "u/0", 0, msi{"u/0": 0}},
		expect{hook.RelationDeparted, "u/0", 0, msi{}},
	), fullTest(
		// A departed replaces a changed.
		send{msi{"u/0": 0}, nil},
		advance{2},
		send{msi{"u/0": 7}, nil},
		send{nil, []string{"u/0"}},
		expect{hook.RelationDeparted, "u/0", 7, msi{}},
	), fullTest(
		// Changed events are ignored if the version has not changed.
		send{msi{"u/0": 0}, nil},
		advance{2},
		send{msi{"u/0": 0}, nil},
	), fullTest(
		// Multiple changed events are compacted into one.
		send{msi{"u/0": 0}, nil},
		advance{2},
		send{msi{"u/0": 3}, nil},
		send{msi{"u/0": 7}, nil},
		send{msi{"u/0": 79}, nil},
		expect{hook.RelationChanged, "u/0", 79, msi{"u/0": 79}},
	), fullTest(
		// Multiple changed events are elided.
		send{msi{"u/0": 0}, nil},
		advance{2},
		send{msi{"u/0": 3}, nil},
		send{msi{"u/0": 7}, nil},
		send{msi{"u/0": 79}, nil},
		expect{hook.RelationChanged, "u/0", 79, msi{"u/0": 79}},
	), fullTest(
		// Latest hooks are run in the original unit order.
		send{msi{"u/0": 0, "u/1": 1}, nil},
		advance{4},
		send{msi{"u/0": 3}, nil},
		send{msi{"u/1": 7}, nil},
		send{nil, []string{"u/0"}},
		expect{hook.RelationDeparted, "u/0", 3, msi{"u/1": 7}},
		expect{hook.RelationChanged, "u/1", 7, msi{"u/1": 7}},
	), fullTest(
		// Test everything we can think of at the same time.
		send{msi{"u/0": 0, "u/1": 0, "u/2": 0, "u/3": 0, "u/4": 0}, nil},
		advance{6},
		// u/0, u/1, u/2 are now up to date; u/3, u/4 are untouched.
		send{msi{"u/0": 1}, nil},
		send{msi{"u/1": 1, "u/2": 1, "u/3": 1, "u/5": 0}, []string{"u/0", "u/4"}},
		send{msi{"u/3": 2}, nil},
		// - Finish off the rest of the initial state, ignoring u/4, but using
		// the latest known settings.
		expect{hook.RelationJoined, "u/3", 2, msi{"u/0": 1, "u/1": 1, "u/2": 1, "u/3": 2}},
		expect{hook.RelationChanged, "u/3", 2, msi{"u/0": 1, "u/1": 1, "u/2": 1, "u/3": 2}},
		// - u/0 was queued for change by the first RUC, but this change is
		// no longer relevant; it's departed in the second RUC, so we run
		// that hook instead.
		expect{hook.RelationDeparted, "u/0", 1, msi{"u/1": 1, "u/2": 1, "u/3": 2}},
		// - Handle the remaining changes in the second RUC, still ignoring u/4.
		// We do run new changed hooks for u/1 and u/2, because the latest settings
		// are newer than those used in their original changed events.
		expect{hook.RelationChanged, "u/1", 1, msi{"u/1": 1, "u/2": 1, "u/3": 2}},
		expect{hook.RelationChanged, "u/2", 1, msi{"u/1": 1, "u/2": 1, "u/3": 2}},
		expect{hook.RelationJoined, "u/5", 0, msi{"u/1": 1, "u/2": 1, "u/3": 2, "u/5": 0}},
		expect{hook.RelationChanged, "u/5", 0, msi{"u/1": 1, "u/2": 1, "u/3": 2, "u/5": 0}},
		// - Ignore the third RUC, because the original joined/changed on u/3
		// was executed after we got the latest settings version.
	), reconcileTest(
		// Check that matching settings versions cause no changes.
		msi{"u/0": 0}, "",
		send{msi{"u/0": 0}, nil},
	), reconcileTest(
		// Check that new settings versions cause appropriate changes.
		msi{"u/0": 0}, "",
		send{msi{"u/0": 1}, nil},
		expect{hook.RelationChanged, "u/0", 1, msi{"u/0": 1}},
	), reconcileTest(
		// Check that a just-joined unit gets its changed hook run first.
		msi{"u/0": 0}, "u/0",
		send{msi{"u/0": 0}, nil},
		expect{hook.RelationChanged, "u/0", 0, msi{"u/0": 0}},
	), reconcileTest(
		// Check that missing units are queued for depart as early as possible.
		msi{"u/0": 0}, "",
		send{msi{"u/1": 0}, nil},
		expect{hook.RelationDeparted, "u/0", 0, msi{}},
		expect{hook.RelationJoined, "u/1", 0, msi{"u/1": 0}},
		expect{hook.RelationChanged, "u/1", 0, msi{"u/1": 0}},
	), reconcileTest(
		// Double-check that a just-joined unit gets its changed hook run first,
		// even when it's due to depart.
		msi{"u/0": 0}, "u/0",
		send{nil, nil},
		expect{hook.RelationChanged, "u/0", 0, msi{"u/0": -1}},
		expect{hook.RelationDeparted, "u/0", 0, msi{}},
	), reconcileTest(
		// Check that missing units don't slip in front of required changed hooks.
		msi{"u/0": 0}, "u/0",
		send{msi{"u/1": 0}, nil},
		expect{hook.RelationChanged, "u/0", 0, msi{"u/0": -1}},
		expect{hook.RelationDeparted, "u/0", 0, msi{}},
		expect{hook.RelationJoined, "u/1", 0, msi{"u/1": 0}},
		expect{hook.RelationChanged, "u/1", 0, msi{"u/1": 0}},
	),
}

func (s *HookQueueSuite) TestAliveHookQueue(c *C) {
	for i, t := range hookQueueTests {
		c.Logf("test %d", i)
		out := make(chan hook.Info)
		in := make(chan state.RelationUnitsChange)
		ruw := &RUW{in, false}
		q := relation.NewAliveHookQueue(t.initial, out, ruw)
		for i, step := range t.steps {
			c.Logf("  step %d", i)
			step.check(c, in, out)
		}
		expect{}.check(c, in, out)
		q.Stop()
		c.Assert(ruw.stopped, Equals, true)
	}
}

var brokenHookQueueTests = []hookQueueTest{
	fullTest(
		// Empty state just gets a broken hook.
		expect{hook: hook.RelationBroken},
	), reconcileTest(
		// Each current member is departed before broken is sent.
		msi{"u/1": 7, "u/4": 33}, "",
		expect{hook.RelationDeparted, "u/1", 7, msi{"u/4": -1}},
		expect{hook.RelationDeparted, "u/4", 33, msi{}},
		expect{hook: hook.RelationBroken},
	), reconcileTest(
		// If there's a pending changed, that must still be respected.
		msi{"u/0": 3}, "u/0",
		expect{hook.RelationChanged, "u/0", 3, msi{"u/0": -1}},
		expect{hook.RelationDeparted, "u/0", 3, msi{}},
		expect{hook: hook.RelationBroken},
	),
}

func (s *HookQueueSuite) TestBrokenHookQueue(c *C) {
	for i, t := range brokenHookQueueTests {
		c.Logf("test %d", i)
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
	in      chan state.RelationUnitsChange
	stopped bool
}

func (w *RUW) Changes() <-chan state.RelationUnitsChange {
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
	check(c *C, in chan state.RelationUnitsChange, out chan hook.Info)
}

type send struct {
	changed  msi
	departed []string
}

func (d send) check(c *C, in chan state.RelationUnitsChange, out chan hook.Info) {
	ruc := state.RelationUnitsChange{Changed: map[string]state.UnitSettings{}}
	for name, version := range d.changed {
		ruc.Changed[name] = state.UnitSettings{
			Version:  version,
			Settings: settings(name, version),
		}
	}
	for _, name := range d.departed {
		ruc.Departed = append(ruc.Departed, name)
	}
	in <- ruc
}

type advance struct {
	count int
}

func (d advance) check(c *C, in chan state.RelationUnitsChange, out chan hook.Info) {
	for i := 0; i < d.count; i++ {
		select {
		case <-out:
		case <-time.After(200 * time.Millisecond):
			c.Fatalf("timed out waiting for event %d", i)
		}
	}
}

type expect struct {
	hook    hook.Kind
	unit    string
	version int
	members msi
}

func (d expect) check(c *C, in chan state.RelationUnitsChange, out chan hook.Info) {
	if d.hook == "" {
		select {
		case unexpected := <-out:
			c.Fatalf("got %#v", unexpected)
		case <-time.After(200 * time.Millisecond):
		}
		return
	}
	expect := hook.Info{
		Kind:          d.hook,
		RelationId:    21345,
		RemoteUnit:    d.unit,
		ChangeVersion: d.version,
	}
	if d.members != nil {
		expect.Members = map[string]map[string]interface{}{}
		for name, version := range d.members {
			expect.Members[name] = settings(name, version)
		}
	}
	select {
	case actual := <-out:
		c.Assert(actual, DeepEquals, expect)
	case <-time.After(200 * time.Millisecond):
		c.Fatalf("timed out waiting for %#v", expect)
	}
}

func settings(name string, version int) map[string]interface{} {
	if version == -1 {
		// Accommodate required events for units no longer present in the
		// relation, whose settings will not be available through the stream
		// of RelationUnitsChanged events.
		return nil
	}
	return map[string]interface{}{
		"unit-name":        name,
		"settings-version": version,
	}
}
