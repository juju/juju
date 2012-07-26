package uniter_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/worker/uniter"
	stdtesting "testing"
	"time"
)

func Test(t *stdtesting.T) { testing.ZkTestPackage(t) }

type HookQueueSuite struct{}

var _ = Suite(&HookQueueSuite{})

type msi map[string]int

type hookQueueTest struct {
	initial uniter.QueueState
	steps   []checker
}

func nilTest(steps ...checker) hookQueueTest {
	return hookQueueTest{uniter.QueueState{21345, nil, ""}, steps}
}

func initialTest(members msi, joined string, steps ...checker) hookQueueTest {
	return hookQueueTest{uniter.QueueState{21345, members, joined}, steps}
}

var hookQueueTests = []hookQueueTest{
	nilTest(
		// Empty initial change causes no hooks.
		send{nil, nil},
	), nilTest(
		// Joined and changed are both run when unit is first detected.
		send{msi{"u0": 0}, nil},
		expect{"joined", "u0", 0, msi{"u0": 0}},
		expect{"changed", "u0", 0, msi{"u0": 0}},
	), nilTest(
		// Automatic changed is run with latest settings.
		send{msi{"u0": 0}, nil},
		expect{"joined", "u0", 0, msi{"u0": 0}},
		send{msi{"u0": 7}, nil},
		expect{"changed", "u0", 7, msi{"u0": 7}},
	), nilTest(
		// Joined is also run with latest settings.
		send{msi{"u0": 0}, nil},
		send{msi{"u0": 7}, nil},
		expect{"joined", "u0", 7, msi{"u0": 7}},
		expect{"changed", "u0", 7, msi{"u0": 7}},
	), nilTest(
		// Nothing happens if a unit departs before its joined is run.
		send{msi{"u0": 0}, nil},
		send{msi{"u0": 7}, nil},
		send{nil, []string{"u0"}},
	), nilTest(
		// A changed is run after a joined, even if a departed is known.
		send{msi{"u0": 0}, nil},
		expect{"joined", "u0", 0, msi{"u0": 0}},
		send{nil, []string{"u0"}},
		expect{"changed", "u0", 0, msi{"u0": 0}},
		expect{"departed", "u0", 0, nil},
	), nilTest(
		// A departed replaces a changed.
		send{msi{"u0": 0}, nil},
		advance{2},
		send{msi{"u0": 7}, nil},
		send{nil, []string{"u0"}},
		expect{"departed", "u0", 7, nil},
	), nilTest(
		// Changed events are ignored if the version has not changed.
		send{msi{"u0": 0}, nil},
		advance{2},
		send{msi{"u0": 0}, nil},
	), nilTest(
		// Multiple changed events are compacted into one.
		send{msi{"u0": 0}, nil},
		advance{2},
		send{msi{"u0": 3}, nil},
		send{msi{"u0": 7}, nil},
		send{msi{"u0": 79}, nil},
		expect{"changed", "u0", 79, msi{"u0": 79}},
	), nilTest(
		// Multiple changed events are elided.
		send{msi{"u0": 0}, nil},
		advance{2},
		send{msi{"u0": 3}, nil},
		send{msi{"u0": 7}, nil},
		send{msi{"u0": 79}, nil},
		expect{"changed", "u0", 79, msi{"u0": 79}},
	), nilTest(
		// Latest hooks are run in the original unit order.
		send{msi{"u0": 0, "u1": 1}, nil},
		advance{4},
		send{msi{"u0": 3}, nil},
		send{msi{"u1": 7}, nil},
		send{nil, []string{"u0"}},
		expect{"departed", "u0", 3, msi{"u1": 7}},
		expect{"changed", "u1", 7, msi{"u1": 7}},
	), nilTest(
		// Test everything we can think of at the same time.
		send{msi{"u0": 0, "u1": 0, "u2": 0, "u3": 0, "u4": 0}, nil},
		advance{6},
		// u0, u1, u2 are now up to date; u3, u4 are untouched.
		send{msi{"u0": 1}, nil},
		send{msi{"u1": 1, "u2": 1, "u3": 1, "u5": 0}, []string{"u0", "u4"}},
		send{msi{"u3": 2}, nil},
		// - Finish off the rest of the initial state, ignoring u4, but using
		// the latest known settings.
		expect{"joined", "u3", 2, msi{"u0": 1, "u1": 1, "u2": 1, "u3": 2}},
		expect{"changed", "u3", 2, msi{"u0": 1, "u1": 1, "u2": 1, "u3": 2}},
		// - u0 was queued for change by the first RUC, but this change is
		// no longer relevant; it's departed in the second RUC, so we run
		// that hook instead.
		expect{"departed", "u0", 1, msi{"u1": 1, "u2": 1, "u3": 2}},
		// - Handle the remaining changes in the second RUC, still ignoring u4.
		// We do run new changed hooks for u1 and u2, because the latest settings
		// are newer than those used in their original changed events.
		expect{"changed", "u1", 1, msi{"u1": 1, "u2": 1, "u3": 2}},
		expect{"changed", "u2", 1, msi{"u1": 1, "u2": 1, "u3": 2}},
		expect{"joined", "u5", 0, msi{"u1": 1, "u2": 1, "u3": 2, "u5": 0}},
		expect{"changed", "u5", 0, msi{"u1": 1, "u2": 1, "u3": 2, "u5": 0}},
		// - Ignore the third RUC, because the original joined/changed on u3
		// was executed after we got the latest settings version.
	), initialTest(
		// Check that matching settings versions cause no changes.
		msi{"u0": 0}, "",
		send{msi{"u0": 0}, nil},
	), initialTest(
		// Check that new settings versions cause appropriate changes.
		msi{"u0": 0}, "",
		send{msi{"u0": 1}, nil},
		expect{"changed", "u0", 1, msi{"u0": 1}},
	), initialTest(
		// Check that a just-joined unit gets its changed hook run first.
		msi{"u0": 0}, "u0",
		send{msi{"u0": 0}, nil},
		expect{"changed", "u0", 0, msi{"u0": 0}},
	), initialTest(
		// Check that missing units are queued for depart as early as possible.
		msi{"u0": 0}, "",
		send{msi{"u1": 0}, nil},
		expect{"departed", "u0", 0, nil},
		expect{"joined", "u1", 0, msi{"u1": 0}},
		expect{"changed", "u1", 0, msi{"u1": 0}},
	), initialTest(
		// Double-check that a just-joined unit gets its changed hook run first,
		// even when it's due to depart.
		msi{"u0": 0}, "u0",
		send{nil, nil},
		expect{"changed", "u0", 0, msi{"u0": -1}},
		expect{"departed", "u0", 0, nil},
	), initialTest(
		// Check that missing units don't slip in front of required changed hooks.
		msi{"u0": 0}, "u0",
		send{msi{"u1": 0}, nil},
		expect{"changed", "u0", 0, msi{"u0": -1}},
		expect{"departed", "u0", 0, nil},
		expect{"joined", "u1", 0, msi{"u1": 0}},
		expect{"changed", "u1", 0, msi{"u1": 0}},
	),
}

