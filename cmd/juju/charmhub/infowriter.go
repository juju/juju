// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/juju/charm/v13"
	"github.com/juju/errors"
	"gopkg.in/yaml.v2"

	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/output"
)

// Note:
// Using yaml formatting for most of the juju info output,
// to keep it similar to snap info, is easiest done in yaml.
// There are exceptions, slices of strings and tables.  These
// are transformed into strings.

func makeInfoWriter(w io.Writer, warningLog Log, config bool, unicodeMode string, baseMode baseMode, in *InfoResponse) Printer {
	iw := infoWriter{
		w:             w,
		warningf:      warningLog,
		in:            in,
		displayConfig: config,
		unicodeMode:   unicodeMode,
		baseMode:      baseMode,
	}
	if iw.in.Type == "charm" {
		return charmInfoWriter{infoWriter: iw}
	}
	return bundleInfoWriter{infoWriter: iw}
}

type infoWriter struct {
	warningf      Log
	w             io.Writer
	in            *InfoResponse
	displayConfig bool
	unicodeMode   string
	baseMode      baseMode
}

type baseMode int

const (
	baseModeNone baseMode = iota
	baseModeArches
	baseModeBases
	baseModeBoth
)

func (iw infoWriter) print(info interface{}) error {
	encoder := yaml.NewEncoder(iw.w)
	defer func() { _ = encoder.Close() }()
	return encoder.Encode(info)
}

func (iw infoWriter) channels() string {
	if len(iw.in.Channels) == 0 {
		return ""
	}

	var buffer bytes.Buffer
	tw := output.TabWriter(&buffer)
	ow := output.Wrapper{TabWriter: tw}
	w := InfoUnicodeWriter(ow, iw.unicodeMode)

	// Iterate Tracks slice instead of Channels map to maintain order
	for _, track := range iw.in.Tracks {
		risks := iw.in.Channels[track]
		shown := false

		// Iterate charm.Risks instead of risks map to standardize order
		for _, risk := range charm.Risks {
			revisions, ok := risks[string(risk)]
			if !ok {
				w.Printf("%s/%s:", track, risk)
				c := UnicodeDash // dash means no revision available
				if shown {
					c = UnicodeUpArrow // points up to revision on previous line
				}
				_, _ = w.PrintlnUnicode(c)
				continue
			}
			shown = true

			switch iw.baseMode {
			case baseModeNone:
				latest := revisions[0] // latest is always first
				w.Println(formatRevision(latest, true))
			case baseModeArches:
				for i, r := range revisions {
					args := []any{formatRevision(r, i == 0)}
					arches := strings.Join(r.Arches, ", ")
					if arches != "" {
						args = append(args, arches)
					}
					w.Println(args...)
				}
			case baseModeBases:
				latest := revisions[0]
				args := []any{formatRevision(latest, true)}
				bases := strings.Join(basesDisplay(latest.Bases), ", ")
				if bases != "" {
					args = append(args, bases)
				}
				w.Println(args...)
			case baseModeBoth:
				latest := revisions[0]
				args := []any{formatRevision(latest, true)}
				arches := strings.Join(latest.Arches, ", ")
				if arches != "" {
					args = append(args, arches)
				}
				bases := strings.Join(basesDisplay(latest.Bases), ", ")
				if bases != "" {
					args = append(args, bases)
				}
				w.Println(args...)
			}
		}
	}
	if err := ow.Flush(); err != nil {
		iw.warningf("%v", errors.Annotate(err, "could not flush channel data to buffer"))
	}
	return buffer.String()
}

// formatRevision formats revision for human-readable tabbed output.
func formatRevision(r Revision, showName bool) string {
	var namePrefix string
	if showName {
		namePrefix = fmt.Sprintf("%s/%s:", r.Track, r.Risk)
	}
	return fmt.Sprintf("%s\t%s\t%s\t(%d)\t%s",
		namePrefix, r.Version, r.ReleasedAt[:10], r.Revision, sizeToStr(r.Size))
}

// basesDisplay returns a slice of bases in the format "name@channel".
func basesDisplay(bases []Base) []string {
	strs := make([]string, len(bases))
	for i, b := range bases {
		base, err := corebase.ParseBase(b.Name, b.Channel)
		if err != nil {
			strs[i] = base.DisplayString()
			continue
		}

		strs[i] = b.Name + "@" + b.Channel
	}
	return strs
}

type bundleInfoOutput struct {
	Name        string `yaml:"name,omitempty"`
	Publisher   string `yaml:"publisher,omitempty"`
	Summary     string `yaml:"summary,omitempty"`
	Description string `yaml:"description,omitempty"`
	StoreURL    string `yaml:"store-url,omitempty"`
	ID          string `yaml:"bundle-id,omitempty"`
	Tags        string `yaml:"tags,omitempty"`
	Charms      string `yaml:"charms,omitempty"`
	Channels    string `yaml:"channels,omitempty"`
	Installed   string `yaml:"installed,omitempty"`
}

