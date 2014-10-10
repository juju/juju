package action

import (
	"bytes"
	"fmt"
	"text/tabwriter"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/juju/apiserver/params"
	yaml "gopkg.in/yaml.v1"
)

// conform ensures all keys of any nested maps are strings.  This is
// necessary because YAML unmarshals map[interface{}]interface{} in nested
// maps, which cannot be serialized by bson.
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

	response := fmt.Sprintf("Action %q on %s finished:\n"+
		"Status: %s\n"+
		"Message: %s\n"+
		"Results:\n%s",
		result.Action.Name, result.Action.Receiver,
		result.Status,
		result.Message,
		string(output))

	err = out.Write(ctx, response)
	if err != nil {
		return err
	}

	return nil
}

// tabbedString returns a columnated string from a list of rows of two items,
// separated by sep.
func tabbedString(inputs [][]string, sep string) (string, error) {
	var b bytes.Buffer

	// Format in tab-separated columns with a tab stop of 8.
	w := new(tabwriter.Writer)
	w.Init(&b, 0, 8, 0, '\t', 0)
	for i, row := range inputs {
		if len(row) != 2 {
			return "", errors.Errorf("row must have only two items, got %#v", row)
		}
		if i == len(inputs)-1 {
			fmt.Fprintf(w, "%s\t%s%s", row[0], sep, row[1])
			continue
		}
		fmt.Fprintf(w, "%s\t%s%s\n", row[0], sep, row[1])
	}
	w.Flush()

	return b.String(), nil
}
