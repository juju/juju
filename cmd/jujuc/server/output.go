package server

import (
	"encoding/json"
	"fmt"
	"io"
	"launchpad.net/gnuflag"
	"launchpad.net/juju/go/cmd"
	"os"
	"sort"
	"strings"
)

// converter converts an arbitrary object into a []byte.
type converter func(value interface{}) ([]byte, error)

// convertSmart converts value to a byte array containing its string
// representation. The output is therefore golang-specific, just as
// the python "smart" format produces python-specific output.
func convertSmart(value interface{}) ([]byte, error) {
	if value == nil {
		return nil, nil
	}
	return []byte(fmt.Sprint(value)), nil
}

// defaultConverters are used by many jujuc Commands.
var defaultConverters = map[string]converter{
	"smart": convertSmart,
	"json":  json.Marshal,
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

// resultWriter exposes flags allowing the user to control the format and
// target of a Command's output.
type resultWriter struct {
	converter *converterValue
	outPath   string
}

// addFlags injects appropriate command line flags into f.
func (rw *resultWriter) addFlags(f *gnuflag.FlagSet, name string, converters map[string]converter) {
	rw.converter = newConverterValue(name, converters)
	f.Var(rw.converter, "format", rw.converter.doc())
	f.StringVar(&rw.outPath, "o", "", "specify an output file")
	f.StringVar(&rw.outPath, "output", "", "")
}

// write converts value, and writes it out, as requested on the command line.
func (rw *resultWriter) write(ctx *cmd.Context, value interface{}) (err error) {
	var target io.Writer
	if rw.outPath == "" {
		target = ctx.Stdout
	} else {
		path := ctx.AbsPath(rw.outPath)
		if target, err = os.Create(path); err != nil {
			return
		}
	}
	var bytes []byte
	bytes, err = rw.converter.convert(value)
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
