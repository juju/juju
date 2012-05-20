package server

import (
	"encoding/json"
	"fmt"
	"io"
	"launchpad.net/gnuflag"
	"launchpad.net/goyaml"
	"launchpad.net/juju/go/cmd"
	"os"
	"reflect"
	"sort"
	"strings"
)

// formatter converts an arbitrary object into a []byte.
type formatter func(value interface{}) ([]byte, error)

// formatYaml marshals value to a yaml-formatted []byte, unless value is nil.
func formatYaml(value interface{}) ([]byte, error) {
	if value == nil {
		return nil, nil
	}
	return goyaml.Marshal(value)
}

// defaultFormatters are used by many jujuc Commands.
var defaultFormatters = map[string]formatter{
	"yaml": formatYaml,
	"json": json.Marshal,
}

// formatterValue implements gnuflag.Value for the --format flag.
type formatterValue struct {
	name       string
	formatters map[string]formatter
}

// newFormatterValue returns a new formatterValue. The initial formatter name
// must be present in formatters.
func newFormatterValue(initial string, formatters map[string]formatter) *formatterValue {
	v := &formatterValue{formatters: formatters}
	if err := v.Set(initial); err != nil {
		panic(err)
	}
	return v
}

// Set stores the chosen formatter name in v.name.
func (v *formatterValue) Set(value string) error {
	if v.formatters[value] == nil {
		return fmt.Errorf("unknown format: %s", value)
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

// output is responsible for interpreting output-related command line flags
// and writing a value to a file or to stdout as directed. The testMode field,
// controlled by the --test flag, is used to indicate that output should be
// suppressed and communicated entirely in the process exit code.
type output struct {
	formatter *formatterValue
	outPath   string
	testMode  bool
}

// addFlags injects appropriate command line flags into f.
func (c *output) addFlags(f *gnuflag.FlagSet, name string, formatters map[string]formatter) {
	c.formatter = newFormatterValue(name, formatters)
	f.Var(c.formatter, "format", c.formatter.doc())
	f.StringVar(&c.outPath, "o", "", "specify an output file")
	f.StringVar(&c.outPath, "output", "", "")
	f.BoolVar(&c.testMode, "test", false, "returns non-zero exit code if value is false/zero/empty")
}

// write formats and outputs value as directed by the --format and --output
// command line flags.
func (c *output) write(ctx *cmd.Context, value interface{}) (err error) {
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
	if bytes != nil {
		_, err = target.Write(bytes)
		if err == nil {
			_, err = target.Write([]byte{'\n'})
		}
	}
	return
}

// truthError returns cmd.ErrSilent if value is nil, false, or 0, or an empty
// array, map, slice, or string.
func truthError(value interface{}) error {
	b := true
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Invalid:
		b = false
	case reflect.Bool:
		b = v.Bool()
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		b = v.Len() != 0
	case reflect.Float32, reflect.Float64:
		b = v.Float() != 0
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		b = v.Int() != 0
	case reflect.Uint, reflect.Uintptr, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		b = v.Uint() != 0
	case reflect.Interface, reflect.Ptr:
		b = !v.IsNil()
	}
	if b {
		return nil
	}
	return cmd.ErrSilent
}
