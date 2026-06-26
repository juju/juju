// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unitless

import (
	"context"
	"sort"

	"github.com/canonical/starform/starform"
	"github.com/canonical/starlark/starlark"
	"github.com/juju/collections/transform"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/errors"
)

const requiredSafety = starlark.MemSafe | starlark.CPUSafe | starlark.TimeSafe | starlark.IOSafe

// ExecutorConfig is passed to an ExecutorFactory.
type ExecutorConfig struct {
	Scriptlet Scriptlet
	MaxAllocs int64
	MaxSteps  int64
	Logger    starform.Logger
}

// Executor handles event and returns the collected intents.
type Executor interface {
	Handle(context.Context, Event) ([]Intent, error)
}

// ExecutorFactory creates an executor for a loaded application scriptlet.
type ExecutorFactory func(context.Context, ExecutorConfig) (Executor, error)

// NewStarformExecutor creates an executor backed by a Starform ScriptSet.
func NewStarformExecutor(ctx context.Context, config ExecutorConfig) (Executor, error) {
	scriptlet := config.Scriptlet
	if err := scriptlet.Validate(); err != nil {
		return nil, errors.Capture(err)
	}
	maxAllocs := config.MaxAllocs
	if maxAllocs == 0 {
		maxAllocs = defaultMaxAllocs
	}
	maxSteps := config.MaxSteps
	if maxSteps == 0 {
		maxSteps = defaultMaxSteps
	}

	scriptSet, err := starform.NewScriptSet(&starform.ScriptSetOptions{
		App: &starform.AppObject{
			Name: scriptlet.AppName,
			Methods: []*starlark.Builtin{
				starlark.NewBuiltinWithSafety("status_set", requiredSafety, statusSet),
			},
		},
		Logger:         config.Logger,
		RequiredSafety: requiredSafety,
		MaxAllocs:      maxAllocs,
		MaxSteps:       maxSteps,
	})
	if err != nil {
		return nil, errors.Errorf("creating Starform script set: %w", err)
	}

	if err := scriptSet.LoadSources(ctx, transform.Slice(scriptlet.Sources, func(s ScriptSource) starform.ScriptSource {
		return s
	})); err != nil {
		return nil, errors.Errorf("loading Starform script sources: %w", err)
	}

	return &starformExecutor{scriptSet: scriptSet}, nil
}

type starformExecutor struct {
	scriptSet *starform.ScriptSet
}

// Handle handles a single Event, by dispatching it to the Scriptlet associated
// with the event name, and returning the resulting intents.
func (e *starformExecutor) Handle(ctx context.Context, event Event) ([]Intent, error) {
	if err := event.Validate(); err != nil {
		return nil, errors.Capture(err)
	}

	attrs, err := attrsToStarlark(event.Attrs)
	if err != nil {
		return nil, errors.Errorf("serialising event attrs: %w", err)
	}
	collector := new(IntentCollector)
	if err := e.scriptSet.Handle(ctx, &starform.EventObject{
		Name:  event.Name,
		Attrs: attrs,
		State: collector,
	}); err != nil {
		return nil, errors.Errorf("handling event %q: %w", event.Name, err)
	}
	return collector.Intents(), nil
}

func attrsToStarlark(attrs map[string]any) (starlark.StringDict, error) {
	result := make(starlark.StringDict, len(attrs))
	keys := make([]string, 0, len(attrs))
	for key := range attrs {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value, err := valueToStarlark(attrs[key])
		if err != nil {
			return nil, errors.Errorf("attr %q: %w", key, err)
		}
		result[key] = value
	}
	return result, nil
}

func valueToStarlark(value any) (starlark.Value, error) {
	switch v := value.(type) {
	case nil:
		return starlark.None, nil
	case starlark.Value:
		return v, nil
	case string:
		return starlark.String(v), nil
	case bool:
		return starlark.Bool(v), nil
	case int:
		return starlark.MakeInt(v), nil
	case int64:
		return starlark.MakeInt64(v), nil
	case float64:
		return starlark.Float(v), nil
	case map[string]string:
		dict := starlark.NewDict(len(v))
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if err := dict.SetKey(starlark.String(key), starlark.String(v[key])); err != nil {
				return nil, err
			}
		}
		return dict, nil
	case map[string]any:
		dict := starlark.NewDict(len(v))
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			value, err := valueToStarlark(v[key])
			if err != nil {
				return nil, errors.Errorf("%q: %w", key, err)
			}
			if err := dict.SetKey(starlark.String(key), value); err != nil {
				return nil, err
			}
		}
		return dict, nil
	default:
		return nil, errors.Errorf("unsupported value type %T", value).Add(coreerrors.NotValid)
	}
}
