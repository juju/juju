package relationer_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/worker/relationer"
	"testing"
)

func Test(t *testing.T) { TestingT(t) }

type HookQueueSuite struct{}

var _ = Suite(&HookQueueSuite{})

type msi map[string]int

func RUC(changed msi, departed []string) state.RelationUnitsChange {
	ruc := state.RelationUnitsChange{Changed: map[string]state.UnitSettings{}}
	for name, version := range changed {
		ruc.Changed[name] = state.UnitSettings{
			Version:  version,
			Settings: settings(name, version),
		}
	}
	for _, name := range departed {
		ruc.Departed = append(ruc.Departed, name)
	}
	return ruc
}

func HI(name, unit string, members msi) relationer.HookInfo {
	hi := relationer.HookInfo{name, unit, map[string]map[string]interface{}{}}
	for name, version := range members {
		hi.Members[name] = settings(name, version)
	}
	return hi
}

func settings(name string, version int) map[string]interface{} {
	return map[string]interface{}{
		"unit-name":        name,
		"settings-version": version,
	}
}

func advance(q *relationer.HookQueue, steps int) {
	for i := 0; i < steps; i++ {
		if i%2 == 0 {
			q.Next()
		} else {
			q.Done()
		}
	}
}

func exhaust(q *relationer.HookQueue) {
	for {
		if _, found := q.Next(); !found {
			return
		}
		q.Done()
	}
}

