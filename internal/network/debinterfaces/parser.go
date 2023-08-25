// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package debinterfaces

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	unknown kind = iota
	allow
	auto
	iface
	mapping
	noAutoDown
	noscripts
	source
	sourceDirectory
)

var validSourceDirectoryFilename = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

type parser struct {
	scanner  *lineScanner
	expander WordExpander
}

// Type is the set of lexical tokens that represent top-level stanza
// identifiers based on the description in the interfaces(5) man page.
type kind int

// ParseError represents an error when parsing a line of a
// Debian-style interfaces definition. This only covers top-level
// definitions.
type ParseError struct {
	Filename string
	Line     string
	LineNum  int
	Message  string
}

// Error returns the parsing error.
func (p *ParseError) Error() string {
	return p.Message
}

func newParseError(s *lineScanner, msg string) *ParseError {
	return &ParseError{
		Filename: s.filename,
		Line:     s.line,
		LineNum:  s.n,
		Message:  msg,
	}
}

func (p parser) newStanzaBase() *stanza {
	return &stanza{
		location: Location{
			Filename: p.scanner.filename,
			LineNum:  p.scanner.n,
		},
		definition: p.scanner.line,
	}
}

func (p parser) parseOptions() []string {
	options := []string{}
	for {
		if !p.scanner.nextLine() {
			return options
		}
		line := p.scanner.line
		if stanzaType(line) != unknown {
			// go back a line
			p.scanner.n--
			p.scanner.line = strings.TrimSpace(p.scanner.lines[p.scanner.n])
			return options
		}
		options = append(options, line)
	}
}

func (p parser) parseAllowStanza() (*AllowStanza, error) {
	words := strings.Fields(p.scanner.line)
	if len(words) < 2 {
		return nil, newParseError(p.scanner, "missing device name")
	}
	return &AllowStanza{
		stanza:      *p.newStanzaBase(),
		DeviceNames: words[1:],
	}, nil
}

func (p parser) parseAutoStanza() (*AutoStanza, error) {
	words := strings.Fields(p.scanner.line)
	if len(words) < 2 {
		return nil, newParseError(p.scanner, "missing device name")
	}
	return &AutoStanza{
		stanza:      *p.newStanzaBase(),
		DeviceNames: words[1:],
	}, nil
}

func (p parser) parseIfaceStanza() (*IfaceStanza, error) {
	s := p.newStanzaBase()
	words := strings.Fields(p.scanner.line)

	if len(words) < 2 {
		return nil, newParseError(p.scanner, "missing device name")
	}

	options := p.parseOptions()

	return &IfaceStanza{
		stanza:              *s,
		DeviceName:          words[1],
		HasBondMasterOption: hasBondMasterOption(options),
		HasBondOptions:      hasBondOptions(options),
		IsAlias:             isAlias(words[1]),
		IsBridged:           hasBridgePortsOption(options),
		IsVLAN:              isVLAN(options),
		Options:             options,
	}, nil
}

func (p parser) parseMappingStanza() (*MappingStanza, error) {
	s := p.newStanzaBase()
	words := strings.Fields(p.scanner.line)
	if len(words) < 2 {
		return nil, newParseError(p.scanner, "missing device name")
	}

	options := p.parseOptions()

	return &MappingStanza{
		stanza:      *s,
		DeviceNames: words[1:],
		Options:     options,
	}, nil
}

func (p parser) parseNoAutoDownStanza() (*NoAutoDownStanza, error) {
	words := strings.Fields(p.scanner.line)
	if len(words) < 2 {
		return nil, newParseError(p.scanner, "missing device name")
	}
	return &NoAutoDownStanza{
		stanza:      *p.newStanzaBase(),
		DeviceNames: words[1:],
	}, nil
}

func (p parser) parseNoScriptsStanza() (*NoScriptsStanza, error) {
	words := strings.Fields(p.scanner.line)
	if len(words) < 2 {
		return nil, newParseError(p.scanner, "missing device name")
	}
	return &NoScriptsStanza{
		stanza:      *p.newStanzaBase(),
		DeviceNames: words[1:],
	}, nil
}

func (p parser) parseSourceStanza() (*SourceStanza, error) {
	words := strings.Fields(p.scanner.line)
	if len(words) < 2 {
		return nil, newParseError(p.scanner, "missing filename")
	}

	pattern := words[1]

	if !strings.HasPrefix(words[1], "/") {
		pattern = filepath.Join(filepath.Dir(p.scanner.filename), words[1])
	}

	files, err := p.expander.Expand(pattern)

	if err != nil {
		return nil, newParseError(p.scanner, err.Error())
	}

	srcStanza := &SourceStanza{
		stanza:  *p.newStanzaBase(),
		Path:    words[1],
		Sources: []string{},
		Stanzas: []Stanza{},
	}

	for _, file := range files {
		stanzas, err := parseSource(file, nil, p.expander)
		if err != nil {
			return nil, err
		}
		srcStanza.Sources = append(srcStanza.Sources, file)
		srcStanza.Stanzas = append(srcStanza.Stanzas, stanzas...)
	}

	return srcStanza, nil
}

