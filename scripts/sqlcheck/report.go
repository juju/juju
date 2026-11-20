// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"go/token"
	"io"
	"os"
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
	// The runner consumes the file, line and col command properties to
	// build annotations and echoes only the message into the job log, so
	// the location is repeated in the message to keep the raw log usable.
	return fmt.Sprintf("::error file=%s,line=%d,col=%d::%s",
		escapeAnnotation(d.File), d.Line, d.Column, escapeAnnotation(d.String()))
}

func escapeAnnotation(s string) string {
	s = strings.ReplaceAll(s, "%", "%25")
	s = strings.ReplaceAll(s, "\r", "%0D")
	s = strings.ReplaceAll(s, "\n", "%0A")
	return s
}

// sortSkips orders skips by file, line and column.
func sortSkips(skips []Skip) {
	sort.Slice(skips, func(i, j int) bool {
		if skips[i].File != skips[j].File {
			return skips[i].File < skips[j].File
		}
		if skips[i].Line != skips[j].Line {
			return skips[i].Line < skips[j].Line
		}
		return skips[i].Column < skips[j].Column
	})
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
		sortSkips(skips)
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

// maxSummaryRows caps the per-file table so the Markdown stays well
// under GitHub's 1MiB per-step job summary limit even for repo-wide
// runs.
const maxSummaryRows = 1000

// maxSummaryRules caps the rules listed per file row.
const maxSummaryRules = 5

// writeGitHubSummary renders a Markdown report for $GITHUB_STEP_SUMMARY.
// GitHub caps job-log annotations at 10 errors per step and drops the
// rest silently, so a repo-wide run cannot surface every violation
// inline; the summary aggregates violations per file instead and has no
// such cap.
func writeGitHubSummary(
	w io.Writer,
	nStmts, nFiles int,
	diags []Diagnostic,
	skips []Skip,
	notes []TemplateNote,
) error {
	var b strings.Builder
	skips = append([]Skip(nil), skips...)
	sortSkips(skips)
	fmt.Fprintf(&b, "## sqlcheck\n\n")
	fmt.Fprintf(&b, "Extracted %d statement(s) from %d file(s): %d sqlfluff violation(s), %d SQL site(s) not linted.\n\n",
		nStmts, nFiles, len(diags), len(skips))

	if len(diags) == 0 {
		b.WriteString("No sqlfluff violations found.\n\n")
	} else {
		b.WriteString("### Violations per file\n\n")
		b.WriteString("| File | Violations | Rules |\n")
		b.WriteString("|---|---:|---|\n")
		rows := fileRuleRows(diags)
		for i, row := range rows {
			if i == maxSummaryRows {
				fmt.Fprintf(&b, "\n… and %d more file(s) with violations.\n", len(rows)-maxSummaryRows)
				break
			}
			fmt.Fprintf(&b, "| `%s` | %d | %s |\n", row.file, row.total, ruleSummary(row.rules))
		}
		b.WriteString("\n")
	}

	if len(skips) > 0 {
		b.WriteString("### Not linted (could not reconstruct SQL)\n\n")
		b.WriteString("| Location | Reason |\n")
		b.WriteString("|---|---|\n")
		for _, sk := range skips {
			reason := mdCell(sk.Reason)
			if sk.Expr != "" {
				reason += " `" + mdCell(sk.Expr) + "`"
			}
			fmt.Fprintf(&b, "| `%s:%d:%d` | %s |\n",
				filepath.ToSlash(sk.File), sk.Line, sk.Column, reason)
		}
		b.WriteString("\n")
	}

	if len(notes) > 0 {
		b.WriteString("### Template coverage\n\n")
		for _, n := range notes {
			fmt.Fprintf(&b, "- `%s`: %s\n", filepath.ToSlash(n.Path), mdCell(n.Message))
		}
		b.WriteString("\n")
	}

	_, err := io.WriteString(w, b.String())
	return err
}

// appendGitHubSummary appends the Markdown report to the file named by
// $GITHUB_STEP_SUMMARY when running inside GitHub Actions; it is a
// no-op elsewhere.
func appendGitHubSummary(
	nStmts, nFiles int,
	diags []Diagnostic,
	skips []Skip,
	notes []TemplateNote,
) error {
	path := os.Getenv("GITHUB_STEP_SUMMARY")
	if path == "" {
		return nil
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("open step summary: %w", err)
	}
	defer f.Close()
	if err := writeGitHubSummary(f, nStmts, nFiles, diags, skips, notes); err != nil {
		return fmt.Errorf("write step summary: %w", err)
	}
	return nil
}

// ruleCount is the number of violations of one sqlfluff rule in one
// file.
type ruleCount struct {
	rule  string
	count int
}

// fileRuleRow aggregates the diagnostics of one file.
type fileRuleRow struct {
	file  string
	total int
	rules []ruleCount
}

// fileRuleRows aggregates diagnostics per file and rule, most violating
// file first.
func fileRuleRows(diags []Diagnostic) []fileRuleRow {
	perFile := map[string]map[string]int{}
	for _, d := range diags {
		rules := perFile[d.File]
		if rules == nil {
			rules = map[string]int{}
			perFile[d.File] = rules
		}
		rules[d.Rule]++
	}
	rows := make([]fileRuleRow, 0, len(perFile))
	for file, rules := range perFile {
		row := fileRuleRow{file: file}
		for rule, count := range rules {
			row.total += count
			row.rules = append(row.rules, ruleCount{rule: rule, count: count})
		}
		sort.Slice(row.rules, func(i, j int) bool {
			if row.rules[i].count != row.rules[j].count {
				return row.rules[i].count > row.rules[j].count
			}
			return row.rules[i].rule < row.rules[j].rule
		})
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].total != rows[j].total {
			return rows[i].total > rows[j].total
		}
		return rows[i].file < rows[j].file
	})
	return rows
}

// ruleSummary renders rule counts as "LT01 (60), LT02 (40)…", listing
// at most maxSummaryRules rules per file.
func ruleSummary(rules []ruleCount) string {
	parts := make([]string, 0, len(rules))
	for i, rc := range rules {
		if i == maxSummaryRules {
			parts = append(parts, fmt.Sprintf("… +%d", len(rules)-maxSummaryRules))
			break
		}
		parts = append(parts, fmt.Sprintf("%s (%d)", rc.rule, rc.count))
	}
	return mdCell(strings.Join(parts, ", "))
}

// mdCell makes a string safe for a Markdown table cell.
func mdCell(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}
