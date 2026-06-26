// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unitless

import (
	"testing"

	"github.com/juju/tc"
)

type starformSuite struct{}

func TestStarformSuite(t *testing.T) {
	tc.Run(t, &starformSuite{})
}

func (s *starformSuite) TestHandleCollectsIntents(c *tc.C) {
	executor, err := NewStarformExecutor(c.Context(), ExecutorConfig{
		Scriptlet: Scriptlet{
			AppName: "juju",
			Sources: []ScriptSource{{
				LoadPath: "hooks.star",
				Source: `
def init():
    juju.observe("config_changed", on_config_changed)

def on_config_changed(event):
    juju.status_set("active", message=event.message)
`,
			}},
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	intents, err := executor.Handle(c.Context(), Event{
		Name: "config_changed",
		Attrs: map[string]any{
			"message": "updated",
			"config": map[string]any{
				"refresh-token": "abc123",
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(intents, tc.DeepEquals, []Intent{{
		Type:    IntentStatusSet,
		Status:  "active",
		Message: "updated",
	}})
}

func (s *starformSuite) TestHandleScriptErrorDiscardsIntents(c *tc.C) {
	executor, err := NewStarformExecutor(c.Context(), ExecutorConfig{
		Scriptlet: Scriptlet{
			AppName: "juju",
			Sources: []ScriptSource{{
				LoadPath: "hooks.star",
				Source: `
def init():
    juju.observe("config_changed", on_config_changed)

def on_config_changed(event):
    juju.status_set("active", message="before failure")
    1 / 0
`,
			}},
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	intents, err := executor.Handle(c.Context(), Event{Name: "config_changed"})
	c.Assert(err, tc.ErrorMatches, `.*division by zero.*`)
	c.Check(intents, tc.IsNil)
}

func (s *starformSuite) TestHandleUnobservedEventHasNoIntents(c *tc.C) {
	executor, err := NewStarformExecutor(c.Context(), ExecutorConfig{
		Scriptlet: Scriptlet{
			AppName: "juju",
			Sources: []ScriptSource{{
				LoadPath: "hooks.star",
				Source: `
def init():
    juju.observe("config_changed", on_config_changed)

def on_config_changed(event):
    juju.status_set("active")
`,
			}},
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	intents, err := executor.Handle(c.Context(), Event{Name: "update_status"})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(intents, tc.DeepEquals, []Intent{})
}
