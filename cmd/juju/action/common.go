// Copyright 2014-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names"
	"gopkg.in/yaml.v1"

	"github.com/juju/juju/apiserver/params"
)

// conform ensures all keys of any nested maps are strings.  This is
// necessary because YAML unmarshals map[interface{}]interface{} in nested
// maps, which cannot be serialized by bson. Also, handle []interface{}.
// cf. gopkg.in/juju/charm.v4/actions.go cleanse
func conform(input interface{}) (interface{}, error) {
	switch typedInput := input.(type) {

	case map[string]interface{}:
		newMap := make(map[string]interface{})
		for key, value := range typedInput {
			newValue, err := conform(value)
			if err != nil {
				return nil, err
			}
			newMap[key] = newValue
		}
		return newMap, nil

	case map[interface{}]interface{}:
		newMap := make(map[string]interface{})
		for key, value := range typedInput {
			typedKey, ok := key.(string)
			if !ok {
				return nil, errors.New("map keyed with non-string value")
			}
			newMap[typedKey] = value
		}
		return conform(newMap)

	case []interface{}:
		newSlice := make([]interface{}, len(typedInput))
		for i, sliceValue := range typedInput {
			newSliceValue, err := conform(sliceValue)
			if err != nil {
				return nil, errors.New("map keyed with non-string value")
			}
			newSlice[i] = newSliceValue
		}
		return newSlice, nil

	default:
		return input, nil
	}
}

// displayActionResult returns any error from an ActionResult and displays
// its response values otherwise.
func displayActionResult(result params.ActionResult, ctx *cmd.Context, out cmd.Output) error {
	if result.Error != nil {
		return result.Error
	}

	if result.Action == nil {
		return errors.New("action for result was nil")
	}

	output, err := yaml.Marshal(result.Output)
	if err != nil {
		return err
	}

	response := struct {
		Action  string
		Target  string
		Status  string
		Message string
		Results string
	}{
		Action:  result.Action.Name,
		Target:  result.Action.Receiver,
		Status:  result.Status,
		Message: result.Message,
		Results: string(output),
	}

	err = out.Write(ctx, response)
	if err != nil {
		return err
	}

	return nil
}

// getActionTagFromPrefix uses the APIClient to get an ActionTag from a prefix.
func getActionTagFromPrefix(api APIClient, prefix string) (names.ActionTag, error) {
	tag := names.ActionTag{}
	tags, err := api.FindActionTagsByPrefix(params.FindTags{Prefixes: []string{prefix}})
	if err != nil {
		return tag, err
	}

	results, ok := tags.Matches[prefix]
	if !ok || len(results) < 1 {
		return tag, errors.Errorf("actions for identifier %q not found", prefix)
	}

	actiontags, rejects := getActionTags(results)
	if len(rejects) > 0 {
		return tag, errors.Errorf("identifier %q got unrecognized entity tags %v", prefix, rejects)
	}

	if len(actiontags) > 1 {
		return tag, errors.Errorf("identifier %q matched multiple actions %v", prefix, actiontags)
	}

	tag = actiontags[0]
	return tag, nil
}

// getActionTags converts a slice of params.Entity to a slice of names.ActionTag, and
// also populates a slice of strings for the params.Entity.Tag that are not a valid
// names.ActionTag.
func getActionTags(entities []params.Entity) (good []names.ActionTag, bad []string) {
	for _, entity := range entities {
		if tag, err := entityToActionTag(entity); err != nil {
			bad = append(bad, entity.Tag)
		} else {
			good = append(good, tag)
		}
	}
	return
}

// entityToActionTag converts the params.Entity type to a names.ActionTag
func entityToActionTag(entity params.Entity) (names.ActionTag, error) {
	return names.ParseActionTag(entity.Tag)
}

// addValueToMap adds the given value to the map on which the method is run.
// This allows us to merge maps such as {foo: {bar: baz}} and {foo: {baz: faz}}
// into {foo: {bar: baz, baz: faz}}.
func addValueToMap(keys []string, value string, target map[string]interface{}) {
	next := target

	for i := range keys {
		// If we are on last key set or overwrite the val.
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
