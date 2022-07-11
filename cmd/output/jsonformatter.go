// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

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

// JSONFormatter is a custom formatter that is used to custom format parsed input.
type JSONFormatter struct {
	// Colors a list of colors that the formatter uses for writing output.
	Colors
	// Number of spaces before the first string is printed.
	Indent int
	// writer writes formatted output to the internal buffer (buff).
	writer *ansiterm.Writer
	// buff is the internal buffer used by writer to write out ansi-color formatted strings.
	buff *bytes.Buffer
	// InitialDepth used as multiplier for the number of spaces to be used for indentation.
	InitialDepth int
	// RawStrings enable parsing as json raw strings
	RawStrings bool
}

// NewFormatter instantiates a new formatter with default options.
func NewFormatter() *JSONFormatter {
	buff, writer := bufferedWriter()
	return &JSONFormatter{
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

func (f *JSONFormatter) writeIndent(buf *bytes.Buffer, depth int) {
	buf.WriteString(strings.Repeat(" ", f.Indent*depth))
}

func (f *JSONFormatter) writeObjSep(buf *bytes.Buffer) {
	if f.Indent != 0 {
		buf.WriteByte('\n')
	}
}

// Marshal traverses the value jsonObj recursively, encoding each field and appending it
// with ansi escape sequence for colored output.
func (f *JSONFormatter) Marshal(jsonObj interface{}) ([]byte, error) {
	buffer := bytes.Buffer{}
	f.marshalValue(jsonObj, &buffer, f.InitialDepth)
	return buffer.Bytes(), nil
}

func (f *JSONFormatter) marshalString(str string, buf *bytes.Buffer) {
	if !f.RawStrings {
		strBytes, _ := json.Marshal(str)
		str = string(strBytes)
	}

	f.Colors.String.Fprint(f.writer, str)
	buf.WriteString(f.buff.String())
	f.buff.Reset()
}

func (f *JSONFormatter) marshalMap(m map[string]interface{}, buf *bytes.Buffer, depth int) {
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

func (f *JSONFormatter) marshalArray(a []interface{}, buf *bytes.Buffer, depth int) {
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

func (f *JSONFormatter) marshalValue(val interface{}, buf *bytes.Buffer, depth int) {

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
	default:
		b, _ := json.Marshal(val)
		var m interface{}
		_ = json.Unmarshal(b, &m)
		f.marshalValue(m, buf, depth)
	}
}