var hookQueueTests = []struct {
	init func(q *relationer.HookQueue)
	adds []state.RelationUnitsChange
	gets []relationer.HookInfo
}{
	// Empty queue.
	{nil, nil, nil},
	// Single changed event.
	{
		nil, []state.RelationUnitsChange{
			RUC(msi{"u0": 0}, nil),
		}, []relationer.HookInfo{
			HI("joined", "u0", msi{"u0": 0}),
			HI("changed", "u0", msi{"u0": 0}),
		},
	},
	// Pair of changed events for the same unit.
	{
		nil, []state.RelationUnitsChange{
			RUC(msi{"u0": 0}, nil),
			RUC(msi{"u0": 7}, nil),
		}, []relationer.HookInfo{
			HI("joined", "u0", msi{"u0": 7}),
			HI("changed", "u0", msi{"u0": 7}),
		},
	},
	// Changed events for a unit while its join is inflight.
	{
		func(q *relationer.HookQueue) {
			q.Add(RUC(msi{"u0": 0}, nil))
			advance(q, 1)
		}, []state.RelationUnitsChange{
			RUC(msi{"u0": 12}, nil),
			RUC(msi{"u0": 37}, nil),
		}, []relationer.HookInfo{
			HI("joined", "u0", msi{"u0": 37}),
			HI("changed", "u0", msi{"u0": 37}),
		},
	},
	// Changed events for a unit while its changed is inflight.
	{
		func(q *relationer.HookQueue) {
			q.Add(RUC(msi{"u0": 0}, nil))
			advance(q, 3)
		}, []state.RelationUnitsChange{
			RUC(msi{"u0": 12}, nil),
			RUC(msi{"u0": 37}, nil),
		}, []relationer.HookInfo{
			HI("changed", "u0", msi{"u0": 37}),
		},
	},
	// Single changed event followed by a departed.
	{
		nil, []state.RelationUnitsChange{
			RUC(msi{"u0": 0}, nil),
			RUC(nil, []string{"u0"}),
		}, nil,
	},
	// Multiple changed events followed by a departed.
	{
		nil, []state.RelationUnitsChange{
			RUC(msi{"u0": 0}, nil),
			RUC(msi{"u0": 23}, nil),
			RUC(nil, []string{"u0"}),
		}, nil,
	},
	// Departed event while joined is inflight.
	// Note that this is the only case where a joined is *not* immediately
	// followed by a changed: this matches the python implementation, which
	// has tests verifying this behaviour.
	{
		func(q *relationer.HookQueue) {
			q.Add(RUC(msi{"u0": 0}, nil))
			advance(q, 1)
		}, []state.RelationUnitsChange{
			RUC(nil, []string{"u0"}),
		}, []relationer.HookInfo{
			HI("joined", "u0", msi{"u0": 0}),
			HI("departed", "u0", nil),
		},
	},
	// Departed event while joined is inflight, and additional change is queued.
	// (The queued change should also be elided.)
	{
		func(q *relationer.HookQueue) {
			q.Add(RUC(msi{"u0": 0}, nil))
			advance(q, 1)
		}, []state.RelationUnitsChange{
			RUC(msi{"u0": 12}, nil),
			RUC(nil, []string{"u0"}),
		}, []relationer.HookInfo{
			HI("joined", "u0", msi{"u0": 12}),
			HI("departed", "u0", nil),
		},
	},
	// Departed event while changed is inflight.
	{
		func(q *relationer.HookQueue) {
			q.Add(RUC(msi{"u0": 0}, nil))
			advance(q, 3)
		}, []state.RelationUnitsChange{
			RUC(nil, []string{"u0"}),
		}, []relationer.HookInfo{
			HI("changed", "u0", msi{"u0": 0}),
			HI("departed", "u0", nil),
		},
	},
	// Departed event while changed is inflight, and additional change is queued.
	{
		func(q *relationer.HookQueue) {
			q.Add(RUC(msi{"u0": 0}, nil))
			advance(q, 3)
		}, []state.RelationUnitsChange{
			RUC(msi{"u0": 12}, nil),
			RUC(nil, []string{"u0"}),
		}, []relationer.HookInfo{
			HI("changed", "u0", msi{"u0": 12}),
			HI("departed", "u0", nil),
		},
	},
	// Departed followed by changed with newer version.
	{
		func(q *relationer.HookQueue) {
			q.Add(RUC(msi{"u0": 0}, nil))
			exhaust(q)
		}, []state.RelationUnitsChange{
			RUC(nil, []string{"u0"}),
			RUC(msi{"u0": 12}, nil),
		}, []relationer.HookInfo{
			HI("changed", "u0", msi{"u0": 12}),
		},
	},
	// Departed followed by changed with same version.
	{
		func(q *relationer.HookQueue) {
			q.Add(RUC(msi{"u0": 12}, nil))
			exhaust(q)
		}, []state.RelationUnitsChange{
			RUC(nil, []string{"u0"}),
			RUC(msi{"u0": 12}, nil),
		}, nil,
	},
	// Changed while departed inflight.
	{
		func(q *relationer.HookQueue) {
			q.Add(RUC(msi{"u0": 0}, nil))
			exhaust(q)
			q.Add(RUC(nil, []string{"u0"}))
			advance(q, 1)
		}, []state.RelationUnitsChange{
			RUC(msi{"u0": 0}, nil),
		}, []relationer.HookInfo{
			HI("departed", "u0", nil),
			HI("joined", "u0", msi{"u0": 0}),
			HI("changed", "u0", msi{"u0": 0}),
		},
	},
	// Exercise everything I can think of at the same time.
	{
		func(q *relationer.HookQueue) {
			q.Add(RUC(msi{"u0": 0, "u1": 0, "u2": 0, "u3": 0, "u4": 0}, nil))
			advance(q, 11) // u0, u1 up to date; u2 changed inflight; u3, u4 untouched.
		}, []state.RelationUnitsChange{
			RUC(msi{"u0": 1}, nil),
			RUC(msi{"u1": 1, "u2": 1, "u3": 1, "u5": 0}, []string{"u0", "u4"}),
			RUC(msi{"u3": 2}, nil),
		}, []relationer.HookInfo{
			// - Finish off the rest of the inited state, ignoring u4, but using
			// the latest known settings.
			HI("changed", "u2", msi{"u0": 1, "u1": 1, "u2": 1}),
			HI("joined", "u3", msi{"u0": 1, "u1": 1, "u2": 1, "u3": 2}),
			HI("changed", "u3", msi{"u0": 1, "u1": 1, "u2": 1, "u3": 2}),
			// - Ignore the first RUC, u0 is going away soon enough.
			// - Handle the changes in the second RUC, still ignoring u4.
			// We do run a new changed hook for u1, because the latest settings
			// are newer than those used in its original changed event.
			HI("changed", "u1", msi{"u0": 1, "u1": 1, "u2": 1, "u3": 2}),
			// No new change for u2, because it used its latest settings in the
			// retry of its initial inflight changed event.
			HI("joined", "u5", msi{"u0": 1, "u1": 1, "u2": 1, "u3": 2, "u5": 0}),
			HI("changed", "u5", msi{"u0": 1, "u1": 1, "u2": 1, "u3": 2, "u5": 0}),
			HI("departed", "u0", msi{"u1": 1, "u2": 1, "u3": 2, "u5": 0}),
			// - Ignore the third RUC, because the original joined/changed on u3
			// was executed after we got the latest settings version.
		},
	},
}

func (s *HookQueueSuite) TestHookQueue(c *C) {
	for i, t := range hookQueueTests {
		c.Logf("test %d", i)
		q := relationer.NewHookQueue()
		if t.init != nil {
			t.init(q)
		}
		for _, ruc := range t.adds {
			q.Add(ruc)
		}
		for i, expect := range t.gets {
			c.Logf("  change %d", i)
			actual, found := q.Next()
			c.Assert(found, Equals, true)
			c.Assert(actual, DeepEquals, expect)
			// We haven't said we're finished with the last hook, so we
			// should get the inflight one again.
			actual, found = q.Next()
			c.Assert(found, Equals, true)
			c.Assert(actual, DeepEquals, expect)
			q.Done()
			c.Assert(func() { q.Done() }, PanicMatches, "can't call Done when no hook is inflight")
		}
		_, found := q.Next()
		c.Assert(found, Equals, false)
		c.Assert(func() { q.Done() }, PanicMatches, "can't call Done when no hook is inflight")
	}
}
