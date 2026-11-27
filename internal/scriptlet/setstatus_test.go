// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package scriptlet

import (
	"github.com/canonical/starform/formtest"
	"github.com/canonical/starform/starform"
	"github.com/canonical/starlark/starlark"
	"github.com/canonical/starlark/startest"
	"github.com/juju/tc"

	coreerrors "github.com/juju/juju/core/errors"
)

// These must match Starform's event object thread-local bridge.
const starformEventObjectLocalKey = "starform-event-object"

type starformEventObjectStorage = struct {
	Event *starform.EventObject
}

func (s *scriptletSuite) TestSetStatusCollectsIntent(c *tc.C) {
	collectors := newTestEnv().IntentCollectors()
	ft := formtest.From(c)
	ft.SetApp(app())
	ft.SetEvent(&starform.EventObject{
		Name: "config_changed",
		Attrs: starlark.StringDict{
			"message": starlark.String("updated"),
		},
		State: collectors,
	})

	ft.RunString(`
def init():
    juju.observe("config_changed", on_config_changed)

def on_config_changed(event):
    juju.set_status("active", message=event.message)
`)

	c.Check(IntentCollector[*StatusCollector](collectors), tc.DeepEquals, &StatusCollector{
		Status:  "active",
		Message: "updated",
	})
}

// TestSetStatusMessageOptional asserts that the message kwarg may be
// omitted, leaving the collected message empty.
func (s *scriptletSuite) TestSetStatusMessageOptional(c *tc.C) {
	collectors := newTestEnv().IntentCollectors()
	ft := formtest.From(c)
	ft.SetApp(app())
	ft.SetEvent(&starform.EventObject{
		Name:  "config_changed",
		State: collectors,
	})

	ft.RunString(`
def init():
    juju.observe("config_changed", on_config_changed)

def on_config_changed(event):
    juju.set_status("active")
`)

	c.Check(IntentCollector[*StatusCollector](collectors).Status, tc.Equals, "active")
}

func (s *scriptletSuite) TestSetStatusUnobservedEventHasNoIntent(c *tc.C) {
	collectors := newTestEnv().IntentCollectors()
	ft := formtest.From(c)
	ft.SetApp(app())
	ft.SetEvent(&starform.EventObject{
		Name:  "update_status",
		State: collectors,
	})

	ft.RunString(`
def init():
    juju.observe("config_changed", on_config_changed)

def on_config_changed(event):
    juju.set_status("active")
`)

	c.Check(IntentCollector[*StatusCollector](collectors), tc.DeepEquals, &StatusCollector{})
}

func (s *scriptletSuite) TestSetStatusEmptyStatusRejected(c *tc.C) {
	st := startest.From(c)
	st.AddLocal(starformEventObjectLocalKey, &starformEventObjectStorage{Event: &starform.EventObject{
		Name:  "config_changed",
		State: newTestEnv().IntentCollectors(),
	}})

	args := starlark.Tuple{starlark.String("")}
	st.RunThread(func(thread *starlark.Thread) {
		_, err := starlark.Call(thread, setStatusBuiltin, args, nil)
		c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
		c.Check(err, tc.ErrorMatches, "status must not be empty")
	})
}

func (s *scriptletSuite) TestSetStatusWithoutCollectors(c *tc.C) {
	st := startest.From(c)
	st.AddLocal(starformEventObjectLocalKey, &starformEventObjectStorage{Event: &starform.EventObject{
		Name: "config_changed",
	}})

	args := starlark.Tuple{starlark.String("active")}
	st.RunThread(func(thread *starlark.Thread) {
		_, err := starlark.Call(thread, setStatusBuiltin, args, nil)
		c.Assert(err, tc.ErrorMatches, `.*getting status collector from thread: unavailable`)
	})
}

func (s *scriptletSuite) TestCollectorNotRegistered(c *tc.C) {
	collectors := (&EnvRegistry{}).IntentCollectors()
	c.Check(func() {
		IntentCollector[*StatusCollector](collectors)
	}, tc.PanicMatches, `internal error: intent collector \*scriptlet\.StatusCollector not registered`)
}

func (s *scriptletSuite) TestSetStatusCPUSafe(c *tc.C) {
	assertSetStatusSafety(c, starlark.CPUSafe, func(st *startest.ST) {
		st.SetMaxSteps(0)
	})
}

func (s *scriptletSuite) TestSetStatusMemSafe(c *tc.C) {
	assertSetStatusSafety(c, starlark.MemSafe, func(st *startest.ST) {
		st.SetMaxAllocs(0)
	})
}

func (s *scriptletSuite) TestSetStatusTimeSafe(c *tc.C) {
	assertSetStatusSafety(c, starlark.TimeSafe, nil)
}

func (s *scriptletSuite) TestSetStatusIOSafe(c *tc.C) {
	assertSetStatusSafety(c, starlark.IOSafe, nil)
}

func newTestEnv() *EnvRegistry {
	env := &EnvRegistry{}
	RegisterEnv(env)
	return env
}

func app() *starform.AppObject {
	return &starform.AppObject{
		Name:    "juju",
		Methods: []*starlark.Builtin{setStatusBuiltin},
	}
}

func assertSetStatusSafety(c *tc.C, safety starlark.SafetyFlags, configure func(*startest.ST)) {
	collectors := newTestEnv().IntentCollectors()
	st := startest.From(c)
	st.AddLocal(starformEventObjectLocalKey, &starformEventObjectStorage{Event: &starform.EventObject{
		Name:  "config_changed",
		State: collectors,
	}})
	st.RequireSafety(safety)
	if configure != nil {
		configure(st)
	}

	// Incoming values are already accounted for by the interpreter, so
	// they are allocated before the measured region; each iteration
	// passes a distinct message.
	messages := []string{"updated", "renewed", "refreshed"}
	kwargsByMessage := make([][]starlark.Tuple, len(messages))
	for i, message := range messages {
		kwargsByMessage[i] = []starlark.Tuple{{
			starlark.String("message"),
			starlark.String(message),
		}}
	}
	args := starlark.Tuple{starlark.String("active")}
	st.RunThread(func(thread *starlark.Thread) {
		for i := 0; i < st.N; i++ {
			kwargs := kwargsByMessage[i%len(kwargsByMessage)]
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

		statusCollector := IntentCollector[*StatusCollector](collectors)
		expectedMessage := messages[(st.N-1)%len(messages)]
		if statusCollector.Status != "active" || statusCollector.Message != expectedMessage {
			st.Errorf(`expected last status "active"/%q, got %q/%q`,
				expectedMessage, statusCollector.Status, statusCollector.Message)
			return
		}
		st.KeepAlive(statusCollector)
	})
}
