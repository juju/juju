// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// sourceEdit is a replacement to apply to one Go source file.
type sourceEdit struct {
	file  string
	start int
	end   int
	repl  string
}

// fixMatchers classify violation messages from the pinned sqlfluff
// version into mechanical source edits. Every edit is anchored at the
// violation position and only ever touches bytes that the extractor
// mapped to literal SQL text.
var (
	// sqlfluff escapes tabs and newlines in the Found section, e.g.
	// "Found ' \\t'", so the class accepts both real and escaped runs.
	fixCollapseSpace = regexp.MustCompile(
		`^Expected only single space before .+\. Found '(?:[ \t]|\\t|\\n)+'`)
	fixTrailingSpace = regexp.MustCompile(
		`^Unnecessary trailing whitespace`)
	fixInsertSpace = regexp.MustCompile(
		`^Expected single whitespace between .+ and .+`)
	fixRemoveSpace = regexp.MustCompile(
		`^Unexpected whitespace before .+`)
	fixNoIndent = regexp.MustCompile(
		`^Line should not be indented\.`)
	fixIndent = regexp.MustCompile(
		`^Expected indent of (\d+) spaces\.`)
	fixLineBreakIndent = regexp.MustCompile(
		`^Expected line break and indent of (\d+) spaces before .+`)
	fixLineBreakNoIndent = regexp.MustCompile(
		`^Expected line break and no indent before .+`)
	fixBlankAfterCTE = regexp.MustCompile(
		`^Blank line expected but not found after CTE closing bracket\.`)
)

// planFixes converts the reported violations of the fixable rule
// families into source edits. Violations that do not map to literal
// source content, or whose fix would touch anything other than plain
// literal SQL bytes, are skipped and counted.
func planFixes(
	stmts []Statement,
	bySynth map[string]*FileRender,
	fluff []fluffFile,
) (edits []sourceEdit, skipped int) {
	byID := map[int]*Statement{}
	for i := range stmts {
		byID[stmts[i].ID] = &stmts[i]
	}
	seen := map[sourceEdit]bool{}
	for _, ff := range fluff {
		rend := matchSynth(bySynth, ff.Filepath)
		if rend == nil {
			continue
		}
		for _, v := range ff.Violations {
			edit, ok := planViolationFix(rend, byID, v)
			if !ok {
				skipped++
				continue
			}
			if seen[edit] {
				continue
			}
			seen[edit] = true
			edits = append(edits, edit)
		}
	}
	sort.Slice(edits, func(i, j int) bool {
		if edits[i].file != edits[j].file {
			return edits[i].file < edits[j].file
		}
		return edits[i].start < edits[j].start
	})
	// Drop overlapping edits, keeping the earliest of each conflict.
	var out []sourceEdit
	for i, e := range edits {
		if i > 0 && edits[i-1].file == e.file && e.start < edits[i-1].end {
			skipped++
			continue
		}
		out = append(out, e)
	}
	return out, skipped
}

func planViolationFix(
	rend *FileRender,
	byID map[int]*Statement,
	v fluffViolation,
) (sourceEdit, bool) {
	renderedOff := lineColToOffset(rend.Synthetic, v.line(), v.col())
	if renderedOff >= len(rend.Synthetic) && len(rend.Synthetic) > 0 {
		renderedOff = len(rend.Synthetic) - 1
	}
	seg, ok := rend.lookup(renderedOff)
	if !ok || seg.Kind == KindVirtual || seg.ContentStart < 0 {
		return sourceEdit{}, false
	}
	st := byID[seg.StmtID]
	if st == nil || st.Eval == nil {
		return sourceEdit{}, false
	}
	anchor := seg.ContentStart + (renderedOff - seg.RenderedStart)
	if anchor < 0 || anchor >= len(st.Eval.Content) {
		return sourceEdit{}, false
	}

	msg := v.Description
	switch {
	case fixCollapseSpace.MatchString(msg):
		return collapseSpace(st, anchor)
	case fixTrailingSpace.MatchString(msg):
		return removeTrailingSpace(st, anchor)
	case fixInsertSpace.MatchString(msg):
		return insertAt(st, anchor, " ")
	case fixRemoveSpace.MatchString(msg):
		return removeSpaceBefore(st, anchor)
	case fixNoIndent.MatchString(msg):
		return setLineIndent(st, anchor, 0)
	case fixIndent.MatchString(msg):
		n, ok := fixIndentCount(fixIndent, msg)
		if !ok {
			return sourceEdit{}, false
		}
		return setLineIndent(st, anchor, n)
	case fixLineBreakIndent.MatchString(msg):
		n, ok := fixIndentCount(fixLineBreakIndent, msg)
		if !ok {
			return sourceEdit{}, false
		}
		return insertLineBreak(st, anchor, strings.Repeat(" ", n))
	case fixLineBreakNoIndent.MatchString(msg):
		return insertLineBreak(st, anchor, "")
	case fixBlankAfterCTE.MatchString(msg):
		return insertLineBreak(st, anchor, "")
	default:
		return sourceEdit{}, false
	}
}

