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

// converter converts an arbitrary object into a []byte.
type converter func(value interface{}) ([]byte, error)

// convertYaml marshals value to a yaml-formatted []byte, unless value is nil.
func convertYaml(value interface{}) ([]byte, error) {
	if value == nil {
		return nil, nil
	}
	return goyaml.Marshal(value)
}

// defaultConverters are used by many jujuc Commands.
var defaultConverters = map[string]converter{
	"yaml": convertYaml,
	"json": json.Marshal,
}

// converterValue implements gnuflag.Value for a --format flag.
type converterValue struct {
	name       string
	converters map[string]converter
}

// newConverterValue returns a new converterValue. The initial converter name
// must be present in converters.
func newConverterValue(initial string, converters map[string]converter) *converterValue {
	v := &converterValue{converters: converters}
	if err := v.Set(initial); err != nil {
		panic(err)
	}
	return v
}

// Set stores the chosen converter name in v.name.
func (v *converterValue) Set(value string) error {
	if v.converters[value] == nil {
		return fmt.Errorf("unknown format: %s", value)
	}
	v.name = value
	return nil
}

// String returns the chosen converter name.
func (v *converterValue) String() string {
	return v.name
}

// doc returns documentation for the --format flag.
func (v *converterValue) doc() string {
	choices := make([]string, len(v.converters))
	i := 0
	for name := range v.converters {
		choices[i] = name
		i++
	}
	sort.Strings(choices)
	return "specify output format (" + strings.Join(choices, "|") + ")"
}

// convert runs the chosen converter on value.
func (v *converterValue) convert(value interface{}) ([]byte, error) {
	return v.converters[v.name](value)
}

// isTruthy return false if value is nil, false, 0, or an empty array/map/slice/string.
func isTruthy(value interface{}) bool {
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Bool:
		return v.Bool()
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return v.Len() != 0
	case reflect.Float32, reflect.Float64:
		return v.Float() != 0
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() != 0
	case reflect.Uint, reflect.Uintptr, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return v.Uint() != 0
	case reflect.Interface, reflect.Ptr:
		return !v.IsNil()
	}
	return true
}

// formatter exposes flags allowing the user to control the format and
// target of a Command's output.
type formatter struct {
	converter *converterValue
	outPath   string
	testMode  bool
}

// addFlags injects appropriate command line flags into f.
func (fm *formatter) addFlags(f *gnuflag.FlagSet, name string, converters map[string]converter) {
	fm.converter = newConverterValue(name, converters)
	f.Var(fm.converter, "format", fm.converter.doc())
	f.StringVar(&fm.outPath, "o", "", "specify an output file")
	f.StringVar(&fm.outPath, "output", "", "")
	f.BoolVar(&fm.testMode, "test", false, "suppress output; communicate result truthiness in return code")
}

// run communicates value to the user, as requested on the command line.
func (fm *formatter) run(ctx *cmd.Context, value interface{}) (err error) {
	if fm.testMode {
		if isTruthy(value) {
			return nil
		} else {
			return cmd.ErrSilent
		}
	}
	var target io.Writer
	if fm.outPath == "" {
		target = ctx.Stdout
	} else {
		path := ctx.AbsPath(fm.outPath)
		if target, err = os.Create(path); err != nil {
			return
		}
	}
	bytes, err := fm.converter.convert(value)
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
