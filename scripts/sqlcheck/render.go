// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"sort"
	"strings"
	"unicode/utf8"
)

func renderPackage(pkg *parsedPackage, stmts []Statement) map[string]*FileRender {
	byFile := map[string][]Statement{}
	for _, st := range stmts {
		if st.Eval.empty() || st.File == "" {
			continue
		}
		byFile[st.File] = append(byFile[st.File], st)
	}
	out := map[string]*FileRender{}
	for path, list := range byFile {
		pf := pkg.files[path]
		if pf == nil {
			continue
		}
		sort.Slice(list, func(i, j int) bool {
			if list[i].Start != list[j].Start {
				return list[i].Start < list[j].Start
			}
			return list[i].ID < list[j].ID
		})
		out[path] = renderFile(pf, list)
	}
	return out
}

func renderFile(pf *parsedFile, stmts []Statement) *FileRender {
	r := &FileRender{GoPath: pf.path}
	goPos := 0
	for _, st := range stmts {
		if st.Start >= goPos {
			r.emitGap(pf.src[goPos:st.Start])
			goPos = max(st.End, st.Start)
		} else {
			r.emitVirtual([]byte("\n"))
		}
		r.emitStatement(st)
	}
	if goPos < len(pf.src) {
		r.emitGap(pf.src[goPos:])
	}
	if len(r.Synthetic) == 0 || r.Synthetic[len(r.Synthetic)-1] != '\n' {
		r.emitVirtual([]byte("\n"))
	}
	return r
}

func (r *FileRender) emitStatement(st Statement) {
	ev := st.Eval
	if ev.empty() {
		return
	}
	// Trim all trailing whitespace so the statement terminator attaches
	// to the last SQL token. Raw strings commonly close with "\n\t\t",
	// and a semicolon emitted after that whitespace is parsed by sqlfluff
	// as a separate (empty) statement, leaving the real one unterminated.
	// The trimmed whitespace is re-emitted after the terminator so any
	// trailing-whitespace findings still map to the original source.
	end := len(ev.Content)
	for end > 0 && isSQLSpace(ev.Content[end-1]) {
		end--
	}
	// If the statement ends in a line comment, terminate before the
	// comment. A semicolon appended after it would itself be commented
	// out, leaving sqlfluff with an unterminated statement.
	terminator := end
	if comment := trailingLineCommentStart(ev.Content[:end]); comment >= 0 {
		terminator = comment
		for terminator > 0 && isSQLSpace(ev.Content[terminator-1]) {
			terminator--
		}
	}
	r.emitEvalPart(ev, 0, terminator, st.ID)
	if !endsWithSemi(ev.Content[:terminator]) {
		r.emitVirtual([]byte(";"))
	}
	r.emitEvalPart(ev, terminator, len(ev.Content), st.ID)
}

// trailingLineCommentStart returns the start of a -- comment on the last
// line of s, or -1 when there is none. Comment markers inside quoted SQL
// strings and identifiers are ignored.
func trailingLineCommentStart(s string) int {
	comment := -1
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if comment >= 0 {
			if c == '\n' {
				comment = -1
			}
			continue
		}
		if quote != 0 {
			if c != quote {
				continue
			}
			if i+1 < len(s) && s[i+1] == quote {
				i++
				continue
			}
			quote = 0
			continue
		}
		switch c {
		case '\'', '"', '`':
			quote = c
		case '-':
			if i+1 < len(s) && s[i+1] == '-' {
				comment = i
				i++
			}
		}
	}
	return comment
}

func isSQLSpace(c byte) bool {
	switch c {
	case ' ', '\t', '\r', '\n':
		return true
	default:
		return false
	}
}

func endsWithSemi(s string) bool {
	for i := len(s) - 1; i >= 0; i-- {
		switch s[i] {
		case ' ', '\t', '\n', '\r':
			continue
		case ';':
			return true
		default:
			return false
		}
	}
	return false
}

func (r *FileRender) emitGap(src []byte) {
	n := 0
	for _, b := range src {
		if b == '\n' {
			n++
		}
	}
	if n == 0 {
		return
	}
	r.emitVirtual([]byte(nNewlines(n)))
}

func nNewlines(n int) string {
	return strings.Repeat("\n", n)
}

func (r *FileRender) emitVirtual(b []byte) {
	if len(b) == 0 {
		return
	}
	start := len(r.Synthetic)
	r.Synthetic = append(r.Synthetic, b...)
	r.Segments = append(r.Segments, Segment{
		RenderedStart: start,
		RenderedEnd:   len(r.Synthetic),
		Kind:          KindVirtual,
		ContentStart:  -1,
	})
}

// emitEvalPart emits the content range [from,to) of ev as segments.
// base is the index of ev.Content[from] within the statement's full
// evaluated content, so segments record content-relative positions
// that the fixer uses to plan source edits.
func (r *FileRender) emitEvalPart(ev *evaluated, from, to, stmtID int) {
	if from >= to {
		return
	}
	runStart := from
	for i := from + 1; i <= to; i++ {
		if i < to && sameRun(ev.Map[i-1], ev.Map[i]) {
			continue
		}
		r.emitRun(ev, runStart, i, stmtID)
		runStart = i
	}
}

func sameRun(a, b origByte) bool {
	if a.Kind != b.Kind || a.File != b.File {
		return false
	}
	switch a.Kind {
	case KindReplacement, KindEscape, KindVirtual:
		return a.Offset == b.Offset
	default:
		return b.Offset == a.Offset+1
	}
}

func (r *FileRender) emitRun(ev *evaluated, start, end, stmtID int) {
	if start >= end {
		return
	}
	off := len(r.Synthetic)
	r.Synthetic = append(r.Synthetic, ev.Content[start:end]...)
	ob := ev.Map[start]
	last := ev.Map[end-1]
	origEnd := last.Offset + 1
	if ob.Kind == KindReplacement || ob.Kind == KindEscape {
		origEnd = ob.Offset + 1
	}
	r.Segments = append(r.Segments, Segment{
		RenderedStart: off,
		RenderedEnd:   len(r.Synthetic),
		OriginalFile:  ob.File,
		OriginalStart: ob.Offset,
		OriginalEnd:   origEnd,
		Kind:          ob.Kind,
		StmtID:        stmtID,
		ContentStart:  start,
	})
}

func lineColToOffset(src []byte, line, col int) int {
	if line < 1 {
		line = 1
	}
	if col < 1 {
		col = 1
	}
	cur := 1
	i := 0
	for cur < line && i < len(src) {
		if src[i] == '\n' {
			cur++
		}
		i++
	}
	runes := 0
	for runes < col-1 && i < len(src) && src[i] != '\n' {
		_, size := utf8.DecodeRune(src[i:])
		if size < 1 {
			size = 1
		}
		i += size
		runes++
	}
	return i
}
