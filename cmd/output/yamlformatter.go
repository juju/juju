// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package output

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/juju/ansiterm"
	yamlv2 "gopkg.in/yaml.v2"
	yamlv3 "gopkg.in/yaml.v3"
	"net"
	"reflect"
	"strconv"
	"strings"
)

const (
	nodeAnchor     = "&"
	leadingDash    = "- "
	documentStart  = "---"
	emptyStructure = "[]"
	emptyPrefix    = " "
	dashPrefix     = "-"
	colon          = ":"
	indent         = "│"
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
	// inString detects strings that are within strings
	inString bool
}

func NewYamlFormatter() *YamlFormatter {
	buff := &bytes.Buffer{}
	writer := ansiterm.NewWriter(buff)
	writer.SetColorCapable(true)
	return &YamlFormatter{
		Colors: Colors{
			Null:           ansiterm.Foreground(ansiterm.Magenta),
			Key:            ansiterm.Foreground(ansiterm.BrightCyan).SetStyle(ansiterm.Bold),
			Bool:           ansiterm.Foreground(ansiterm.Magenta),
			Number:         ansiterm.Foreground(ansiterm.Magenta),
			KeyValSep:      ansiterm.Foreground(ansiterm.BrightMagenta),
			Ip:             ansiterm.Foreground(ansiterm.Yellow),
			String:         ansiterm.Foreground(ansiterm.Default),
			IndentLine:     ansiterm.Foreground(ansiterm.Default),
			DocumentStart:  ansiterm.Foreground(ansiterm.Default),
			EmptyStructure: ansiterm.Foreground(ansiterm.DarkGray),
			Dash:           ansiterm.Foreground(ansiterm.BrightYellow),
			NodeAnchor:     ansiterm.Foreground(ansiterm.BrightGreen),
		},
		UseIndentLines: true,
		writer:         writer,
		buff:           buff,
	}
}

func marshalYaml(obj interface{}) ([]byte, error) {
	buffer := bytes.Buffer{}
	if err := NewYamlFormatter().MarshalYaml("", false, &buffer, obj); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func (f *YamlFormatter) MarshalYaml(prefix string, skipIndentOnFirstLine bool, buf *bytes.Buffer, val interface{}) error {
	switch v := val.(type) {
	case yamlv2.MapSlice:
		return f.marshalMapSlice(prefix, skipIndentOnFirstLine, buf, v)
	case []interface{}:
		return f.marshalSlice(prefix, skipIndentOnFirstLine, v, buf)
	case []yamlv2.MapSlice:
		return f.marshalSlice(prefix, skipIndentOnFirstLine, f.simplify(v), buf)
	case yamlv3.Node:
		return f.marshalNode(prefix, skipIndentOnFirstLine, buf, &v)
	default:
		switch reflect.TypeOf(val).Kind() {
		case reflect.Ptr:
			return f.MarshalYaml(prefix, skipIndentOnFirstLine, buf, reflect.ValueOf(val).Elem().Interface())
		case reflect.Struct:
			return f.marshalStruct(prefix, skipIndentOnFirstLine, buf, v)
		default:
			return f.marshalScalar(prefix, v, buf)
		}
	}
}

func (f *YamlFormatter) marshalScalar(prefix string, obj interface{}, buf *bytes.Buffer) error {
	// process nil values immediately and return afterwards
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
		line := scanner.Text()
		f.formatYamlLine(line, buf)
	}

	return nil
}

