package output

import (
	"bytes"
	"encoding/json"
	"sort"
	"strconv"
	"strings"

	"github.com/juju/ansiterm"
)

const (
	valueSep   = ","
	null       = "null"
	beginMap   = "{"
	endMap     = "}"
	beginArray = "["
	endArray   = "]"
	emptyMap   = "{}"
	emptyArray = "[]"
)

// Colors holds Color for each of the JSON data types.
type Colors struct {
	// Null is the Color for JSON nil.
	Null *ansiterm.Context
	// Bool is the Color for boolean values.
	Bool *ansiterm.Context
	// Number is the Color for number values.
	Number *ansiterm.Context
	// String is the Color for string values.
	String *ansiterm.Context
	// Key is the Color for JSON keys.
	Key *ansiterm.Context
	//KeyValSep separates key from values.
	KeyValSep *ansiterm.Context
	//InitialDepth used as multiplier for the number of spaces to be used for indentation.
	InitialDepth int
	// RawStrings enable parsing as json raw strings
	RawStrings bool
}

// Formatter is a custom formatter that is used to custom format parsed input.
type Formatter struct {
	// Colors a list of colors that the formatter uses for writing output.
	Colors
	// Number of spaces before the first string is printed.
	Indent int
	// writer writes formatted output to the internal buffer (buff).
	writer *ansiterm.Writer
	// buff is the internal buffer used by writer to write out ansi-color formatted strings.
	buff *bytes.Buffer
}

// NewFormatter instantiates a new formatter with default options.
func NewFormatter() *Formatter {
	buff, writer := bufferedWriter()
	return &Formatter{
		Colors: Colors{
			Null:      ansiterm.Foreground(ansiterm.Magenta),
			Key:       ansiterm.Foreground(ansiterm.BrightCyan).SetStyle(ansiterm.Bold),
			Bool:      ansiterm.Foreground(ansiterm.Magenta),
			Number:    ansiterm.Foreground(ansiterm.Magenta),
			KeyValSep: ansiterm.Foreground(ansiterm.BrightMagenta),
			String:    ansiterm.Foreground(ansiterm.Default),
		},
		writer: writer,
		buff:   buff,
	}
}

func bufferedWriter() (*bytes.Buffer, *ansiterm.Writer) {
	buff := &bytes.Buffer{}
	writer := ansiterm.NewWriter(buff)
	writer.SetColorCapable(true)
	return buff, writer
}

// marshal JSON data with default options
func marshal(jsonObj interface{}) ([]byte, error) {
	return NewFormatter().Marshal(jsonObj)
}

func (f *Formatter) writeIndent(buf *bytes.Buffer, depth int) {
	buf.WriteString(strings.Repeat(" ", f.Indent*depth))
}

func (f *Formatter) writeObjSep(buf *bytes.Buffer) {
	if f.Indent != 0 {
		buf.WriteByte('\n')
	}
}

func (f *Formatter) Marshal(jsonObj interface{}) ([]byte, error) {
	buffer := bytes.Buffer{}
	f.marshalValue(jsonObj, &buffer, f.InitialDepth)
	return buffer.Bytes(), nil
}

func (f *Formatter) marshalString(str string, buf *bytes.Buffer) {
	if !f.RawStrings {
		strBytes, _ := json.Marshal(str)
		str = string(strBytes)
	}

	f.Colors.String.Fprint(f.writer, str)
	buf.WriteString(f.buff.String())
	f.buff.Reset()
}

func (f *Formatter) marshalMap(m map[string]interface{}, buf *bytes.Buffer, depth int) {
	remaining := len(m)

	if remaining == 0 {
		buf.WriteString(emptyMap)
		return
	}

	keys := make([]string, 0)
	for key := range m {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	buf.WriteString(beginMap)
	f.writeObjSep(buf)

	f.Colors.KeyValSep.Fprint(f.writer, ":")
	keyValSep := f.buff.String()
	f.buff.Reset()

	for _, key := range keys {
		f.writeIndent(buf, depth+1)
		f.Colors.Key.Fprintf(f.writer, "\"%s\"%s", key, keyValSep)
		buf.WriteString(f.buff.String())
		f.buff.Reset()
		f.marshalValue(m[key], buf, depth+1)
		remaining--
		if remaining != 0 {
			buf.WriteString(valueSep)
		}
		f.writeObjSep(buf)
	}
	f.writeIndent(buf, depth)
	buf.WriteString(endMap)
}

func (f *Formatter) marshalArray(a []interface{}, buf *bytes.Buffer, depth int) {
	if len(a) == 0 {
		buf.WriteString(emptyArray)
		return
	}

	buf.WriteString(beginArray)
	f.writeObjSep(buf)

	for i, v := range a {
		f.writeIndent(buf, depth+1)
		f.marshalValue(v, buf, depth+1)
		if i < len(a)-1 {
			buf.WriteString(valueSep)
		}
		f.writeObjSep(buf)
	}
	f.writeIndent(buf, depth)
	buf.WriteString(endArray)
}

func (f *Formatter) marshalValue(val interface{}, buf *bytes.Buffer, depth int) {
	switch v := val.(type) {
	case map[string]interface{}:
		f.marshalMap(v, buf, depth)
	case []interface{}:
		f.marshalArray(v, buf, depth)
	case string:
		f.marshalString(v, buf)
	case float64:
		f.Colors.Number.Fprint(f.writer, strconv.FormatFloat(v, 'f', -1, 64))
		buf.WriteString(f.buff.String())
		f.buff.Reset()
	case bool:
		f.Colors.Bool.Fprint(f.writer, strconv.FormatBool(v))
		buf.WriteString(f.buff.String())
		f.buff.Reset()
	case nil:
		f.Colors.Null.Fprint(f.writer, null)
		buf.WriteString(f.buff.String())
		f.buff.Reset()
	case json.Number:
		f.Colors.Number.Fprint(f.writer, v.String())
		buf.WriteString(f.buff.String())
		f.buff.Reset()
	}
}
