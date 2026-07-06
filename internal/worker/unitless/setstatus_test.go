// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unitless

import (
	"github.com/canonical/starform/starform"
	"github.com/canonical/starlark/starlark"
	"github.com/canonical/starlark/startest"
	"github.com/juju/tc"
)

// These must match Starform's event object thread-local bridge.
const starformEventObjectLocalKey = "starform-event-object"

type starformEventObjectStorage = struct {
	Event *starform.EventObject
}

func (s *starformSuite) TestSetStatusCPUSafe(c *tc.C) {
	assertSetStatusSafety(c, starlark.CPUSafe, func(st *startest.ST) {
		st.SetMinSteps(1)
	})
}

func (s *starformSuite) TestSetStatusMemSafe(c *tc.C) {
	assertSetStatusSafety(c, starlark.MemSafe, nil)
}

func (s *starformSuite) TestSetStatusTimeSafe(c *tc.C) {
	assertSetStatusSafety(c, starlark.TimeSafe, nil)
}

func (s *starformSuite) TestSetStatusIOSafe(c *tc.C) {
	assertSetStatusSafety(c, starlark.IOSafe, nil)
}

func assertSetStatusSafety(c *tc.C, safety starlark.SafetyFlags, configure func(*startest.ST)) {
	collector := &IntentCollector{}
	st := startest.From(c)
	st.AddLocal(starformEventObjectLocalKey, &starformEventObjectStorage{Event: &starform.EventObject{
		Name:  "config_changed",
		State: collector,
	}})
	st.RequireSafety(safety)
	if configure != nil {
		configure(st)
	}

	args := starlark.Tuple{starlark.String("active")}
	kwargs := []starlark.Tuple{{
		starlark.String("message"),
		starlark.String("updated"),
	}}
	st.RunThread(func(thread *starlark.Thread) {
		previousNumIntents := len(collector.intents)
		for i := 0; i < st.N; i++ {
			result, err := starlark.Call(thread, setStatusBuiltin, args, kwargs)
			if err != nil {
				st.Error(err)
				return
			}
			if result != starlark.None {
				st.Errorf("expected None, got %v", result)
				return
			}
		}

		expectedNumIntents := previousNumIntents + st.N
		if len(collector.intents) != expectedNumIntents {
			st.Errorf("expected %d intents, got %d", expectedNumIntents, len(collector.intents))
			return
		}
		if st.N > 0 {
			assertSetStatusIntent(st, collector.intents[len(collector.intents)-1])
		}
		st.KeepAlive(collector.intents)
	})
}

func assertSetStatusIntent(st *startest.ST, intent Intent) {
	if intent.Type != IntentSetStatus {
		st.Errorf("expected intent type %q, got %q", IntentSetStatus, intent.Type)
		return
	}
	if status := intent.Args["status"]; status != "active" {
		st.Errorf(`expected status "active", got %q`, status)
	}
	if message := intent.Args["message"]; message != "updated" {
		st.Errorf(`expected message "updated", got %q`, message)
	}
}
