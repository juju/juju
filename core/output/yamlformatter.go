// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package output

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/juju/ansiterm"
	yamlv2 "gopkg.in/yaml.v2"
)

type YamlFormatter struct {
	// Colors a list of colors that the formatter uses for writing output.
	Colors
	// writer writes formatted output to the internal buffer (buff).
	writer *ansiterm.Writer
	// buff is the internal buffer used by writer to write out ansi-color formatted strings.
	buff *bytes.Buffer
	// UseIndentLines if set to true sets indentation
	UseIndentLines bool
	// raw is yaml serialised string
	raw string
	// key is yaml key parsed from a serialized string
	key string
	// val is yaml value parsed from a serialized string
	val string
	// foundChompingIndicator used to indicate presence of a multiline text i.e comment
	foundChompingIndicator bool
	// indentationSpaceBeforeComment used to track number of indentation spaces just before a comment line.
	indentationSpaceBeforeComment int
}

func NewYamlFormatter() *YamlFormatter {
	buff := &bytes.Buffer{}
	writer := ansiterm.NewWriter(buff)
	writer.SetColorCapable(true)
	return &YamlFormatter{
		Colors: Colors{
			Null:      ansiterm.Foreground(ansiterm.Magenta),
			Key:       ansiterm.Foreground(ansiterm.BrightCyan).SetStyle(ansiterm.Bold),
			Bool:      ansiterm.Foreground(ansiterm.Magenta),
			Number:    ansiterm.Foreground(ansiterm.Magenta),
			KeyValSep: ansiterm.Foreground(ansiterm.BrightMagenta),
			Ip:        ansiterm.Foreground(ansiterm.Cyan),
			String:    ansiterm.Foreground(ansiterm.Default),
			Multiline: ansiterm.Foreground(ansiterm.DarkGray),
			Comment:   ansiterm.Foreground(ansiterm.BrightMagenta),
		},
		UseIndentLines: true,
		writer:         writer,
		buff:           buff,
	}
}

func marshalYaml(obj interface{}) ([]byte, error) {
	buffer := bytes.Buffer{}
	if err := NewYamlFormatter().MarshalYaml(&buffer, obj); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

// MarshalYaml renders yaml string with ansi color escape sequences to output colorized yaml string.
func (f *YamlFormatter) MarshalYaml(buf *bytes.Buffer, obj interface{}) error {
	if obj == nil {
		f.Colors.Null.Fprint(f.writer, null)
		buf.WriteString(f.buff.String())
		f.buff.Reset()
		buf.WriteString("\n")
		return nil
	}

	// any other value: Run through Go YAML marshaller and colorize afterwards
	data, err := yamlv2.Marshal(obj)
	if err != nil {
		return err
	}

	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		if scanner.Text() == "EOF" {
			break
		}
		// Check for errors during read
		if err := scanner.Err(); err != nil {
			return err
		}

		f.raw = scanner.Text()

		f.formatYamlLine(buf)
	}

	return nil
}

func (f *YamlFormatter) formatYamlLine(buf *bytes.Buffer) {
	if f.foundChompingIndicator && (f.indentationSpaces() > f.indentationSpaceBeforeComment) {
		// found multiline comment or configmap, not treated as YAML at all
		f.colorMultiline(buf)
	} else if f.isKeyValue() {
		// valid YAML key: val line.
		if f.isComment() {
			f.colorComment(buf)
		} else if f.isValueNumber() {
			// value is a number
			f.colorKeyNumber(buf)
		} else if f.isValueIP() {
			// value is an ip address x.x.x.x
			f.colorKeyIP(buf)
		} else if f.isValueCIDR() {
			// value is cidr address x.x.x.x/x
			f.colorKeyCIDR(buf)
		} else if f.isValueBoolean() {
			f.colorKeyBool(buf)
		} else {
			// this is a normal key/val line
			f.colorKeyValue(buf)
		}

		// sign of a possible multiline text coming next
		if f.valueHasChompingIndicator() {
			//set flag for next execution in the iteration
			f.foundChompingIndicator = true
			// save current number of indentation spaces
			f.indentationSpaceBeforeComment = f.indentationSpaces()
		} else {
			// reset multiline flag
			f.foundChompingIndicator = false
		}
	} else if !f.isEmptyLine() {
		// is not a YAML key: value and is not empty
		if f.isComment() {
			f.colorComment(buf)
		} else if f.isElementOfList() {
			f.colorListElement(buf)
		} else {
			// invalid YAML line. probably because of indentation.
			buf.WriteString(fmt.Sprintf("%v\n", f.raw))
		}
		f.foundChompingIndicator = false
	} else if f.isEmptyLine() {
		// an empty line
		buf.WriteString(f.raw)
	}
}

// isKeyValue checks if the string contains a key value pair, if true it returns the key value pair.
func (f *YamlFormatter) isKeyValue() bool {
	if strings.Contains(f.raw, "://") {
		// it is a URL not key/val
		return false
	} else if strings.Contains(f.raw, ":") {
		// Contains the ":" delimiter but it might not be a key/val
		s := strings.Split(f.raw, ":")
		if strings.HasPrefix(s[1], " ") || len(s[1]) == 0 {
			// Checking if it's either:
			// - a proper key/val entry with a space after ":"
			// - or just key: \n
			f.key = s[0]
			f.val = strings.TrimSpace(strings.Join(s[1:], ":"))
			return true
		} else if len(s) > 2 && (strings.HasPrefix(s[len(s)-1], " ") || len(s[len(s)-1]) == 0) {
			// Multiple ":" separators found, checking last one
			/*
				key:{"type":"Example"}:
						  .: {}
						  f:description: {}
						  f:default: {}
			*/
			f.key = strings.Join(s[0:len(s)-1], ":")
			f.val = strings.TrimSpace(s[len(s)-1])
			return true
		}

	}
	return false
}

