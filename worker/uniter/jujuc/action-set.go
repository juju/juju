// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"fmt"
	"strings"

	"github.com/juju/cmd"
	"launchpad.net/gnuflag"
	"launchpad.net/goyaml"
)

// phrase is a struct representing a list of keys and a value to be added to
// a map, e.g.:
//  - outfile.format=bzip2
//  - person.home.address.number=123
//  - x=5
//  - "does not compute"
type phrase struct {
	keys  []string
	value interface{}
}

type nestedMap map[string]interface{}

// ActionSetCommand implements the relation-get command.
type ActionSetCommand struct {
	cmd.CommandBase
	ctx  Context
	args []string
}

// NewActionSetCommand returns a new ActionSetCommand with the given context.
func NewActionSetCommand(ctx Context) cmd.Command {
	return &ActionSetCommand{ctx: ctx}
}

// Info returns the content for --help.
func (c *ActionSetCommand) Info() *cmd.Info {
	doc := `
action-set commits the given map as the return value of the Action.  If a
bare value is given, it will be converted to a map.  This value will be
returned to the stateservice and client after completion of the Action.
Subsequent calls to action-set before completion of the Action will add the
values to the map, unless there is a conflict in which case the new value
will overwrite the old value.

Example usage:
 action-set outfile.size=10G
 action-set foo
 action-set foo.bar.baz=2 foo.bar.zab="3"
`
	return &cmd.Info{
		Name:    "action-set",
		Args:    "<values>",
		Purpose: "set action response",
		Doc:     doc,
	}
}

// SetFlags handles known option flags.
// TODO(binary132): add cmd.Input type as in cmd.Output for YAML piping.
func (c *ActionSetCommand) SetFlags(f *gnuflag.FlagSet) {
}

// Init currently accepts all input.  Malformed values will be rejected
// in the Run step.
func (c *ActionSetCommand) Init(args []string) error {
	c.args = args
	return nil
}

// Run adds the given phrases (such as foo.bar=baz) to the existing map of
// results for the Action.
func (c *ActionSetCommand) Run(ctx *cmd.Context) error {
	if len(c.args) == 0 {
		return nil
	}

	// c.ctx.ActionResults() will get us the current map of
	// action results, allowing us to add to it.
	results, alreadyFailed := c.ctx.ActionResults()
	if alreadyFailed {
		return nil
	}

	nm := nestedMap(results)
	for i, arg := range c.args {
		phraseSplit := strings.SplitN(arg, "=", 2)
		var toUnmarshal []byte
		newPhrase := phrase{keys: strings.Split(phraseSplit[0], ".")}
		if len(phraseSplit) == 1 {
			// If we had a bare value, then define the pair as
			// "val$i:foo"
			toUnmarshal = []byte(arg)
			newPhrase.keys[0] = fmt.Sprintf("val%d", i)
		} else {
			toUnmarshal = []byte(phraseSplit[1])
		}
		var yamlVal interface{}
		err := goyaml.Unmarshal([]byte(toUnmarshal), &yamlVal)
		if err != nil {
			c.ctx.ActionSetFailed(err)
			return nil
		}
		newPhrase.value = yamlVal
		nm.addPhrase(newPhrase)
	}

	c.ctx.ActionSetResults(map[string]interface{}(nm))
	return nil
}

// addPhrase adds the given phrase to the map on which the method is run.
// This allows us to merge maps such as {foo: {bar: baz}} and {foo: {baz: faz}}
// into {foo: {bar: baz, baz: faz}}.
func (nm nestedMap) addPhrase(p phrase) {
	next := nm
	k := p.keys
	v := p.value

	for i := range k {
		if i == len(k)-1 {
			next[k[i]] = v
			break
		}

		if iface, ok := next[k[i]]; ok {
			next = iface.(map[string]interface{})
		} else {
			m := map[string]interface{}{}
			next[k[i]] = m
			next = m
		}
	}
}

// TODO(binary132): Use this for YAML piping.
// coerceKeysToStrings rejects maps containing containin non-string keys, and coerces
// acceptable map[interface{}]interface{}'s to maps keyed by strings.
// This is necessary because goyaml unmarshals nested maps as map[i{}]i{},
// which is unsuitable for marshaling to BSON for storage in the stateserver.
func coerceKeysToStrings(input interface{}) (interface{}, error) {
	switch typedInput := input.(type) {

	// In this case, recurse in.
	case map[string]interface{}:
		newMap := make(map[string]interface{})
		for key, value := range typedInput {
			newValue, err := coerceKeysToStrings(value)
			if err != nil {
				return nil, err
			}
			newMap[key] = newValue
		}
		return newMap, nil

	// Coerce keys to strings and error out if there's a problem; then recurse.
	case map[interface{}]interface{}:
		newMap := make(map[string]interface{})
		for key, value := range typedInput {
			typedKey, ok := key.(string)
			if !ok {
				return nil, fmt.Errorf("map keyed with non-string value %#v", key)
			}
			newMap[typedKey] = value
		}
		return coerceKeysToStrings(newMap)

	// Recurse into any maps contained in lists
	case []interface{}:
		newSlice := make([]interface{}, 0)
		for _, sliceValue := range typedInput {
			newSliceValue, err := coerceKeysToStrings(sliceValue)
			if err != nil {
				return nil, err
			}
			newSlice = append(newSlice, newSliceValue)
		}
		return newSlice, nil

	// Other kinds of values are OK.
	default:
		return input, nil
	}
}
