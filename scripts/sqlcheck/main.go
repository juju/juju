// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"go/token"
	"maps"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
)

const maxFixPasses = 10

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	fs := flag.NewFlagSet("sqlcheck", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	reportOnly := fs.Bool("report-only", false, "print violations but exit 0 unless the tool itself fails")
	github := fs.Bool("github", false, "emit GitHub Actions error annotations")
	writeDir := fs.String("write-extracted", "", "write rendered SQL into this directory instead of a temp dir")
	config := fs.String("config", "", "path to sqlfluff config (default <repo>/.github/.sqlfluff)")
	rules := fs.String("rules", "", "only report these sqlfluff rules, comma-separated (e.g. LT01,CP01)")
	fix := fs.Bool("fix", false, "apply the mechanical fixes for spacing and indentation violations to the Go sources, then re-lint")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "usage: sqlcheck [flags] [files-or-dirs-or-globs...]\n")
		fmt.Fprintf(fs.Output(), "  globs match tracked files, e.g. 'domain/*/state/**' or '**/*_test.go'\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "sqlcheck: %v\n", err)
		return 2
	}
	root, err := findRepoRoot(cwd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sqlcheck: %v\n", err)
		return 2
	}

	if os.Getenv("GITHUB_ACTIONS") == "true" {
		*github = true
	}

	cfg := *config
	if cfg == "" {
		cfg = filepath.Join(root, ".github", ".sqlfluff")
	}
	if !filepath.IsAbs(cfg) {
		cfg = filepath.Join(cwd, cfg)
	}
	if _, err := os.Stat(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "sqlcheck: sqlfluff config not found: %s\n", cfg)
		return 2
	}

	bin, err := lookSQLFluff()
	if err != nil {
		fmt.Fprintf(os.Stderr, "sqlcheck: %v\n", err)
		return 2
	}

	rels, err := discoverGoFiles(ctx, root, fs.Args())
	if err != nil {
		fmt.Fprintf(os.Stderr, "sqlcheck: %v\n", err)
		return 2
	}
	ruleFilter := parseRules(*rules)

	extractDir := *writeDir
	cleanup := false
	if extractDir == "" {
		tmp, err := os.MkdirTemp("", "sqlcheck-")
		if err != nil {
			fmt.Fprintf(os.Stderr, "sqlcheck: %v\n", err)
			return 2
		}
		extractDir = tmp
		cleanup = true
	}
	if cleanup {
		defer os.RemoveAll(extractDir)
	}

	var pass *lintPass
	for iter := 0; ; iter++ {
		pass, err = runLintPass(ctx, bin, cfg, root, rels, ruleFilter, extractDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "sqlcheck: %v\n", err)
			return 2
		}
		if !*fix || iter >= maxFixPasses || len(pass.fluff) == 0 {
			break
		}
		edits, skipped := planFixes(pass.stmts, pass.absSynth, pass.fluff)
		if len(edits) == 0 {
			break
		}
		changed, err := applyEdits(root, edits)
		if err != nil {
			fmt.Fprintf(os.Stderr, "sqlcheck: %v\n", err)
			return 2
		}
		fmt.Fprintf(os.Stderr, "sqlcheck: fix pass %d: applied %d edit(s) to %d file(s); %d violation(s) not auto-fixable\n",
			iter+1, len(edits), changed, skipped)
	}

	for _, d := range pass.diags {
		fmt.Println(formatDiagnostic(d, *github))
	}
	var notes []TemplateNote
	if shouldCheckTemplates(root, fs.Args()) {
		notes = checkTemplates(root)
	}
	fmt.Fprint(os.Stderr, writeSummary(len(pass.stmts), pass.nFiles, pass.skips, notes, len(pass.diags)))

	if len(pass.diags) > 0 && !*reportOnly {
		return 1
	}
	return 0
}

// lintPass is the outcome of one extract, render, lint and map pass over
// the Go files in scope.
type lintPass struct {
	fset     *token.FileSet
	stmts    []Statement
	skips    []Skip
	absSynth map[string]*FileRender
	fluff    []fluffFile
	diags    []Diagnostic
	nFiles   int
}

func runLintPass(
	ctx context.Context,
	bin, cfg, root string,
	rels []string,
	ruleFilter map[string]bool,
	extractDir string,
) (*lintPass, error) {
	files, err := readGoFiles(root, rels)
	if err != nil {
		return nil, err
	}

	fset := token.NewFileSet()
	pass := &lintPass{fset: fset}
	renders := map[string]*FileRender{}
	nextID := 1
	for _, group := range groupByPackage(files) {
		pkg, err := parsePackage(fset, group)
		if err != nil {
			return nil, err
		}
		st, sk := extractPackage(pkg, &nextID)
		pass.stmts = append(pass.stmts, st...)
		pass.skips = append(pass.skips, sk...)
		maps.Copy(renders, renderPackage(pkg, st))
	}

	bySynth, err := writeExtracted(extractDir, renders)
	if err != nil {
		return nil, err
	}
	pass.absSynth = map[string]*FileRender{}
	for p, r := range bySynth {
		abs, err := filepath.Abs(p)
		if err != nil {
			abs = p
		}
		pass.absSynth[abs] = r
		pass.absSynth[p] = r
	}

	if len(bySynth) > 0 {
		fluff, _, err := runSQLFluff(ctx, bin, cfg, extractDir, ruleFilter)
		if err != nil {
			return nil, err
		}
		pass.fluff = fluff
	}
	pass.diags = mapDiagnostics(fset, pass.stmts, pass.absSynth, pass.fluff)
	if len(ruleFilter) > 0 {
		kept := make([]Diagnostic, 0, len(pass.diags))
		for _, d := range pass.diags {
			if ruleFilter[d.Rule] {
				kept = append(kept, d)
			}
		}
		pass.diags = kept
	}

	seen := map[string]bool{}
	for _, st := range pass.stmts {
		if !seen[st.File] {
			seen[st.File] = true
			pass.nFiles++
		}
	}
	return pass, nil
}
