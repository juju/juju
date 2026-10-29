// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
)

type fluffFile struct {
	Filepath   string           `json:"filepath"`
	Violations []fluffViolation `json:"violations"`
}

type fluffViolation struct {
	LineNo       int    `json:"line_no"`
	LinePos      int    `json:"line_pos"`
	StartLineNo  int    `json:"start_line_no"`
	StartLinePos int    `json:"start_line_pos"`
	Code         string `json:"code"`
	Description  string `json:"description"`
	Name         string `json:"name"`
}

func (v fluffViolation) line() int {
	if v.StartLineNo != 0 {
		return v.StartLineNo
	}
	return v.LineNo
}

func (v fluffViolation) col() int {
	if v.StartLinePos != 0 {
		return v.StartLinePos
	}
	return v.LinePos
}

func (v fluffViolation) rule() string {
	if v.Code != "" {
		return v.Code
	}
	return v.Name
}

func writeExtracted(dir string, renders map[string]*FileRender) (map[string]*FileRender, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	bySynth := map[string]*FileRender{}
	for path, rend := range renders {
		if len(rend.Synthetic) == 0 {
			continue
		}
		synth := filepath.Join(dir, filepath.FromSlash(path)+".sql")
		if err := os.MkdirAll(filepath.Dir(synth), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(synth, rend.Synthetic, 0o644); err != nil {
			return nil, err
		}
		bySynth[synth] = rend
	}
	return bySynth, nil
}

func runSQLFluff(ctx context.Context, bin, config, dir string, rules map[string]bool) ([]fluffFile, string, error) {
	args := []string{"lint", "--config", config, "--format", "json"}
	if len(rules) > 0 {
		args = append(args, "--rules", joinRules(rules))
	}
	args = append(args, dir)
	cmd := exec.CommandContext(ctx, bin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil && !isFluffLintExit(err) {
		return nil, stderr.String(), fmt.Errorf(
			"sqlfluff lint failed: %w\n%s",
			err, truncate(stderr.String(), maxStderrBytes))
	}
	data := bytes.TrimSpace(stdout.Bytes())
	if len(data) == 0 {
		return nil, stderr.String(), nil
	}
	var files []fluffFile
	if err := json.Unmarshal(data, &files); err != nil {
		return nil, stderr.String(), fmt.Errorf("parsing sqlfluff JSON: %w\n%s", err, truncate(string(data), maxStderrBytes))
	}
	return files, stderr.String(), nil
}

func joinRules(rules map[string]bool) string {
	names := make([]string, 0, len(rules))
	for r := range rules {
		names = append(names, r)
	}
	sort.Strings(names)
	return strings.Join(names, ",")
}

func isFluffLintExit(err error) bool {
	var ex *exec.ExitError
	if !errors.As(err, &ex) {
		return false
	}
	return ex.ExitCode() == 1
}

func lookSQLFluff(root string) (string, error) {
	bin, err := exec.LookPath("sqlfluff")
	if err != nil {
		return "", fmt.Errorf(
			"sqlfluff not found in PATH; install the repository-pinned version: %w",
			err)
	}
	if err := checkSQLFluffVersion(bin, root); err != nil {
		return "", err
	}
	return bin, nil
}

// checkSQLFluffVersion verifies that the sqlfluff on PATH reports the
// version pinned in .github/sqlfluff-version. The fix matchers are
// coupled to the pinned version's violation wording, so a mismatch
// must fail loudly instead of silently never matching. When the pin
// file is absent the check is skipped.
func checkSQLFluffVersion(bin, root string) error {
	pin, err := os.ReadFile(filepath.Join(root, ".github", "sqlfluff-version"))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("reading pinned sqlfluff version: %w", err)
	}
	want := strings.TrimSpace(string(pin))
	out, err := exec.Command(bin, "--version").Output()
	if err != nil {
		return fmt.Errorf("sqlfluff --version failed: %w", err)
	}
	got := parseVersionOutput(string(out))
	if got == "" {
		return fmt.Errorf(
			"could not parse sqlfluff version from %q; the repository pins %s",
			strings.TrimSpace(string(out)), want)
	}
	if got != want {
		return fmt.Errorf(
			"sqlfluff version %s does not match the repository pin %s: install sqlfluff==%s",
			got, want, want)
	}
	return nil
}

// parseVersionOutput extracts the version token from sqlfluff --version
// output, which may be a bare "3.4.2" or the older
// "sqlfluff, version 3.4.2" form.
func parseVersionOutput(out string) string {
	for f := range strings.FieldsSeq(out) {
		if f[0] >= '0' && f[0] <= '9' {
			return f
		}
	}
	return ""
}

// matchSynth maps a sqlfluff-reported path back to its FileRender.
// Direct and cleaned lookups are tried first. The heuristic fallback
// (basename and suffix matching) considers candidates in sorted key
// order and only returns a match when it is unambiguous; if several
// synthetic files match equally, no guess is made.
func matchSynth(bySynth map[string]*FileRender, fluffPath string) *FileRender {
	if r, ok := bySynth[fluffPath]; ok {
		return r
	}
	cleaned := filepath.Clean(fluffPath)
	if r, ok := bySynth[cleaned]; ok {
		return r
	}
	fluffSlash := filepath.ToSlash(fluffPath)
	var candidates []string
	seen := map[*FileRender]bool{}
	for _, synth := range slices.Sorted(maps.Keys(bySynth)) {
		rend := bySynth[synth]
		if seen[rend] {
			// Absolute and relative aliases of one render are not an
			// ambiguity.
			continue
		}
		synthSlash := filepath.ToSlash(synth)
		matches := filepath.Base(synth) == filepath.Base(fluffPath) ||
			strings.HasSuffix(synthSlash, fluffSlash) ||
			strings.HasSuffix(fluffSlash, synthSlash)
		if !matches {
			continue
		}
		seen[rend] = true
		candidates = append(candidates, synth)
	}
	if len(candidates) != 1 {
		// No heuristic match, or several equally plausible ones.
		return nil
	}
	return bySynth[candidates[0]]
}

// maxStderrBytes bounds the size of sqlfluff's stderr or JSON dump that is
// embedded in error messages; beyond that, the tail is unlikely to help.
const maxStderrBytes = 2000

// truncate returns s, with the tail replaced by "..." when it exceeds n
// bytes. The result may be up to n+3 bytes long.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