func (f *YamlFormatter) marshalValue(prefix string, val interface{}, skipIndentOnFirstLine bool, buf *bytes.Buffer) error {
	switch v := val.(type) {
	case map[string]interface{}:
		return f.marshalSlice(prefix, skipIndentOnFirstLine, val.([]interface{}), buf)
	case []interface{}:
		return f.marshalSlice(prefix, skipIndentOnFirstLine, v, buf)
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

	return nil
}

func (f *YamlFormatter) marshalString(str string, buf *bytes.Buffer) error {
	strBytes, err := yamlv2.Marshal(str)
	if err != nil {
		return err
	}
	str = string(strBytes)

	f.Colors.String.Fprint(f.writer, str)
	buf.WriteString(f.buff.String())
	f.buff.Reset()

	return nil
}

func (f *YamlFormatter) addPrefix() string {
	var str string
	if f.UseIndentLines {
		f.Colors.IndentLine.Fprintf(f.writer, "%s ", indent)
		str = f.buff.String()
		f.buff.Reset()
		return str
	}
	f.Colors.IndentLine.Fprintf(f.writer, emptyPrefix)
	str = f.buff.String()
	f.buff.Reset()
	return str
}

func (f *YamlFormatter) isScalar(obj interface{}) bool {
	switch v := obj.(type) {
	case *yamlv3.Node:
		return v.Kind == yamlv3.ScalarNode
	case yamlv2.MapSlice, []interface{}, []yamlv2.MapSlice:
		return false
	default:
		return true
	}
}

//
func (f *YamlFormatter) marshalSlice(prefix string, skipIndentOnFirstLine bool, vals []interface{}, buf *bytes.Buffer) error {
	for _, val := range vals {
		buf.WriteString(prefix)
		f.Colors.Dash.Fprint(f.writer, "-")
		buf.WriteString(f.buff.String())
		f.buff.Reset()
		buf.WriteString(emptyPrefix)

		prefix += f.addPrefix()
		if err := f.MarshalYaml(prefix, true, buf, val); err != nil {
			return err
		}
	}

	return nil
}

func (f *YamlFormatter) marshalMapSlice(prefix string, skipIndentOnFirstLine bool, buf *bytes.Buffer, mapSlice yamlv2.MapSlice) error {
	f.Colors.KeyValSep.Fprint(f.writer, ":")
	keyValSep := f.buff.String()
	f.buff.Reset()

	for i, mapItem := range mapSlice {
		if i > 0 || !skipIndentOnFirstLine {
			buf.WriteString(prefix)
		}

		f.Colors.Key.Fprintf(f.writer, "%s%s", mapItem.Key, keyValSep)
		buf.WriteString(f.buff.String())
		f.buff.Reset()

		switch mapItem.Value.(type) {
		case yamlv2.MapSlice:
			if len(mapItem.Value.(yamlv2.MapSlice)) == 0 {
				buf.WriteString(emptyPrefix)
				buf.WriteString(emptyMap)
				buf.WriteString("\n")
			} else {
				buf.WriteString("\n")
				prefix += f.addPrefix()
				if err := f.marshalMapSlice(prefix, false, buf, mapItem.Value.(yamlv2.MapSlice)); err != nil {
					return err
				}
			}
		case []interface{}:
			if len(mapItem.Value.([]interface{})) == 0 {
				buf.WriteString(emptyPrefix)
				f.Colors.EmptyStructure.Fprint(f.writer, emptyStructure)
				buf.WriteString(f.buff.String())
				f.buff.Reset()
			} else {
				buf.WriteString("\n")
				if err := f.marshalSlice(prefix, false, mapItem.Value.([]interface{}), buf); err != nil {
					return err
				}
			}
		default:
			buf.WriteString(emptyPrefix)
			if err := f.marshalScalar(prefix, mapItem.Value, buf); err != nil {
				return err
			}
		}
	}

	return nil
}

func (f *YamlFormatter) marshalNode(prefix string, skipIndentOnFirstLine bool, buf *bytes.Buffer, node *yamlv3.Node) error {
	switch node.Kind {
	case yamlv3.DocumentNode:
		f.Colors.DocumentStart.Fprint(f.writer, documentStart)
		buf.WriteString(f.buff.String())
		f.buff.Reset()
		buf.WriteString("\n")
		for _, content := range node.Content {
			if err := f.MarshalYaml(prefix, false, buf, content); err != nil {
				return err
			}
		}
		if len(node.FootComment) > 0 {
			f.Colors.Comment.Fprint(f.writer, node.FootComment)
			buf.WriteString(f.buff.String())
			f.buff.Reset()
		}
	case yamlv3.SequenceNode:
		for i, content := range node.Content {
			if i == 0 {
				if !skipIndentOnFirstLine {
					buf.WriteString(prefix)
				}
			} else {
				buf.WriteString(prefix)
			}
			f.Colors.Dash.Fprint(f.writer, dashPrefix)
			buf.WriteString(f.buff.String())
			f.buff.Reset()
			buf.WriteString(emptyPrefix)
			prefix += f.addPrefix()
			if err := f.marshalNode(prefix, true, buf, content); err != nil {
				return err
			}
		}

	case yamlv3.MappingNode:
		for i := 0; i < len(node.Content); i += 2 {
			if !skipIndentOnFirstLine || i > 0 {
				buf.WriteString(prefix)
			}

			key := node.Content[i]
			if len(key.HeadComment) > 0 {
				f.Colors.Comment.Fprint(f.writer, key.HeadComment)
				buf.WriteString(f.buff.String())
				f.buff.Reset()
				buf.WriteString("\n")
			}
			f.Colors.Key.Fprint(f.writer, key.Value)
			buf.WriteString(f.buff.String())
			f.buff.Reset()

			value := node.Content[i+1]
			switch value.Kind {
			case yamlv3.MappingNode:
				if len(value.Content) == 0 {
					buf.WriteString(emptyPrefix)
					f.Colors.EmptyStructure.Fprint(f.writer, emptyStructure)
					buf.WriteString(f.buff.String())
					f.buff.Reset()
					buf.WriteString("\n")
				} else {
					buf.WriteString(f.createAnchorDefinition(value))
					buf.WriteString("\n")
					if err := f.marshalNode(prefix, false, buf, value); err != nil {
						return err
					}
				}
			case yamlv3.SequenceNode:
				if len(value.Content) == 0 {
					buf.WriteString(emptyPrefix)
					f.Colors.EmptyStructure.Fprint(f.writer, emptyMap)
					buf.WriteString(f.buff.String())
					f.buff.Reset()
					buf.WriteString("\n")
				} else {
					buf.WriteString(f.createAnchorDefinition(value))
					buf.WriteString("\n")
					if err := f.marshalNode(prefix, false, buf, value); err != nil {
						return err
					}
				}
			case yamlv3.ScalarNode:
				buf.WriteString(f.createAnchorDefinition(value))
				buf.WriteString(emptyPrefix)
				prefix += f.addPrefix()
				if err := f.marshalNode(prefix, false, buf, value); err != nil {
					return err
				}
			case yamlv3.AliasNode:
				f.Colors.NodeAnchor.Fprintf(f.writer, "*%v", value.Value)
				buf.WriteString(f.buff.String())
				f.buff.Reset()
			}
			if len(key.FootComment) > 0 {
				f.Colors.Comment.Fprint(f.writer, key.FootComment)
				buf.WriteString(f.buff.String())
				f.buff.Reset()
				buf.WriteString("\n")
			}
		}
	case yamlv3.ScalarNode:
		parse := func() string {
			var str string
			lines := strings.Split(node.Value, "\n")
			if len(lines) == 1 {
				if strings.ContainsAny(node.Value, " *&:,") {
					str += fmt.Sprintf("\"%s\"", node.Value)
				} else {
					str += node.Value
				}
			} else {
				str += fmt.Sprintf("%s\n", indent)
				for i, line := range lines {
					str += fmt.Sprintf("%s%v", prefix, line)
					if i != len(lines)-1 {
						str += "\n"
					}
				}
			}
			return str
		}

		switch node.Tag {
		case "!!binary":
			f.Colors.Binary.Fprint(f.writer, parse())
			buf.WriteString(f.buff.String())
			f.buff.Reset()
		case "!!str":
			f.Colors.String.Fprint(f.writer, parse())
			buf.WriteString(f.buff.String())
			f.buff.Reset()
		case "!!float":
			f.Colors.Number.Fprint(f.writer, parse())
			buf.WriteString(f.buff.String())
			f.buff.Reset()
		case "!!int":
			f.Colors.Number.Fprint(f.writer, parse())
			buf.WriteString(f.buff.String())
			f.buff.Reset()
		case "!!bool":
			f.Colors.Bool.Fprint(f.writer, parse())
			buf.WriteString(f.buff.String())
			f.buff.Reset()
		}

		if len(node.LineComment) > 0 {
			f.Colors.Comment.Fprint(f.writer, node.LineComment)
			buf.WriteString(f.buff.String())
			f.buff.Reset()
		}
		buf.WriteString("\n")
		if len(node.FootComment) > 0 {
			f.Colors.Comment.Fprint(f.writer, node.FootComment)
			buf.WriteString(f.buff.String())
			f.buff.Reset()
			buf.WriteString("\n")
		}
	case yamlv3.AliasNode:
		if err := f.marshalNode(prefix, skipIndentOnFirstLine, buf, node.Alias); err != nil {
			return err
		}
	}
	return nil
}

func (f *YamlFormatter) createAnchorDefinition(node *yamlv3.Node) string {
	var str string
	if len(node.Anchor) != 0 {
		f.Colors.NodeAnchor.Fprintf(f.writer, "%s%v", nodeAnchor, node.Anchor)
		str = f.buff.String()
		f.buff.Reset()
	}
	return str
}

func (f *YamlFormatter) simplify(list []yamlv2.MapSlice) []interface{} {
	result := make([]interface{}, len(list))
	for idx, value := range list {
		result[idx] = value
	}

	return result
}

func (f *YamlFormatter) marshalStruct(prefix string, skipIndentOnFirstLine bool, buf *bytes.Buffer, obj interface{}) error {
	// There might be better ways to do it. With generic struct objects, the
	// only option is to do a round trip marshal and unmarshal to get the
	// object into a universal Go YAML library version 3 node object and
	// to render the node instead.
	data, err := yamlv3.Marshal(obj)
	if err != nil {
		return err
	}

	var tmp yamlv3.Node
	if err := yamlv3.Unmarshal(data, &tmp); err != nil {
		return err
	}
	return f.MarshalYaml(prefix, skipIndentOnFirstLine, buf, tmp)
}

func followAlias(node *yamlv3.Node) *yamlv3.Node {
	if node != nil && node.Alias != nil {
		return followAlias(node.Alias)
	}

	return node
}

func (f *YamlFormatter) formatYamlLine(line string, buf *bytes.Buffer) {
	indentCount := findIndent(line)
	indent := toSpaces(indentCount)
	trimmedLine := strings.TrimLeft(line, emptyPrefix)

	if f.inString {
		// if inString is true, the line must be a part of a string which is broken into several lines
		fmt.Fprintf(f.writer, "%s%s\n", indent, f.colorYamlString(trimmedLine))
		f.inString = !f.isStringClosed(trimmedLine)
		buf.WriteString(f.buff.String())
		f.buff.Reset()
		return
	}

	splitted := strings.SplitN(trimmedLine, ": ", 2) //assuming key does not contain ": " while value might

	if len(splitted) == 2 {
		// key: value
		key, val := splitted[0], splitted[1]
		fmt.Fprintf(f.writer, "%s%s: %s\n", indent, f.colorYamlKey(key, indentCount, 2), f.colorYamlValue(val))
		f.inString = f.isStringOpenedButNotClosed(val)
		buf.WriteString(f.buff.String())
		f.buff.Reset()
		return
	}

	// the line is just a "key:" or an element of an array
	if strings.HasSuffix(splitted[0], ":") {
		// key:
		fmt.Fprintf(f.writer, "%s%s\n", indent, f.colorYamlKey(splitted[0], indentCount, 2))
		buf.WriteString(f.buff.String())
		f.buff.Reset()
		return
	}

	fmt.Fprintf(f.writer, "%s%s\n", indent, f.colorYamlValue(splitted[0]))
	buf.WriteString(f.buff.String())
	f.buff.Reset()
}

func (f *YamlFormatter) colorYamlKey(key string, indentCount int, width int) string {
	var str string
	hasColon := strings.HasSuffix(key, colon)
	hasLeadingDash := strings.HasPrefix(key, leadingDash)
	key = strings.TrimSuffix(key, colon)
	key = strings.TrimPrefix(key, leadingDash)

	format := "%s"
	if hasColon {
		format += colon
	}

	if hasLeadingDash {
		format = leadingDash + format
		indentCount += 2
	}

	f.Colors.Key.Fprintf(f.writer, format, key)
	str = f.buff.String()
	f.buff.Reset()

	return str
}

func (f *YamlFormatter) colorYamlValue(val string) string {
	if val == emptyMap {
		return emptyMap
	}

	hasLeadingDash := strings.HasPrefix(val, leadingDash)
	val = strings.TrimPrefix(val, leadingDash)

	isDoubleQuoted := strings.HasPrefix(val, leadingDash) && strings.HasSuffix(val, leadingDash)
	trimmedVal := strings.TrimSuffix(strings.TrimPrefix(val, `"`), `"`)

	var format string
	switch {
	case hasLeadingDash && isDoubleQuoted:
		format = `- "%s"`
	case hasLeadingDash:
		format = "- %s"
	case isDoubleQuoted:
		format = `"%s"`
	default:
		format = "%s"
	}

	v := f.colorByType(trimmedVal)
	return fmt.Sprintf(format, v)
}

func (f *YamlFormatter) colorYamlString(val string) string {
	var str string

	isDoubleQuoted := strings.HasPrefix(val, `"`) && strings.HasPrefix(val, `"`)
	trimmedVal := strings.TrimRight(strings.TrimLeft(val, `"`), `"`)

	var format string
	switch {
	case isDoubleQuoted:
		format = `"%s"`
	default:
		format = "%s"
	}

	f.Colors.String.Fprintf(f.writer, format, trimmedVal)
	str = f.buff.String()
	f.buff.Reset()

	return str
}

func (f *YamlFormatter) colorByType(val interface{}) string {
	var str string
	switch v := val.(type) {
	case float64:
		f.Colors.Number.Fprint(f.writer, strconv.FormatFloat(v, 'f', -1, 64))
		str = f.buff.String()
		f.buff.Reset()
	case int:
		f.Colors.Number.Fprint(f.writer, v)
		str = f.buff.String()
		f.buff.Reset()
	case bool:
		f.Colors.Bool.Fprint(f.writer, strconv.FormatBool(v))
		str = f.buff.String()
		f.buff.Reset()
	case string:
		if isNetAddr(v) {
			splitted := strings.Split(v, "/")
			ip := splitted[0]
			net := splitted[1]
			f.Colors.Ip.Fprint(f.writer, ip)
			ip = f.buff.String()
			f.buff.Reset()
			f.Colors.Number.Fprint(f.writer, net)
			net = f.buff.String()
			f.buff.Reset()
			str = fmt.Sprintf("%s/%s", ip, net)
		} else {
			if num, err := strconv.Atoi(v); err == nil {
				f.Colors.Number.Fprint(f.writer, num)
				str = f.buff.String()
				f.buff.Reset()
			} else {
				str = f.colorYamlString(v)
			}
		}
	case nil:
		f.Colors.Null.Fprint(f.writer, null)
		str = f.buff.String()
		f.buff.Reset()
	}

	return str
}

func (f *YamlFormatter) isStringClosed(line string) bool {
	return strings.HasSuffix(line, "'") || strings.HasSuffix(line, `"`)
}

func (f *YamlFormatter) isStringOpenedButNotClosed(line string) bool {
	return (strings.HasPrefix(line, "'") && !strings.HasSuffix(line, "'")) ||
		(strings.HasPrefix(line, `"`) && !strings.HasSuffix(line, `"`))
}

// findIndent returns a length of indent (spaces at left) in the given line
func findIndent(line string) int {
	return len(line) - len(strings.TrimLeft(line, " "))
}

// toSpaces returns repeated spaces whose length is n.
func toSpaces(n int) string {
	return strings.Repeat(" ", n)
}

func isNetAddr(val string) bool {
	ip, net, err := net.ParseCIDR(val)
	if err == nil {
		if net.Contains(ip) {
			return true
		}
	}
	return false
}
