package server

import (
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
type converters map[string]converter

// convertSmart is an output converter which is not as smart as it sounds.
func convertSmart(value interface{}) ([]byte, error) {
	if value == nil {
		return nil, nil
	}
	// In python, we'd be returning str(value)... but it seems moderately
	// insane to spend any time at all writing a python stringifier for go.
	return []byte(fmt.Sprintf("%v", value)), nil
}

// converterValue implements gnuflag.Value for a --format flag.
type converterValue struct {
	value      string
	converters converters
}

// newConverterValue returns a new converterValue. The initial converter name
// must be present in converters.
func newConverterValue(initial string, converters converters) *converterValue {
	v := &converterValue{converters: converters}
	if err := v.Set(initial); err != nil {
		panic(err)
	}
	return v
}

// Set stores the chosen converter name in v.value.
func (v *converterValue) Set(value string) error {
	if _, found := v.converters[value]; !found {
		return fmt.Errorf("unknown format: %s", value)
	}
	v.value = value
	return nil
}

// String returns the chosen converter name.
func (v *converterValue) String() string {
	return v.value
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
	return "specify output format (%s)" + strings.Join(choices, "|")
}

// write uses the chosen converter to convert value to a []byte, and writes it
// to target.
func (v *converterValue) write(target io.Writer, value interface{}) (err error) {
	bytes, err := v.converters[v.value](value)
	if err != nil {
		return
	}
	if bytes != nil {
		bytes = append(bytes, byte('\n'))
		_, err = target.Write(bytes)
	}
	return
}

// resultWriter is responsible for writing command output, and exposes flags
// to allow the user to specify format and target.
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
		if target, err = os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0644); err != nil {
			return
		}
	}
	return rw.converter.write(target, value)
}
