package relationer_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/worker/relationer"
	stdtesting "testing"
)

func Test(t *stdtesting.T) { testing.ZkTestPackage(t) }

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

var hookQueueTests = []struct {
	// init returns the number of times to call Next on the initial
	// queue state before adding the RelationUnitsChange events.
	init func(q *relationer.HookQueue) int
	// adds are all added to the queue in order after calling init
	// and advancing the queue.
	adds []state.RelationUnitsChange
	// prev, if true, will cause the first hook in gets to test the
	// result of calling Prev rather than Next.
	prev bool
	// gets should be the complete list of expected HookInfos.
	gets []relationer.HookInfo
}{
	// Empty queue.
	{nil, nil, false, nil},
	// Single changed event.
	{
		nil, []state.RelationUnitsChange{
			RUC(msi{"u0": 0}, nil),
		}, false, []relationer.HookInfo{
			HI("joined", "u0", msi{"u0": 0}),
			HI("changed", "u0", msi{"u0": 0}),
		},
	},
	// Pair of changed events for the same unit.
	{
		nil, []state.RelationUnitsChange{
			RUC(msi{"u0": 0}, nil),
			RUC(msi{"u0": 7}, nil),
		}, false, []relationer.HookInfo{
			HI("joined", "u0", msi{"u0": 7}),
			HI("changed", "u0", msi{"u0": 7}),
		},
	},
	// Changed events for a unit while its join is inflight.
	{
		func(q *relationer.HookQueue) int {
			q.Add(RUC(msi{"u0": 0}, nil))
			return 1
		}, []state.RelationUnitsChange{
			RUC(msi{"u0": 12}, nil),
			RUC(msi{"u0": 37}, nil),
		}, true, []relationer.HookInfo{
			HI("joined", "u0", msi{"u0": 37}),
			HI("changed", "u0", msi{"u0": 37}),
		},
	},
	// Changed events for a unit while its changed is inflight.
	{
		func(q *relationer.HookQueue) int {
			q.Add(RUC(msi{"u0": 0}, nil))
			return 2
		}, []state.RelationUnitsChange{
			RUC(msi{"u0": 12}, nil),
			RUC(msi{"u0": 37}, nil),
		}, true, []relationer.HookInfo{
			HI("changed", "u0", msi{"u0": 37}),
		},
	},
	// Single changed event followed by a departed.
	{
		nil, []state.RelationUnitsChange{
			RUC(msi{"u0": 0}, nil),
			RUC(nil, []string{"u0"}),
		}, false, nil,
	},
	// Multiple changed events followed by a departed.
	{
		nil, []state.RelationUnitsChange{
			RUC(msi{"u0": 0}, nil),
			RUC(msi{"u0": 23}, nil),
			RUC(nil, []string{"u0"}),
		}, false, nil,
	},
	// Departed event while joined is inflight. Not that the python version
	// does *not* always run the "changed" hook in this situation, which we
	// have decided is a Bad Thing, so we have fixed it.
	{
		func(q *relationer.HookQueue) int {
			q.Add(RUC(msi{"u0": 0}, nil))
			return 1
		}, []state.RelationUnitsChange{
			RUC(nil, []string{"u0"}),
		}, true, []relationer.HookInfo{
			HI("joined", "u0", msi{"u0": 0}),
			HI("changed", "u0", msi{"u0": 0}),
			HI("departed", "u0", nil),
		},
	},
	// Departed event while joined is inflight, and additional change is queued.
	// (Only a single changed should run, with latest settings.)
	{
		func(q *relationer.HookQueue) int {
			q.Add(RUC(msi{"u0": 0}, nil))
			return 1
		}, []state.RelationUnitsChange{
			RUC(msi{"u0": 12}, nil),
			RUC(nil, []string{"u0"}),
		}, true, []relationer.HookInfo{
			HI("joined", "u0", msi{"u0": 12}),
			HI("changed", "u0", msi{"u0": 12}),
			HI("departed", "u0", nil),
		},
	},
	// Departed event while changed is inflight.
	{
		func(q *relationer.HookQueue) int {
			q.Add(RUC(msi{"u0": 0}, nil))
			return 2
		}, []state.RelationUnitsChange{
			RUC(nil, []string{"u0"}),
		}, true, []relationer.HookInfo{
			HI("changed", "u0", msi{"u0": 0}),
			HI("departed", "u0", nil),
		},
	},
	// Departed event while changed is inflight, and additional change is queued.
	{
		func(q *relationer.HookQueue) int {
			q.Add(RUC(msi{"u0": 0}, nil))
			return 2
		}, []state.RelationUnitsChange{
			RUC(msi{"u0": 12}, nil),
			RUC(nil, []string{"u0"}),
		}, true, []relationer.HookInfo{
			HI("changed", "u0", msi{"u0": 12}),
			HI("departed", "u0", nil),
		},
	},
	// Departed followed by changed with newer version.
	{
		func(q *relationer.HookQueue) int {
			q.Add(RUC(msi{"u0": 0}, nil))
			return 2
		}, []state.RelationUnitsChange{
			RUC(nil, []string{"u0"}),
			RUC(msi{"u0": 12}, nil),
		}, false, []relationer.HookInfo{
			HI("changed", "u0", msi{"u0": 12}),
		},
	},
	// Departed followed by changed with same version.
	{
		func(q *relationer.HookQueue) int {
			q.Add(RUC(msi{"u0": 12}, nil))
			return 2
		}, []state.RelationUnitsChange{
			RUC(nil, []string{"u0"}),
			RUC(msi{"u0": 12}, nil),
		}, false, nil,
	},
	// Changed while departed inflight.
	{
		func(q *relationer.HookQueue) int {
			q.Add(RUC(msi{"u0": 0}, nil))
			q.Next()
			q.Done()
			q.Next()
			q.Done()
			q.Add(RUC(nil, []string{"u0"}))
			return 1
		}, []state.RelationUnitsChange{
			RUC(msi{"u0": 0}, nil),
		}, true, []relationer.HookInfo{
			HI("departed", "u0", nil),
			HI("joined", "u0", msi{"u0": 0}),
			HI("changed", "u0", msi{"u0": 0}),
		},
	},
	// Departed while changed already queued (the departed should occur at
	// the time the original changed was expected to occur).
	{
		func(q *relationer.HookQueue) int {
			q.Add(RUC(msi{"u0": 0, "u1": 0}, nil))
			return 4
		}, []state.RelationUnitsChange{
			RUC(msi{"u0": 1}, nil),
			RUC(msi{"u1": 1}, nil),
			RUC(nil, []string{"u0"}),
		}, false, []relationer.HookInfo{
			HI("departed", "u0", msi{"u1": 1}),
			HI("changed", "u1", msi{"u1": 1}),
		},
	},
	// Exercise everything I can think of at the same time.
	{
		func(q *relationer.HookQueue) int {
			q.Add(RUC(msi{"u0": 0, "u1": 0, "u2": 0, "u3": 0, "u4": 0}, nil))
			return 6 // u0, u1 up to date; u2 changed inflight; u3, u4 untouched.
		}, []state.RelationUnitsChange{
			RUC(msi{"u0": 1}, nil),
			RUC(msi{"u1": 1, "u2": 1, "u3": 1, "u5": 0}, []string{"u0", "u4"}),
			RUC(msi{"u3": 2}, nil),
		}, true, []relationer.HookInfo{
			// - Finish off the rest of the inited state, ignoring u4, but using
			// the latest known settings.
			HI("changed", "u2", msi{"u0": 1, "u1": 1, "u2": 1}),
			HI("joined", "u3", msi{"u0": 1, "u1": 1, "u2": 1, "u3": 2}),
			HI("changed", "u3", msi{"u0": 1, "u1": 1, "u2": 1, "u3": 2}),
			// - u0 was queued for change by the first RUC, but this change is
			// no longer relevant; it's departed in the second RUC, so we run
			// that hook instead.
			HI("departed", "u0", msi{"u1": 1, "u2": 1, "u3": 2}),
			// - Handle the remaining changes in the second RUC, still ignoring u4.
			// We do run a new changed hook for u1, because the latest settings
			// are newer than those used in its original changed event.
			HI("changed", "u1", msi{"u1": 1, "u2": 1, "u3": 2}),
			// No new change for u2, because it used its latest settings in the
			// retry of its initial inflight changed event.
			HI("joined", "u5", msi{"u1": 1, "u2": 1, "u3": 2, "u5": 0}),
			HI("changed", "u5", msi{"u1": 1, "u2": 1, "u3": 2, "u5": 0}),
			// - Ignore the third RUC, because the original joined/changed on u3
			// was executed after we got the latest settings version.
		},
	},
}

func (s *HookQueueSuite) TestHookQueue(c *C) {
	for i, t := range hookQueueTests {
		c.Logf("test %d", i)
		q := relationer.NewHookQueue()
		c.Assert(func() { q.Done() }, PanicMatches, "no inflight hook")
		if t.init != nil {
			steps := t.init(q)
			for i := 0; i < steps; i++ {
				c.Logf("%#v", q.Next())
				if i != steps-1 || !t.prev {
					c.Logf("done")
					q.Done()
				}
			}
		}
		for _, ruc := range t.adds {
			q.Add(ruc)
		}
		for i, expect := range t.gets {
			c.Logf("  change %d", i)
			c.Assert(q.Pending(), Equals, true)
			c.Assert(q.Next(), DeepEquals, expect)
			c.Assert(q.Pending(), Equals, true)
			c.Assert(q.Next(), DeepEquals, expect)
			q.Done()
		}
		if q.Pending() {
			c.Fatalf("unexpected %#v", q.Next())
		}
		c.Assert(func() { q.Next() }, PanicMatches, "queue is empty")
	}
}