func (s *HookQueueSuite) TestHookQueue(c *C) {
	for i, t := range hookQueueTests {
		c.Logf("test %d", i)
		out := make(chan uniter.HookInfo)
		in := make(chan state.RelationUnitsChange)
		ruw := &RUW{in, false}
		q := uniter.NewHookQueue(t.initial, out, ruw)
		for i, step := range t.steps {
			c.Logf("  step %d", i)
			step.check(c, in, out)
		}
		expect{}.check(c, in, out)
		q.Stop()
		c.Assert(ruw.stopped, Equals, true)
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
	check(c *C, in chan state.RelationUnitsChange, out chan uniter.HookInfo)
}

type send struct {
	changed  msi
	departed []string
}

func (d send) check(c *C, in chan state.RelationUnitsChange, out chan uniter.HookInfo) {
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

func (d advance) check(c *C, in chan state.RelationUnitsChange, out chan uniter.HookInfo) {
	for i := 0; i < d.count; i++ {
		select {
		case <-out:
		case <-time.After(200 * time.Millisecond):
			c.Fatalf("timed out waiting for event %d", i)
		}
	}
}

type expect struct {
	hook, unit string
	version    int
	members    msi
}

func (d expect) check(c *C, in chan state.RelationUnitsChange, out chan uniter.HookInfo) {
	if d.hook == "" {
		select {
		case unexpected := <-out:
			c.Fatalf("got %#v", unexpected)
		case <-time.After(200 * time.Millisecond):
		}
		return
	}
	expect := uniter.HookInfo{
		HookKind:   d.hook,
		RelationId: 21345,
		RemoteUnit: d.unit,
		Version:    d.version,
		Members:    map[string]map[string]interface{}{},
	}
	for name, version := range d.members {
		expect.Members[name] = settings(name, version)
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