type bundleInfoWriter struct {
	infoWriter
}

func (b bundleInfoWriter) Print() error {
	out := &bundleInfoOutput{
		Name:        b.in.Name,
		ID:          b.in.ID,
		Summary:     b.in.Summary,
		Publisher:   b.in.Publisher,
		Description: b.in.Description,
		Tags:        strings.Join(b.in.Tags, ", "),
		Channels:    b.channels(),
	}
	return b.print(out)
}

type charmInfoOutput struct {
	Name        string                 `yaml:"name,omitempty"`
	Publisher   string                 `yaml:"publisher,omitempty"`
	Summary     string                 `yaml:"summary,omitempty"`
	Description string                 `yaml:"description,omitempty"`
	StoreURL    string                 `yaml:"store-url,omitempty"`
	ID          string                 `yaml:"charm-id,omitempty"`
	Supports    string                 `yaml:"supports,omitempty"`
	Tags        string                 `yaml:"tags,omitempty"`
	Subordinate bool                   `yaml:"subordinate"`
	Relations   relationOutput         `yaml:"relations,omitempty"`
	Channels    string                 `yaml:"channels,omitempty"`
	Installed   string                 `yaml:"installed,omitempty"`
	Config      map[string]interface{} `yaml:"config,omitempty"`
}

type relationOutput struct {
	Provides map[string]string `json:"provides,omitempty"`
	Requires map[string]string `json:"requires,omitempty"`
}

type charmInfoWriter struct {
	infoWriter
}

func (c charmInfoWriter) Print() error {
	out := &charmInfoOutput{
		Name:        c.in.Name,
		ID:          c.in.ID,
		Summary:     c.in.Summary,
		Publisher:   c.in.Publisher,
		Supports:    strings.Join(basesDisplay(c.in.Supports), ", "),
		StoreURL:    c.in.StoreURL,
		Description: c.in.Description,
		Channels:    c.channels(),
		Tags:        strings.Join(c.in.Tags, ", "),
	}
	if c.in.Charm != nil {
		out.Subordinate = c.in.Charm.Subordinate
		if c.displayConfig && c.in.Charm.Config != nil {
			out.Config = make(map[string]interface{}, 1)
			out.Config["settings"] = c.in.Charm.Config.Options
		}
	}
	if rels, err := c.relations(); err == nil {
		out.Relations = rels
	}
	return c.print(out)
}

func (c charmInfoWriter) relations() (relationOutput, error) {
	if c.in.Charm == nil {
		return relationOutput{}, errors.NotFoundf("charm")
	}
	requires, foundRequires := c.in.Charm.Relations["requires"]
	provides, foundProvides := c.in.Charm.Relations["provides"]
	if !foundProvides && !foundRequires {
		return relationOutput{}, errors.NotFoundf("charm relations")
	}
	var relations relationOutput
	if foundProvides {
		relations.Provides = provides
	}
	if foundRequires {
		relations.Requires = requires
	}
	return relations, nil
}

func sizeToStr(size int) string {
	suffixes := []string{"B", "kB", "MB", "GB", "TB", "PB", "EB"}
	for _, suf := range suffixes {
		if size < 1000 {
			return fmt.Sprintf("%d%s", size, suf)
		}
		size /= 1000
	}
	return ""
}

// InfoUnicodeWriter creates a unicode writer that wraps a writer for writing
// data from the info endpoint.
func InfoUnicodeWriter(writer output.Wrapper, mode string) *UnicodeWriter {
	var unicodes map[UnicodeCharIdent]string
	if canUnicode(mode, defaultOSEnviron{}) {
		unicodes = InfoUnicodeMap()
	} else {
		unicodes = InfoASCIIMap()
	}
	return &UnicodeWriter{
		Wrapper:  writer,
		unicodes: unicodes,
	}
}

const (
	UnicodeDash    UnicodeCharIdent = "dash"
	UnicodeUpArrow UnicodeCharIdent = "up-arrow"
	UnicodeTick    UnicodeCharIdent = "tick"
)

// InfoUnicodeMap defines the unicode character map that is used for outputting
// unicode characters.
func InfoUnicodeMap() map[UnicodeCharIdent]string {
	return map[UnicodeCharIdent]string{
		UnicodeDash:    "–",
		UnicodeUpArrow: "↑",
		UnicodeTick:    "✓",
	}
}

// InfoASCIIMap defines the ascii character map that is used for outputting
// the fallback to unicode characters when not supported.
func InfoASCIIMap() map[UnicodeCharIdent]string {
	return map[UnicodeCharIdent]string{
		UnicodeDash:    "--",
		UnicodeUpArrow: "^",
		UnicodeTick:    "*",
	}
}
