// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package runner

import (
	"github.com/juju/names"
)

// ActionData contains the tag, parameters, and results of an Action.
type ActionData struct {
	Name           string
	Tag            names.ActionTag
	Params         map[string]interface{}
	Failed         bool
	ResultsMessage string
	ResultsMap     map[string]interface{}
}

// NewActionData builds a suitable ActionData struct with no nil members.
// this should only be called in the event that an Action hook is being requested.
func newActionData(name string, tag *names.ActionTag, params map[string]interface{}) *ActionData {
	return &ActionData{
		Name:       name,
		Tag:        *tag,
		Params:     params,
		ResultsMap: map[string]interface{}{},
	}
}

// actionStatus messages define the possible states of a completed Action.
const (
	actionStatusInit   = "init"
	actionStatusFailed = "fail"
)

// addValueToMap adds the given value to the map on which the method is run.
// This allows us to merge maps such as {foo: {bar: baz}} and {foo: {baz: faz}}
// into {foo: {bar: baz, baz: faz}}.
func addValueToMap(keys []string, value string, target map[string]interface{}) {
	next := target

	for i := range keys {
		// if we are on last key set the value.
		// shouldn't be a problem.  overwrites existing vals.
		if i == len(keys)-1 {
			next[keys[i]] = value
			break
		}

		if iface, ok := next[keys[i]]; ok {
			switch typed := iface.(type) {
			case map[string]interface{}:
				// If we already had a map inside, keep
				// stepping through.
				next = typed
			default:
				// If we didn't, then overwrite value
				// with a map and iterate with that.
				m := map[string]interface{}{}
				next[keys[i]] = m
				next = m
			}
			continue
		}

		// Otherwise, it wasn't present, so make it and step
		// into.
		m := map[string]interface{}{}
		next[keys[i]] = m
		next = m
	}
}
