// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package debinterfaces

import (
	"bytes"
	"strings"
)

// FlattenStanzas flattens all stanzas, and recursively for all
// 'source' and 'source-directory' stanzas, returning a single slice.
func FlattenStanzas(stanzas []Stanza) []Stanza {
	result := make([]Stanza, 0)

	for _, s := range stanzas {
		switch v := s.(type) {
		case SourceStanza:
			result = append(result, v.Stanzas...)
		case SourceDirectoryStanza:
			result = append(result, v.Stanzas...)
		default:
			result = append(result, v)
		}
	}

	return result
}

// FormatStanzas returns a string representing all stanzas
// definitions, recursively expanding stanzas found in both source and
// source-directory definitions.
func FormatStanzas(stanzas []Stanza, count int) string {
	var buffer bytes.Buffer

	for i, s := range stanzas {
		buffer.WriteString(FormatDefinition(s.Definition(), count))
		buffer.WriteString("\n")
		// If the current stanza is 'auto' and the next one is
		// 'iface' then don't add an additional blank line
		// between them.
		if _, ok := stanzas[i].(AutoStanza); ok {
			if i+1 < len(stanzas) {
				if _, ok := stanzas[i+1].(IfaceStanza); !ok {
					buffer.WriteString("\n")
				}
			}
		} else if i+1 < len(stanzas) {
			buffer.WriteString("\n")
		}
	}

	return strings.TrimSuffix(buffer.String(), "\n")
}

// FormatDefinition formats the complete stanza, indenting any options
// with count leading spaces.
func FormatDefinition(definition []string, count int) string {
	var buffer bytes.Buffer

	spacer := strings.Repeat(" ", count)

	for i, d := range definition {
		if i == 0 {
			buffer.WriteString(d)
			buffer.WriteString("\n")
		} else {
			buffer.WriteString(spacer)
			buffer.WriteString(d)
			buffer.WriteString("\n")
		}
	}

	return strings.TrimSuffix(buffer.String(), "\n")
}