func (p parser) parseSourceDirectoryStanza() (*SourceDirectoryStanza, error) {
	words := strings.Fields(p.scanner.line)
	if len(words) < 2 {
		return nil, newParseError(p.scanner, "missing directory")
	}

	expansions, err := p.expander.Expand(words[1])

	if err != nil {
		// We want file/line number information so use the
		// Expand() error as the message but let
		// newParseError() record on which line it happened.
		return nil, newParseError(p.scanner, err.Error())
	}

	var dir = words[1]

	if len(expansions) > 0 {
		dir = expansions[0]
	}

	if !strings.HasPrefix(dir, "/") {
		// find directory relative to current input file
		dir = filepath.Join(filepath.Dir(p.scanner.filename), dir)
	}

	files, err := os.ReadDir(dir)

	if err != nil {
		return nil, newParseError(p.scanner, err.Error())
	}

	dirStanza := &SourceDirectoryStanza{
		stanza:  *p.newStanzaBase(),
		Path:    words[1],
		Sources: []string{},
		Stanzas: []Stanza{},
	}

	for _, file := range files {
		if !validSourceDirectoryFilename.MatchString(file.Name()) {
			continue
		}
		path := filepath.Join(dir, file.Name())
		stanzas, err := parseSource(path, nil, p.expander)
		if err != nil {
			return nil, err
		}
		dirStanza.Sources = append(dirStanza.Sources, path)
		dirStanza.Stanzas = append(dirStanza.Stanzas, stanzas...)
	}

	return dirStanza, nil
}

func (p parser) parseInput() ([]Stanza, error) {
	stanzas := []Stanza{}

	for {
		if !p.scanner.nextLine() {
			break
		}

		switch stanzaType(p.scanner.line) {
		case allow:
			allowStanza, err := p.parseAllowStanza()
			if err != nil {
				return nil, err
			}
			stanzas = append(stanzas, *allowStanza)
		case auto:
			autoStanza, err := p.parseAutoStanza()
			if err != nil {
				return nil, err
			}
			stanzas = append(stanzas, *autoStanza)
		case iface:
			ifaceStanza, err := p.parseIfaceStanza()
			if err != nil {
				return nil, err
			}
			stanzas = append(stanzas, *ifaceStanza)
		case mapping:
			mappingStanza, err := p.parseMappingStanza()
			if err != nil {
				return nil, err
			}
			stanzas = append(stanzas, *mappingStanza)
		case noAutoDown:
			noAutoDownStanza, err := p.parseNoAutoDownStanza()
			if err != nil {
				return nil, err
			}
			stanzas = append(stanzas, *noAutoDownStanza)
		case noscripts:
			noScriptsStanza, err := p.parseNoScriptsStanza()
			if err != nil {
				return nil, err
			}
			stanzas = append(stanzas, *noScriptsStanza)
		case source:
			sourceStanza, err := p.parseSourceStanza()
			if err != nil {
				return nil, err
			}
			stanzas = append(stanzas, *sourceStanza)
		case sourceDirectory:
			sourceDirectoryStanza, err := p.parseSourceDirectoryStanza()
			if err != nil {
				return nil, err
			}
			stanzas = append(stanzas, *sourceDirectoryStanza)
		default:
			return nil, newParseError(p.scanner, "misplaced option")
		}
	}

	return stanzas, nil
}

func stanzaType(definition string) kind {
	words := strings.Fields(definition)
	if len(words) > 0 {
		switch words[0] {
		case "auto":
			return auto
		case "iface":
			return iface
		case "mapping":
			return mapping
		case "no-auto-down":
			return noAutoDown
		case "no-scripts":
			return noscripts
		case "source":
			return source
		case "source-directory":
			return sourceDirectory
		}
		if strings.HasPrefix(words[0], "allow-") {
			return allow
		}
	}
	return unknown
}

// If input is not nil, parseSource parses the source from input; the
// filename is only used when recording position information. The type
// of the argument for the input parameter must be string, []byte, or
// io.Reader. If input == nil, Parse parses the file specified by
// filename.
//
// If the source could not be read, then Stanzas is nil and the error
// indicates the specific failure.
func parseSource(filename string, src interface{}, wordExpander WordExpander) ([]Stanza, error) {
	scanner, err := newScanner(filename, src)

	if err != nil {
		return nil, err
	}

	p := parser{
		expander: wordExpander,
		scanner:  scanner,
	}

	return p.parseInput()
}

func hasOptionIdent(ident string, options []string) bool {
	for _, o := range options {
		words := strings.Fields(o)
		if len(words) > 0 && words[0] == ident {
			return true
		}
	}
	return false
}

func hasOptionPrefix(prefix string, options []string) bool {
	for _, o := range options {
		words := strings.Fields(o)
		for _, w := range words {
			if strings.HasPrefix(w, prefix) {
				return true
			}
		}
	}
	return false
}

func hasBridgePortsOption(options []string) bool {
	return hasOptionIdent("bridge_ports", options)
}

func isVLAN(options []string) bool {
	return hasOptionIdent("vlan-raw-device", options)
}

func hasBondOptions(options []string) bool {
	return hasOptionPrefix("bond-", options)
}

func hasBondMasterOption(options []string) bool {
	return hasOptionIdent("bond-master", options)
}

func isAlias(name string) bool {
	return strings.Contains(name, ":")
}

// Parse parses the definitions of a single Debian style network
// interfaces(5) file and returns the corresponding set of stanza
// definitions.
func Parse(filename string) ([]Stanza, error) {
	return parseSource(filename, nil, newWordExpander())
}
