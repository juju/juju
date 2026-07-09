// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unitless

import (
	"context"
	"reflect"
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
		App:            newAppObject(),
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

func newAppObject() *starform.AppObject {
	return &starform.AppObject{
		Name: "juju",
		Methods: []*starlark.Builtin{
			// TODO (manadart 2026-07-06): There will be two builtin types.
			// 1) Those that append intents (like this one), which will be
			//    reusable in agents and probably live in a scriptlet domain.
			// 2) Those that query external state, and will *not* be common.
			//    This is because agents will need to go via an API, and
			//    server-side workers directly via domain services.
			// I forsee a visitor defined in the domain, which will add all
			// the intent builtins to a script set.
			setStatusBuiltin,
		},
	}
}

type starformExecutor struct {
	scriptSet *starform.ScriptSet
}

// Handle handles a single Event, by dispatching it to the Scriptlet associated
// with the event name, and returning the resulting intents.
func (e *starformExecutor) Handle(ctx context.Context, event Event) ([]Intent, error) {
	attrs, err := attrsToStarlark(event.Attrs)
	if err != nil {
		return nil, errors.Errorf("serialising event attrs: %w", err)
	}
	collector := IntentCollector{}
	eo := starform.EventObject{
		Name:  event.Name,
		Attrs: attrs,
		State: &collector,
	}

	if err := e.scriptSet.Handle(ctx, &eo); err != nil {
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
	default:
		reflected := reflect.ValueOf(value)
		switch reflected.Kind() {
		case reflect.Map:
			return mapToStarlark(reflected)
		case reflect.Slice:
			return sliceToStarlark(reflected)
		}
		return nil, errors.Errorf("unsupported value type %T", value).Add(coreerrors.NotValid)
	}
}

// mapToStarlark converts a valid map to a starlark.Dict.
// A valid map at this point means string keys.
func mapToStarlark(value reflect.Value) (starlark.Value, error) {
	if value.Type().Key().Kind() != reflect.String {
		return nil, errors.Errorf("unsupported map key type %s", value.Type().Key()).Add(coreerrors.NotValid)
	}

	dict := starlark.NewDict(value.Len())
	keys := value.MapKeys()
	sort.Slice(keys, func(i, j int) bool {
		return keys[i].String() < keys[j].String()
	})
	for _, key := range keys {
		item, err := valueToStarlark(value.MapIndex(key).Interface())
		if err != nil {
			return nil, errors.Errorf("%q: %w", key.String(), err)
		}
		if err := dict.SetKey(starlark.String(key.String()), item); err != nil {
			return nil, err
		}
	}
	return dict, nil
}

func sliceToStarlark(value reflect.Value) (starlark.Value, error) {
	result := make([]starlark.Value, value.Len())
	for i := 0; i < value.Len(); i++ {
		item, err := valueToStarlark(value.Index(i).Interface())
		if err != nil {
			return nil, errors.Errorf("[%d]: %w", i, err)
		}
		result[i] = item
	}
	return starlark.NewList(result), nil
}