// maxAutoFixIndent bounds memory allocation if an unexpected external
// diagnostic contains a pathological indent count.
const maxAutoFixIndent = 1000

func fixIndentCount(re *regexp.Regexp, msg string) (int, bool) {
	n, err := strconv.Atoi(re.FindStringSubmatch(msg)[1])
	return n, err == nil && n <= maxAutoFixIndent
}

// literalSpan verifies that the content range [i,j) maps to contiguous
// literal bytes of a single Go file and returns the corresponding
// source span.
func literalSpan(st *Statement, i, j int) (sourceEdit, bool) {
	if i < 0 || j > len(st.Eval.Content) || i >= j {
		return sourceEdit{}, false
	}
	m := st.Eval.Map
	if m[i].Kind != KindLiteral || m[i].File == "" {
		return sourceEdit{}, false
	}
	for k := i; k < j; k++ {
		if m[k].Kind != KindLiteral || m[k].File != m[i].File {
			return sourceEdit{}, false
		}
		if m[k].Offset != m[i].Offset+(k-i) {
			return sourceEdit{}, false
		}
	}
	return sourceEdit{
		file:  m[i].File,
		start: m[i].Offset,
		end:   m[j-1].Offset + 1,
	}, true
}

// collapseSpace replaces the run of spaces and tabs starting at the
// anchor with a single space.
func collapseSpace(st *Statement, anchor int) (sourceEdit, bool) {
	content := st.Eval.Content
	j := anchor
	for j < len(content) && (content[j] == ' ' || content[j] == '\t') {
		j++
	}
	if j == anchor || j >= len(content) {
		return sourceEdit{}, false
	}
	edit, ok := literalSpan(st, anchor, j)
	if !ok {
		return sourceEdit{}, false
	}
	edit.repl = " "
	return edit, true
}

// removeSpaceBefore removes the run of spaces and tabs starting at the
// anchor entirely, for findings where sqlfluff wants no whitespace at
// all before the following token (function call brackets, commas and
// the like).
func removeSpaceBefore(st *Statement, anchor int) (sourceEdit, bool) {
	content := st.Eval.Content
	j := anchor
	for j < len(content) && (content[j] == ' ' || content[j] == '\t') {
		j++
	}
	if j == anchor || j >= len(content) {
		return sourceEdit{}, false
	}
	edit, ok := literalSpan(st, anchor, j)
	if !ok {
		return sourceEdit{}, false
	}
	edit.repl = ""
	return edit, true
}

// removeTrailingSpace removes the run of spaces and tabs between the
// anchor and the end of the rendered line.
func removeTrailingSpace(st *Statement, anchor int) (sourceEdit, bool) {
	content := st.Eval.Content
	j := anchor
	for j < len(content) && (content[j] == ' ' || content[j] == '\t') {
		j++
	}
	if j == anchor || (j < len(content) && content[j] != '\n') {
		return sourceEdit{}, false
	}
	edit, ok := literalSpan(st, anchor, j)
	if !ok {
		return sourceEdit{}, false
	}
	edit.repl = ""
	return edit, true
}

