// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"
	"sort"
	"strings"

	"launchpad.net/gnuflag"
	"launchpad.net/goyaml"
)

// Formatter converts an arbitrary object into a []byte.
type Formatter func(value interface{}) ([]byte, error)

// FormatYaml marshals value to a yaml-formatted []byte, unless value is nil.
func FormatYaml(value interface{}) ([]byte, error) {
	if value == nil {
		return nil, nil
	}
	result, err := goyaml.Marshal(value)
	if err != nil {
		return nil, err
	}
	for i := len(result) - 1; i > 0; i-- {
		if result[i] != '\n' {
			break
		}
		result = result[:i]
	}
	return result, nil
}

// FormatJson marshals value to a json-formatted []byte.
var FormatJson = json.Marshal

// FormatSmart marshals value into a []byte according to the following rules:
//   * string:        untouched
//   * bool:          converted to `True` or `False` (to match pyjuju)
//   * int or float:  converted to sensible strings
//   * []string:      joined by `\n`s into a single string
//   * anything else: delegate to FormatYaml
func FormatSmart(value interface{}) ([]byte, error) {
	if value == nil {
		return nil, nil
	}
	v := reflect.ValueOf(value)
	switch kind := v.Kind(); kind {
	case reflect.String:
		return []byte(value.(string)), nil
	case reflect.Array, reflect.Slice:
		if v.Type().Elem().Kind() == reflect.String {
			return []byte(strings.Join(value.([]string), "\n")), nil
		}
	case reflect.Bool:
		if value.(bool) {
			return []byte("True"), nil
		}
		return []byte("False"), nil
	case reflect.Map, reflect.Float32, reflect.Float64:
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
	default:
		return nil, fmt.Errorf("cannot marshal %#v", value)
	}
	return FormatYaml(value)
}

// DefaultFormatters holds the formatters that can be
// specified with the --format flag.
var DefaultFormatters = map[string]Formatter{
	"smart": FormatSmart,
	"yaml":  FormatYaml,
	"json":  FormatJson,
}

// formatterValue implements gnuflag.Value for the --format flag.
type formatterValue struct {
	name       string
	formatters map[string]Formatter
}

// newFormatterValue returns a new formatterValue. The initial Formatter name
// must be present in formatters.
func newFormatterValue(initial string, formatters map[string]Formatter) *formatterValue {
	v := &formatterValue{formatters: formatters}
	if err := v.Set(initial); err != nil {
		panic(err)
	}
	return v
}

// Set stores the chosen formatter name in v.name.
func (v *formatterValue) Set(value string) error {
	if v.formatters[value] == nil {
		return fmt.Errorf("unknown format %q", value)
	}
	v.name = value
	return nil
}

// String returns the chosen formatter name.
func (v *formatterValue) String() string {
	return v.name
}

// doc returns documentation for the --format flag.
func (v *formatterValue) doc() string {
	choices := make([]string, len(v.formatters))
	i := 0
	for name := range v.formatters {
		choices[i] = name
		i++
	}
	sort.Strings(choices)
	return "specify output format (" + strings.Join(choices, "|") + ")"
}

// format runs the chosen formatter on value.
func (v *formatterValue) format(value interface{}) ([]byte, error) {
	return v.formatters[v.name](value)
}

// Output is responsible for interpreting output-related command line flags
// and writing a value to a file or to stdout as directed.
type Output struct {
	formatter *formatterValue
	outPath   string
}

// AddFlags injects the --format and --output command line flags into f.
func (c *Output) AddFlags(f *gnuflag.FlagSet, defaultFormatter string, formatters map[string]Formatter) {
	c.formatter = newFormatterValue(defaultFormatter, formatters)
	f.Var(c.formatter, "format", c.formatter.doc())
	f.StringVar(&c.outPath, "o", "", "specify an output file")
	f.StringVar(&c.outPath, "output", "", "")
}

// Write formats and outputs the value as directed by the --format and
// --output command line flags.
func (c *Output) Write(ctx *Context, value interface{}) (err error) {
	var target io.Writer
	if c.outPath == "" {
		target = ctx.Stdout
	} else {
		path := ctx.AbsPath(c.outPath)
		if target, err = os.Create(path); err != nil {
			return
		}
	}
	bytes, err := c.formatter.format(value)
	if err != nil {
		return
	}
	if len(bytes) > 0 {
		_, err = target.Write(bytes)
		if err == nil {
			_, err = target.Write([]byte{'\n'})
		}
	}
	return
}

func (c *Output) Name() string {
	return c.formatter.name
}
