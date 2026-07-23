// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unitless

import (
	"testing"

	"github.com/canonical/starlark/starlark"
	"github.com/juju/tc"

	coreerrors "github.com/juju/juju/core/errors"
	domainunitless "github.com/juju/juju/domain/unitless"
)

type starformSuite struct{}

func TestStarformSuite(t *testing.T) {
	tc.Run(t, &starformSuite{})
}

func (s *starformSuite) TestExecutorConfigValidate(c *tc.C) {
	config := validExecutorConfig()
	c.Assert(config.Validate(), tc.ErrorIsNil)

	config.Scriptlet = domainunitless.Scriptlet{}
	err := config.Validate()
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
	c.Check(err, tc.ErrorMatches, "no scriptlet sources not valid")

	config = validExecutorConfig()
	config.MaxAllocs = -1
	err = config.Validate()
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
	c.Check(err, tc.ErrorMatches, "negative MaxAllocs not valid")

	config = validExecutorConfig()
	config.MaxSteps = -1
	err = config.Validate()
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
	c.Check(err, tc.ErrorMatches, "negative MaxSteps not valid")

	config = validExecutorConfig()
	config.Logger = nil
	err = config.Validate()
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
	c.Check(err, tc.ErrorMatches, "nil Logger not valid")
}

func (s *starformSuite) TestNewStarformExecutorRejectsInvalidConfig(c *tc.C) {
	config := validExecutorConfig()
	config.Logger = nil

	executor, err := NewStarformExecutor(c.Context(), config)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
	c.Check(executor, tc.IsNil)
}

func (s *starformSuite) TestHandleCollectsIntents(c *tc.C) {
	executor, err := NewStarformExecutor(c.Context(), ExecutorConfig{
		Scriptlet: domainunitless.Scriptlet{
			Sources: []domainunitless.ScriptSource{{
				LoadPath: "hooks.star",
				Source: `
def init():
    juju.observe("config_changed", on_config_changed)

def on_config_changed(event):
    juju.set_status("active", message=event.message)
`,
			}},
		},
		MaxAllocs: defaultMaxAllocs,
		MaxSteps:  defaultMaxSteps,
		Logger:    NewStarformLogAdapter(newRecordingLogger()),
	})
	c.Assert(err, tc.ErrorIsNil)

	intents, err := executor.Handle(c.Context(), domainunitless.Event{
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
		Type: IntentSetStatus,
		Args: map[string]any{
			"status":  "active",
			"message": "updated",
		},
	}})
}

func (s *starformSuite) TestHandleScriptErrorDiscardsIntents(c *tc.C) {
	executor, err := NewStarformExecutor(c.Context(), ExecutorConfig{
		Scriptlet: domainunitless.Scriptlet{
			Sources: []domainunitless.ScriptSource{{
				LoadPath: "hooks.star",
				Source: `
def init():
    juju.observe("config_changed", on_config_changed)

def on_config_changed(event):
    juju.set_status("active", message="before failure")
    1 / 0
`,
			}},
		},
		MaxAllocs: defaultMaxAllocs,
		MaxSteps:  defaultMaxSteps,
		Logger:    NewStarformLogAdapter(newRecordingLogger()),
	})
	c.Assert(err, tc.ErrorIsNil)

	intents, err := executor.Handle(c.Context(), domainunitless.Event{Name: "config_changed"})
	c.Assert(err, tc.ErrorMatches, `.*division by zero.*`)
	c.Check(intents, tc.IsNil)
}

func (s *starformSuite) TestValueToStarlarkConvertsTypedSlices(c *tc.C) {
	value, err := valueToStarlark([]map[string]any{{
		"names":   []string{"juju", "unitless"},
		"enabled": true,
	}, {
		"ports": []int{80, 443},
	}})
	c.Assert(err, tc.ErrorIsNil)

	list, ok := value.(*starlark.List)
	c.Assert(ok, tc.IsTrue)
	c.Assert(list.Len(), tc.Equals, 2)

	first, ok := list.Index(0).(*starlark.Dict)
	c.Assert(ok, tc.IsTrue)
	names, found, err := first.Get(starlark.String("names"))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(found, tc.IsTrue)
	c.Check(names.String(), tc.Equals, `["juju", "unitless"]`)

	second, ok := list.Index(1).(*starlark.Dict)
	c.Assert(ok, tc.IsTrue)
	ports, found, err := second.Get(starlark.String("ports"))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(found, tc.IsTrue)
	c.Check(ports.String(), tc.Equals, `[80, 443]`)
}

func (s *starformSuite) TestValueToStarlarkConvertsTypedMaps(c *tc.C) {
	value, err := valueToStarlark(map[string][]string{
		"names": {"juju", "unitless"},
	})
	c.Assert(err, tc.ErrorIsNil)

	dict, ok := value.(*starlark.Dict)
	c.Assert(ok, tc.IsTrue)
	names, found, err := dict.Get(starlark.String("names"))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(found, tc.IsTrue)
	c.Check(names.String(), tc.Equals, `["juju", "unitless"]`)

	value, err = valueToStarlark(map[string]map[string]bool{
		"features": {
			"enabled": true,
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	dict, ok = value.(*starlark.Dict)
	c.Assert(ok, tc.IsTrue)
	featuresValue, found, err := dict.Get(starlark.String("features"))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(found, tc.IsTrue)
	features, ok := featuresValue.(*starlark.Dict)
	c.Assert(ok, tc.IsTrue)
	enabled, found, err := features.Get(starlark.String("enabled"))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(found, tc.IsTrue)
	c.Check(enabled, tc.Equals, starlark.True)
}

func (s *starformSuite) TestValueToStarlarkRejectsNonStringMapKeys(c *tc.C) {
	_, err := valueToStarlark(map[int]string{1: "one"})
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
	c.Check(err, tc.ErrorMatches, `unsupported map key type int.*`)
}

func validExecutorConfig() ExecutorConfig {
	return ExecutorConfig{
		Scriptlet: domainunitless.Scriptlet{
			Sources: []domainunitless.ScriptSource{{
				LoadPath: "hooks.star",
				Source:   "def init(): pass",
			}},
		},
		MaxAllocs: defaultMaxAllocs,
		MaxSteps:  defaultMaxSteps,
		Logger:    NewStarformLogAdapter(newRecordingLogger()),
	}
}
