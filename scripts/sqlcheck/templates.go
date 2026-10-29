// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"context"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var templateVarRE = regexp.MustCompile(`\{\{[^}]*\}\}`)

// checkTemplates reports SQL-bearing templates under generate/ whose
// generated Go output cannot be found among the repository's committed
// generated files. Templates whose output is committed are not linted
// directly; their generated Go files are linted with everything else.
func checkTemplates(root string) []TemplateNote {
	genDir := filepath.Join(root, "generate")
	info, err := os.Stat(genDir)
	if err != nil || !info.IsDir() {
		return nil
	}

	var notes []TemplateNote
	// One repo-wide scan of the generated Go output feeds every
	// template check; scanning per template would be
	// O(templates × repository size).
	genFiles := generatedGoFiles(root)
	// Best-effort: a walk failure yields no notes, which surfaces as a
	// template-coverage warning rather than a hard error.
	_ = filepath.WalkDir(genDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		ext := filepath.Ext(path)
		if ext != ".tmpl" && ext != ".gotmpl" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		if !templateLooksLikeSQL(string(data)) {
			return nil
		}
		if templateOutputCommitted(genFiles, data) {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			rel = path
		}
		notes = append(notes, TemplateNote{
			Path:    filepath.ToSlash(rel),
			Message: "SQL-bearing template has no matching committed generated Go output",
		})
		return nil
	})
	return notes
}

// shouldCheckTemplates reports whether the template coverage check should
// run for the given discovery arguments. With no arguments the whole
// repository, including generate/, is in scope. With explicit arguments
// the check only matters when the scope may still include the
// repository's generate/ directory; scoping to concrete files or
// directories elsewhere cannot change template coverage. Glob patterns
// and unreadable paths are treated conservatively, because skipping the
// check must never hide a coverage regression.
func shouldCheckTemplates(root string, args []string) bool {
	if len(args) == 0 {
		return true
	}
	genDir := filepath.Join(root, "generate")
	cwd, err := os.Getwd()
	if err != nil {
		return true
	}
	for _, arg := range args {
		if hasGlobMeta(arg) {
			// A glob may match generated output under generate/.
			return true
		}
		path := arg
		if !filepath.IsAbs(path) {
			path = filepath.Join(cwd, path)
		}
		info, err := os.Stat(path)
		if err != nil {
			// Leave unreadable paths to discovery to report.
			return true
		}
		if pathMayCover(info.IsDir(), path, genDir) {
			return true
		}
	}
	return false
}

// pathMayCover reports whether the file or directory at path may cover
// the target directory: the path is the target or inside it, or the path
// is a directory that contains the target.
func pathMayCover(isDir bool, path, target string) bool {
	if rel, err := filepath.Rel(target, path); err == nil {
		if rel == "." || !strings.HasPrefix(rel, "..") {
			return true
		}
	}
	if !isDir {
		return false
	}
	if rel, err := filepath.Rel(path, target); err == nil {
		if rel == "." || !strings.HasPrefix(rel, "..") {
			return true
		}
	}
	return false
}

// templateOutputCommitted reports whether any generated Go file contains
// SQL matching one of the template's distinctive SQL lines. Template
// variables are treated as wildcards when matching.
func templateOutputCommitted(genFiles []goFile, tmpl []byte) bool {
	pats := templateSQLPatterns(tmpl)
	if len(pats) == 0 {
		return false
	}
	for _, gf := range genFiles {
		for _, p := range pats {
			if p.Match(gf.Src) {
				return true
			}
		}
	}
	return false
}

// generatedGoFiles returns the fully-read contents of every tracked Go
// file that carries a "Code generated" header. The template coverage
// check needs repo-wide generated output even when linting is scoped to
// specific paths.
func generatedGoFiles(root string) []goFile {
	rels, err := discoverGoFiles(context.Background(), root, nil)
	if err != nil {
		return nil
	}
	var out []goFile
	for _, rel := range rels {
		path := filepath.Join(root, rel)
		head, ok := readHead(path, 512)
		if !ok || !strings.Contains(head, "Code generated") {
			continue
		}
		src, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		out = append(out, goFile{Path: rel, Src: src})
	}
	return out
}

// readHead returns the first n bytes of a file. It reports false when the
// file cannot be opened or read.
func readHead(path string, n int64) (string, bool) {
	f, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer f.Close()
	head, err := io.ReadAll(io.LimitReader(f, n))
	if err != nil {
		return "", false
	}
	return string(head), true
}

// minTemplateStaticLen is the smallest total size of static text that is
// distinctive enough to anchor a search pattern; shorter fragments risk
// matching unrelated lines.
const minTemplateStaticLen = 12

// templateSQLPatterns builds one pattern per SQL-bearing template line,
// with the static fragments anchored and template variables acting as
// bounded wildcards. Lines whose static fragments are too short to be
// distinctive are skipped.
func templateSQLPatterns(tmpl []byte) []*regexp.Regexp {
	var pats []*regexp.Regexp
	for line := range strings.SplitSeq(string(tmpl), "\n") {
		if !templateLooksLikeSQL(line) {
			continue
		}
		parts := templateVarRE.Split(line, -1)
		static := 0
		for _, part := range parts {
			static += len(part)
		}
		if static < minTemplateStaticLen {
			continue
		}
		var b strings.Builder
		for i, part := range parts {
			if i > 0 {
				b.WriteString(`[\s\S]{0,160}?`)
			}
			b.WriteString(regexp.QuoteMeta(part))
		}
		re, err := regexp.Compile(b.String())
		if err != nil {
			continue
		}
		pats = append(pats, re)
	}
	return pats
}

func templateLooksLikeSQL(s string) bool {
	upper := strings.ToUpper(s)
	for _, kw := range []string{"SELECT ", "INSERT ", "UPDATE ", "DELETE ", "CREATE ", "ALTER "} {
		if strings.Contains(upper, kw) {
			return true
		}
	}
	return strings.Contains(s, "sqlair.Prepare") || strings.Contains(s, "MakePatch")
}
