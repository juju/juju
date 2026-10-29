// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"go/token"
	"path/filepath"
	"sort"
	"strings"
)

func mapDiagnostics(
	fset *token.FileSet,
	stmts []Statement,
	bySynth map[string]*FileRender,
	files []fluffFile,
) []Diagnostic {
	byID := map[int]*Statement{}
	for i := range stmts {
		byID[stmts[i].ID] = &stmts[i]
	}
	var out []Diagnostic
	for _, ff := range files {
		rend := matchSynth(bySynth, ff.Filepath)
		if rend == nil {
			abs, err := filepath.Abs(ff.Filepath)
			if err == nil {
				rend = matchSynth(bySynth, abs)
			}
		}
		if rend == nil {
			continue
		}
		for _, v := range ff.Violations {
			d, ok := mapViolation(fset, rend, byID, v)
			if !ok {
				continue
			}
			out = append(out, d)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.File != b.File {
			return a.File < b.File
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		if a.Column != b.Column {
			return a.Column < b.Column
		}
		if a.Rule != b.Rule {
			return a.Rule < b.Rule
		}
		return a.Message < b.Message
	})
	return out
}

func mapViolation(
	fset *token.FileSet,
	rend *FileRender,
	byID map[int]*Statement,
	v fluffViolation,
) (Diagnostic, bool) {
	offset := lineColToOffset(rend.Synthetic, v.line(), v.col())
	if offset >= len(rend.Synthetic) && len(rend.Synthetic) > 0 {
		offset = len(rend.Synthetic) - 1
	}
	seg, ok := rend.lookup(offset)
	if !ok {
		return Diagnostic{}, false
	}
	if seg.Kind == KindVirtual {
		return Diagnostic{}, false
	}
	st := byID[seg.StmtID]
	rule := v.rule()
	if st != nil && st.ignores(rule) {
		return Diagnostic{}, false
	}
	origOff := seg.OriginalStart
	if seg.Kind == KindLiteral && offset >= seg.RenderedStart {
		origOff = seg.OriginalStart + (offset - seg.RenderedStart)
		if seg.OriginalEnd > seg.OriginalStart && origOff >= seg.OriginalEnd {
			origOff = seg.OriginalEnd - 1
		}
	}
	file := seg.OriginalFile
	if file == "" {
		file = rend.GoPath
	}
	pos := filePosition(fset, file, origOff)
	msg := v.Description
	if msg == "" {
		msg = v.Name
	}
	return Diagnostic{
		File:    filepath.ToSlash(file),
		Line:    pos.Line,
		Column:  pos.Column,
		Rule:    rule,
		Message: msg,
		StmtID:  seg.StmtID,
	}, true
}

func filePosition(fset *token.FileSet, file string, offset int) token.Position {
	var found *token.File
	fset.Iterate(func(f *token.File) bool {
		if f.Name() == file {
			found = f
			return false
		}
		return true
	})
	if found == nil {
		return token.Position{Filename: file}
	}
	if offset < 0 {
		offset = 0
	}
	if offset > found.Size() {
		offset = found.Size()
	}
	return found.Position(found.Pos(offset))
}

func formatDiagnostic(d Diagnostic, github bool) string {
	if !github {
		return d.String()
	}
	msg := d.Rule
	if msg != "" {
		msg += " "
	}
	msg += d.Message
	return fmt.Sprintf("::error file=%s,line=%d,col=%d::%s",
		escapeAnnotation(d.File), d.Line, d.Column, escapeAnnotation(msg))
}

func escapeAnnotation(s string) string {
	s = strings.ReplaceAll(s, "%", "%25")
	s = strings.ReplaceAll(s, "\r", "%0D")
	s = strings.ReplaceAll(s, "\n", "%0A")
	return s
}

func writeSummary(
	nStmts, nFiles int,
	skips []Skip,
	notes []TemplateNote,
	nDiags int,
) string {
	var b strings.Builder
	fmt.Fprintf(&b, "sqlcheck: extracted %d statement(s) from %d file(s)\n", nStmts, nFiles)
	fmt.Fprintf(&b, "sqlcheck: %d sqlfluff violation(s)\n", nDiags)
	if len(skips) == 0 {
		fmt.Fprintf(&b, "sqlcheck: 0 SQL site(s) not linted\n")
	} else {
		fmt.Fprintf(&b, "sqlcheck: %d SQL site(s) not linted (could not reconstruct SQL)\n", len(skips))
		sort.Slice(skips, func(i, j int) bool {
			if skips[i].File != skips[j].File {
				return skips[i].File < skips[j].File
			}
			if skips[i].Line != skips[j].Line {
				return skips[i].Line < skips[j].Line
			}
			return skips[i].Column < skips[j].Column
		})
		for _, sk := range skips {
			fmt.Fprintf(&b, "  not linted: %s:%d:%d: %s\n",
				filepath.ToSlash(sk.File), sk.Line, sk.Column, sk.Reason)
			if sk.Expr != "" {
				fmt.Fprintf(&b, "    %s\n", sk.Expr)
			}
		}
	}
	for _, n := range notes {
		fmt.Fprintf(&b, "sqlcheck: template %s: %s\n", filepath.ToSlash(n.Path), n.Message)
	}
	return b.String()
}
