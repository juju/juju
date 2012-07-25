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

var hookQueueTests = [][]checker{
	{
	// No steps; just implicitly check it's empty.
	}, {
		// Joined and changed are both run when unit is first detected.
		send{msi{"u0": 0}, nil},
		expect{"joined", "u0", 0, msi{"u0": 0}},
		expect{"changed", "u0", 0, msi{"u0": 0}},
	}, {
		// Automatic changed is run with latest settings.
		send{msi{"u0": 0}, nil},
		expect{"joined", "u0", 0, msi{"u0": 0}},
		send{msi{"u0": 7}, nil},
		expect{"changed", "u0", 7, msi{"u0": 7}},
	}, {
		// Joined is also run with latest settings.
		send{msi{"u0": 0}, nil},
		send{msi{"u0": 7}, nil},
		expect{"joined", "u0", 7, msi{"u0": 7}},
		expect{"changed", "u0", 7, msi{"u0": 7}},
	}, {
		// Nothing happens if a unit departs before its joined is run.
		send{msi{"u0": 0}, nil},
		send{msi{"u0": 7}, nil},
		send{nil, []string{"u0"}},
	}, {
		// A changed is run after a joined, even if a departed is known.
		send{msi{"u0": 0}, nil},
		expect{"joined", "u0", 0, msi{"u0": 0}},
		send{nil, []string{"u0"}},
		expect{"changed", "u0", 0, msi{"u0": 0}},
		expect{"departed", "u0", 0, nil},
	}, {
		// A departed replaces a changed.
		send{msi{"u0": 0}, nil},
		advance{2},
		send{msi{"u0": 7}, nil},
		send{nil, []string{"u0"}},
		expect{"departed", "u0", 7, nil},
	}, {
		// Changed events are ignored if the version has not changed.
		send{msi{"u0": 0}, nil},
		advance{2},
		send{msi{"u0": 0}, nil},
	}, {
		// Multiple changed events are compacted into one.
		send{msi{"u0": 0}, nil},
		advance{2},
		send{msi{"u0": 3}, nil},
		send{msi{"u0": 7}, nil},
		send{msi{"u0": 79}, nil},
		expect{"changed", "u0", 79, msi{"u0": 79}},
	}, {
		// Multiple changed events are elided.
		send{msi{"u0": 0}, nil},
		advance{2},
		send{msi{"u0": 3}, nil},
		send{msi{"u0": 7}, nil},
		send{msi{"u0": 79}, nil},
		expect{"changed", "u0", 79, msi{"u0": 79}},
	}, {
		// Latest hooks are run in the original unit order.
		send{msi{"u0": 0, "u1": 1}, nil},
		advance{4},
		send{msi{"u0": 3}, nil},
		send{msi{"u1": 7}, nil},
		send{nil, []string{"u0"}},
		expect{"departed", "u0", 3, msi{"u1": 7}},
		expect{"changed", "u1", 7, msi{"u1": 7}},
	}, {
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
	},
}

func (s *HookQueueSuite) TestHookQueue(c *C) {
	for i, t := range hookQueueTests {
		c.Logf("test %d", i)
		in := make(chan state.RelationUnitsChange)
		out := make(chan uniter.HookInfo)
		uniter.HookQueue(out, in)
		for i, step := range t {
			c.Logf("  step %d", i)
			step.check(c, in, out)
		}
		expect{}.check(c, in, out)
	}
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
	return map[string]interface{}{
		"unit-name":        name,
		"settings-version": version,
	}
}
