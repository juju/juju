// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the LGPLv3, see LICENSE file for details.

package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/juju/gnuflag"
	goyaml "gopkg.in/yaml.v2"
)

// Formatter writes the arbitrary object into the writer.
type Formatter func(writer io.Writer, value interface{}) error

// FormatYaml writes out value as yaml to the writer, unless value is nil.
func FormatYaml(writer io.Writer, value interface{}) error {
	if value == nil {
		return nil
	}
	result, err := goyaml.Marshal(value)
	if err != nil {
		return err
	}
	for i := len(result) - 1; i > 0; i-- {
		if result[i] != '\n' {
			break
		}
		result = result[:i]
	}

	if len(result) > 0 {
		result = append(result, '\n')
		_, err = writer.Write(result)
		return err
	}
	return nil
}

// FormatJson writes out value as json.
func FormatJson(writer io.Writer, value interface{}) error {
	result, err := json.Marshal(value)
	if err != nil {
		return err
	}
	result = append(result, '\n')
	_, err = writer.Write(result)
	return err
}

// FormatSmart marshals value into a []byte according to the following rules:
//   * string:        untouched
//   * bool:          converted to `True` or `False` (to match pyjuju)
//   * int or float:  converted to sensible strings
//   * []string:      joined by `\n`s into a single string
//   * anything else: delegate to FormatYaml
func FormatSmart(writer io.Writer, value interface{}) error {
	if value == nil {
		return nil
	}
	valueStr := ""
	switch value := value.(type) {
	case string:
		valueStr = value
	case []string:
		valueStr = strings.Join(value, "\n")
	case bool:
		if value {
			valueStr = "True"
		} else {
			valueStr = "False"
		}
	default:
		return FormatYaml(writer, value)
	}
	if valueStr == "" {
		return nil
	}
	_, err := writer.Write([]byte(valueStr + "\n"))
	return err
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
	return "Specify output format (" + strings.Join(choices, "|") + ")"
}

// format runs the chosen formatter on value.
func (v *formatterValue) format(writer io.Writer, value interface{}) error {
	return v.formatters[v.name](writer, value)
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
	f.StringVar(&c.outPath, "o", "", "Specify an output file")
	f.StringVar(&c.outPath, "output", "", "")
}

// Write formats and outputs the value as directed by the --format and
// --output command line flags.
func (c *Output) Write(ctx *Context, value interface{}) (err error) {
	formatterName := c.formatter.name
	formatter := c.formatter.formatters[formatterName]
	// If the formatter is not one of the default ones, add a new line at the end.
	// This keeps consistent behaviour with the current code.
	var newline bool
	if _, found := DefaultFormatters[formatterName]; !found {
		newline = true
	}
	if err := c.writeFormatter(ctx, formatter, value, newline); err != nil {
		return err
	}
	return nil
}

// WriteFormatter formats and outputs the value with the given formatter,
// to the output directed by the --output command line flag.
func (c *Output) WriteFormatter(ctx *Context, formatter Formatter, value interface{}) (err error) {
	return c.writeFormatter(ctx, formatter, value, false)
}

func (c *Output) writeFormatter(ctx *Context, formatter Formatter, value interface{}, newline bool) (err error) {
	var target io.Writer
	if c.outPath == "" {
		target = ctx.Stdout
	} else {
		path := ctx.AbsPath(c.outPath)
		var f *os.File
		if f, err = os.Create(path); err != nil {
			return
		}
		defer f.Close()
		target = f
	}
	if err := formatter(target, value); err != nil {
		return err
	}
	if newline {
		fmt.Fprintln(target)
	}
	return nil
}

func (c *Output) Name() string {
	return c.formatter.name
}
