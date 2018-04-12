// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package debinterfaces

var _ Stanza = (*stanza)(nil)

// stanza implements Stanza.
type stanza struct {
	definition string
	location   Location
}

// Stanza represents a network definition as described by
// interfaces(8).
type Stanza interface {
	// A definition is the top-level stanza, together with any
	// options.
	Definition() []string

	// The file and linenumber, if any, where the stanza was
	// declared.
	Location() Location
}

// Location represents source file and line number information
type Location struct {
	Filename string
	LineNum  int
}

// AllowStanza are lines beginning with the word 'allow-*'.
type AllowStanza struct {
	stanza
	DeviceNames []string
}

// AutoStanza are lines beginning with the word "auto".
type AutoStanza struct {
	stanza
	DeviceNames []string
}

// IfaceStanza are lines beginning with 'iface'.
type IfaceStanza struct {
	stanza
	DeviceName          string
	HasBondMasterOption bool
	HasBondOptions      bool
	IsAlias             bool
	IsBridged           bool
	IsVLAN              bool
	Options             []string
}

// MappingStanza are lines beginning with the word "mapping".
type MappingStanza struct {
	stanza
	DeviceNames []string
	Options     []string
}

// NoAutoDownStanza are lines beginning with "no-auto-down".
type NoAutoDownStanza struct {
	stanza
	DeviceNames []string
}

// NoScriptsStanza are lines beginning with "no-scripts".
type NoScriptsStanza struct {
	stanza
	DeviceNames []string
}

// SourceStanza are lines beginning with "source".
type SourceStanza struct {
	stanza
	Path    string
	Sources []string
	Stanzas []Stanza
}

// SourceDirectoryStanza are lines beginning with "source-directory".
type SourceDirectoryStanza struct {
	stanza
	Path    string
	Sources []string
	Stanzas []Stanza
}

func definitionWithOptions(definition string, options []string) []string {
	result := make([]string, 1+len(options))
	result[0] = definition
	for i := range options {
		result[i+1] = options[i]
	}
	return result
}

// Location returns the filename and line number of the first line of
// the definition, if any.
func (s stanza) Location() Location {
	return s.location
}

// Definition returns all the lines that define the stanza. The
// individual lines are trimmed of leading and trailing whitespace.
func (s stanza) Definition() []string {
	return []string{s.definition}
}

// Definition returns all the lines that define the stanza. The
// individual lines are trimmed of leading and trailing whitespace.
func (s IfaceStanza) Definition() []string {
	return definitionWithOptions(s.definition, s.Options)
}

// Definition returns all the lines that define the stanza. The
// individual lines are trimmed of leading and trailing whitespace.
func (s MappingStanza) Definition() []string {
	return definitionWithOptions(s.definition, s.Options)
}