// setLineIndent replaces the leading whitespace of the content line
// holding the anchor with n spaces. A line with no leading whitespace
// gets an insertion instead.
func setLineIndent(st *Statement, anchor int, n int) (sourceEdit, bool) {
	content := st.Eval.Content
	lineStart := anchor
	for lineStart > 0 && content[lineStart-1] != '\n' {
		lineStart--
	}
	j := lineStart
	for j < len(content) && (content[j] == ' ' || content[j] == '\t') {
		j++
	}
	if j > lineStart {
		edit, ok := literalSpan(st, lineStart, j)
		if !ok {
			return sourceEdit{}, false
		}
		edit.repl = strings.Repeat(" ", n)
		return edit, true
	}
	// No leading whitespace: insert n spaces before the first byte of
	// the line. That byte must be mapped content.
	m := st.Eval.Map
	if m[lineStart].Kind == KindVirtual || m[lineStart].File == "" {
		return sourceEdit{}, false
	}
	return insertAt(st, lineStart, strings.Repeat(" ", n))
}

// atQuoteAfterName reports whether the anchor is a quote character
// directly preceded by a name character, i.e. the pair forms the start
// of a SQLite hexadecimal blob literal such as x” or X'00FF'.
func atQuoteAfterName(st *Statement, anchor int) bool {
	content := st.Eval.Content
	if anchor >= len(content) {
		return false
	}
	if content[anchor] != '\'' && content[anchor] != '"' {
		return false
	}
	if anchor == 0 {
		return false
	}
	prev := content[anchor-1]
	switch {
	case prev >= 'a' && prev <= 'z',
		prev >= 'A' && prev <= 'Z',
		prev >= '0' && prev <= '9',
		prev == '_':
		return true
	}
	return false
}

// insertLineBreak inserts a line break (plus indent) before the mapped
// content byte at anchor, consuming any run of spaces and tabs that
// precedes it so no trailing whitespace is left behind.
func insertLineBreak(st *Statement, anchor int, indent string) (sourceEdit, bool) {
	content := st.Eval.Content
	i := anchor
	for i > 0 && (content[i-1] == ' ' || content[i-1] == '\t') {
		i--
	}
	if i == anchor {
		return insertAt(st, anchor, "\n"+indent)
	}
	edit, ok := literalSpan(st, i, anchor)
	if !ok {
		return insertAt(st, anchor, "\n"+indent)
	}
	text := "\n" + indent
	if !st.Eval.Map[i].RawString {
		text = strings.ReplaceAll(text, "\n", "\\n")
	}
	edit.repl = text
	return edit, true
}

// insertAt inserts text before the mapped content byte at anchor. The
// inserted text may contain newlines, which are escaped when the byte
// belongs to an interpreted string literal.
//
// A space is never inserted between a name character and a following
// quote: in SQLite that adjacency forms a hexadecimal blob literal
// (x'...'), and the pinned sqlfluff mis-parses the empty form x” as an
// identifier followed by a quoted literal.
func insertAt(st *Statement, anchor int, text string) (sourceEdit, bool) {
	m := st.Eval.Map
	ob := m[anchor]
	if ob.Kind != KindLiteral || ob.File == "" {
		return sourceEdit{}, false
	}
	if text == " " && atQuoteAfterName(st, anchor) {
		return sourceEdit{}, false
	}
	if strings.Contains(text, "\n") && !ob.RawString {
		text = strings.ReplaceAll(text, "\n", "\\n")
	}
	return sourceEdit{
		file:  ob.File,
		start: ob.Offset,
		end:   ob.Offset,
		repl:  text,
	}, true
}

// applyEdits applies non-overlapping edits to the source files under
// root and returns how many files changed. Edits are applied bottom-up
// so earlier offsets stay valid.
func applyEdits(root string, edits []sourceEdit) (int, error) {
	byFile := map[string][]sourceEdit{}
	for _, e := range edits {
		byFile[e.file] = append(byFile[e.file], e)
	}
	changed := 0
	for _, file := range slices.Sorted(maps.Keys(byFile)) {
		list := byFile[file]
		sort.Slice(list, func(i, j int) bool { return list[i].start > list[j].start })
		data, err := os.ReadFile(filepath.Join(root, file))
		if err != nil {
			return changed, err
		}
		for _, e := range list {
			if e.start < 0 || e.end > len(data) || e.start > e.end {
				return changed, fmt.Errorf("edit out of range in %s: [%d,%d)", file, e.start, e.end)
			}
			data = append(data[:e.start], append([]byte(e.repl), data[e.end:]...)...)
		}
		if err := os.WriteFile(filepath.Join(root, file), data, 0o644); err != nil {
			return changed, err
		}
		changed++
	}
	return changed, nil
}