func (f *YamlFormatter) isComment() bool {
	if string(strings.TrimSpace(f.raw)[0]) == "#" { // convert the ascii code to string before matching
		// line is a comment
		return true
	}
	return false
}

func (f *YamlFormatter) isValueBoolean() bool {
	return strings.EqualFold(f.val, "true") || strings.EqualFold(f.val, "false")
}

func (f *YamlFormatter) isValueNumber() bool {
	_, err := strconv.Atoi(f.val)
	if err == nil {
		// is a number
		return true
	}
	return false
}

func (f *YamlFormatter) isValueIP() bool {
	_, err := strconv.Atoi(strings.ReplaceAll(f.val, ".", ""))
	if err == nil {
		// is an ip
		return true
	}
	return false
}

func (f *YamlFormatter) isValueCIDR() bool {
	ip, net, err := net.ParseCIDR(f.val)
	if err == nil {
		if net.Contains(ip) {
			return true
		}
	}
	return false
}

func (f *YamlFormatter) isEmptyLine() bool {
	if len(strings.TrimSpace(f.raw)) > 0 {
		return false
	}
	return true
}

func (f *YamlFormatter) isElementOfList() bool {
	if string(strings.TrimSpace(f.raw)[0]) == "-" {
		return true
	}
	return false
}

// indentationSpaces checks how many indentation spaces were used before
// the chomping indicator to catch a possible multiline comment or config
func (f *YamlFormatter) indentationSpaces() int {
	return len(f.raw) - len(strings.TrimLeft(f.raw, " "))
}

// valueHasChompingIndicator checks for multiline chomping indicator
// ">", ">-", ">+", "|", "|-", "|+"
func (f *YamlFormatter) valueHasChompingIndicator() bool {
	indicators := []string{">", ">-", ">+", "|", "|-", "|+"}
	for _, in := range indicators {
		if strings.Contains(f.val, in) {
			return true
		}
	}
	return false
}

// colors yaml key together with its separator i.e key:
func (f *YamlFormatter) colorKeyValSep(buf *bytes.Buffer) {
	f.Colors.KeyValSep.Fprint(f.writer, ":")
	keyValSep := f.buff.String()
	f.buff.Reset()

	f.Colors.Key.Fprintf(f.writer, "%v%s ", f.key, keyValSep)
	buf.WriteString(f.buff.String())
	f.buff.Reset()
}

// colorKeyValue colors a key and a string value pair
func (f *YamlFormatter) colorKeyValue(buf *bytes.Buffer) {
	f.colorKeyValSep(buf)

	f.Colors.String.Fprint(f.writer, f.val)
	buf.WriteString(f.buff.String())
	f.buff.Reset()

	buf.WriteString("\n")

}

func (f *YamlFormatter) colorComment(buf *bytes.Buffer) {
	f.Colors.Comment.Fprintf(f.writer, "%v %v\n", f.key, f.val)
	buf.WriteString(f.buff.String())
	f.buff.Reset()
}

func (f *YamlFormatter) colorMultiline(buf *bytes.Buffer) {
	f.Colors.Multiline.Fprintf(f.writer, "%v\n", f.raw)
	buf.WriteString(f.buff.String())
	f.buff.Reset()
}

// colorListElement colors elements belonging to a list.
func (f *YamlFormatter) colorListElement(buf *bytes.Buffer) {
	f.Colors.String.Fprintf(f.writer, "%v\n", f.raw)
	buf.WriteString(f.buff.String())
	f.buff.Reset()
}

// colors ip block address i.e x.x.x.x/x
func (f *YamlFormatter) colorKeyCIDR(buf *bytes.Buffer) {
	keyBuffer := bytes.Buffer{}
	f.colorKeyValSep(&keyBuffer)

	splitted := strings.Split(f.val, "/")
	ip := splitted[0]
	net := splitted[1]
	f.Colors.Ip.Fprint(f.writer, ip)
	ip = f.buff.String()
	f.buff.Reset()
	f.Colors.Number.Fprint(f.writer, net)
	net = f.buff.String()
	f.buff.Reset()
	f.Colors.Key.Fprintf(f.writer, "%v%v/%v\n", keyBuffer.String(), ip, net)
	str := f.buff.String()
	f.buff.Reset()
	buf.WriteString(str)
}

func (f *YamlFormatter) colorKeyIP(buf *bytes.Buffer) {
	f.colorKeyValSep(buf)

	f.Colors.Ip.Fprint(f.writer, f.val)
	buf.WriteString(f.buff.String())
	f.buff.Reset()
	buf.WriteString("\n")
}

func (f *YamlFormatter) colorKeyNumber(buf *bytes.Buffer) {
	f.colorKeyValSep(buf)

	f.Colors.Number.Fprintf(f.writer, "%v\n", f.val)
	buf.WriteString(f.buff.String())
	f.buff.Reset()
}

func (f *YamlFormatter) colorKeyBool(buf *bytes.Buffer) {
	f.colorKeyValSep(buf)

	f.Colors.Bool.Fprintf(f.writer, "%v\n", f.val)
	buf.WriteString(f.buff.String())
	f.buff.Reset()
}
