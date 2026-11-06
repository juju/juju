// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"go/token"
	"strings"
)

// SegmentKind classifies a span of rendered SQL for source mapping.
type SegmentKind int

const (
	// KindLiteral is SQL text copied from a Go string literal.
	KindLiteral SegmentKind = iota
	// KindReplacement is a dummy token substituted for a sqlair
	// expression or fmt.Sprintf verb.
	KindReplacement
	// KindVirtual is synthetic content (blanked Go, semicolons, newlines).
	KindVirtual
	// KindEscape is SQL produced by expanding a Go escape sequence.
	KindEscape
)

func (k SegmentKind) String() string {
	switch k {
	case KindLiteral:
		return "literal"
	case KindReplacement:
		return "replacement"
	case KindVirtual:
		return "virtual"
	case KindEscape:
		return "escape"
	default:
		return "unknown"
	}
}

// origByte maps one rendered-or-content byte back to the Go source.
type origByte struct {
	File   string
	Offset int
	Kind   SegmentKind
	// RawString records literal bytes copied from a backtick string.
	// The fixer uses it to decide whether inserted newlines need escaping.
	RawString bool
}

// evaluated is reconstructed SQL plus a per-byte origin map.
type evaluated struct {
	Content string
	Map     []origByte
}

func (e *evaluated) empty() bool {
	return e == nil || e.Content == ""
}

func concatEvaluated(parts ...*evaluated) *evaluated {
	var b strings.Builder
	total := 0
	for _, p := range parts {
		if p == nil {
			continue
		}
		total += len(p.Map)
	}
	m := make([]origByte, 0, total)
	for _, p := range parts {
		if p == nil {
			continue
		}
		b.WriteString(p.Content)
		m = append(m, p.Map...)
	}
	return &evaluated{Content: b.String(), Map: m}
}

func (e *evaluated) dropFirstByte() *evaluated {
	if e == nil || len(e.Content) == 0 {
		return e
	}
	return &evaluated{Content: e.Content[1:], Map: e.Map[1:]}
}

func (e *evaluated) span() (file string, start, end int) {
	if e == nil {
		return "", 0, 0
	}
	start = -1
	end = -1
	for _, ob := range e.Map {
		if ob.Kind == KindVirtual || ob.File == "" {
			continue
		}
		if file == "" {
			file = ob.File
		}
		if ob.File != file {
			continue
		}
		if start < 0 || ob.Offset < start {
			start = ob.Offset
		}
		if ob.Offset+1 > end {
			end = ob.Offset + 1
		}
	}
	if start < 0 {
		return file, 0, 0
	}
	return file, start, end
}

// Statement is one extracted SQL statement.
type Statement struct {
	ID          int
	File        string
	Start       int
	End         int
	Eval        *evaluated
	FromSprintf bool
	IgnoreAll   bool
	IgnoreRules map[string]bool
	RelatedPos  []token.Pos
}

func (s *Statement) ignores(rule string) bool {
	if s == nil {
		return false
	}
	if s.IgnoreAll {
		return true
	}
	return s.IgnoreRules[rule]
}

// Skip is a SQL site that could not be reconstructed statically.
type Skip struct {
	File   string
	Line   int
	Column int
	Reason string
	Expr   string
}

// TemplateNote records a SQL-bearing template whose generated Go may
// not be in the repository.
type TemplateNote struct {
	Path    string
	Message string
}

// Segment maps a rendered byte range back to original Go source.
type Segment struct {
	RenderedStart int
	RenderedEnd   int
	OriginalFile  string
	OriginalStart int
	OriginalEnd   int
	Kind          SegmentKind
	StmtID        int
	// ContentStart is the index of the first byte of this segment within
	// the owning statement's evaluated content, or -1 for synthetic
	// segments. The fixer uses it to map rendered positions back to
	// content positions.
	ContentStart int
}

func (s Segment) contains(offset int) bool {
	return offset >= s.RenderedStart && offset < s.RenderedEnd
}

// FileRender is synthetic SQL for one Go file plus its source map.
type FileRender struct {
	GoPath    string
	Synthetic []byte
	Segments  []Segment
}

func (r *FileRender) lookup(offset int) (Segment, bool) {
	if r == nil {
		return Segment{}, false
	}
	segs := r.Segments
	lo, hi := 0, len(segs)
	for lo < hi {
		mid := (lo + hi) / 2
		if segs[mid].RenderedEnd <= offset {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	if lo < len(segs) && segs[lo].contains(offset) {
		return segs[lo], true
	}
	return Segment{}, false
}

// Diagnostic is a lint finding mapped to original Go coordinates.
type Diagnostic struct {
	File    string
	Line    int
	Column  int
	Rule    string
	Message string
	StmtID  int
}

func (d Diagnostic) String() string {
	if d.Rule == "" {
		return fmt.Sprintf("%s:%d:%d: %s", d.File, d.Line, d.Column, d.Message)
	}
	return fmt.Sprintf("%s:%d:%d: %s %s", d.File, d.Line, d.Column, d.Rule, d.Message)
}
