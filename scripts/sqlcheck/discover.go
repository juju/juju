// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type goFile struct {
	Path string
	Src  []byte
}

func findRepoRoot(start string) (string, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	for {
		_, err := os.Stat(filepath.Join(dir, "go.mod"))
		if err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("repository root not found from %s", start)
		}
		dir = parent
	}
}

func discoverGoFiles(ctx context.Context, root string, args []string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(args) > 0 {
		return discoverFromArgs(ctx, root, args)
	}
	paths, err := gitGoFiles(ctx, root)
	if err == nil {
		return filterGoFiles(root, paths), nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var out []string
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		if d.IsDir() {
			if excludedRel(rel) && rel != "." {
				return fs.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" || excludedRel(rel) {
			return nil
		}
		out = append(out, rel)
		return nil
	})
	return out, err
}

func discoverFromArgs(ctx context.Context, root string, args []string) ([]string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	var out []string
	for _, arg := range args {
		if hasGlobMeta(arg) {
			matches, err := globGoFiles(ctx, root, arg)
			if err != nil {
				return nil, err
			}
			out = append(out, matches...)
			continue
		}
		path := arg
		if !filepath.IsAbs(path) {
			path = filepath.Join(cwd, arg)
		}
		info, err := os.Stat(path)
		if err != nil {
			return nil, err
		}
		if info.IsDir() {
			err = filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if d.IsDir() {
					return nil
				}
				if filepath.Ext(p) != ".go" {
					return nil
				}
				rel, relErr := filepath.Rel(root, p)
				if relErr != nil {
					return relErr
				}
				if excludedRel(rel) {
					return nil
				}
				out = append(out, rel)
				return nil
			})
			if err != nil {
				return nil, err
			}
			continue
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil, err
		}
		if filepath.Ext(rel) != ".go" {
			continue
		}
		out = append(out, rel)
	}
	return uniqueStrings(out), nil
}

func gitGoFiles(ctx context.Context, root string) ([]string, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", root, "ls-files", "-z", "--", "*.go")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var paths []string
	for p := range bytes.SplitSeq(out, []byte{0}) {
		if len(p) == 0 {
			continue
		}
		paths = append(paths, string(p))
	}
	return paths, nil
}

// globGoFiles expands a wildcard pattern such as "domain/*/state/**"
// against the tracked Go files of the repository. It returns an error
// when the pattern matches nothing, mirroring git pathspec behaviour.
func globGoFiles(ctx context.Context, root, pattern string) ([]string, error) {
	all, err := discoverGoFiles(ctx, root, nil)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, rel := range all {
		if matchPath(pattern, rel) {
			out = append(out, rel)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("pattern %q matched no tracked Go files", pattern)
	}
	return out, nil
}

func filterGoFiles(root string, paths []string) []string {
	var out []string
	for _, p := range paths {
		rel := filepath.Clean(p)
		if excludedRel(rel) {
			continue
		}
		out = append(out, rel)
	}
	return out
}

func excludedRel(rel string) bool {
	rel = filepath.ToSlash(rel)
	if rel == "scripts/sqlcheck" || strings.HasPrefix(rel, "scripts/sqlcheck/") {
		return true
	}
	for p := range strings.SplitSeq(rel, "/") {
		switch p {
		case ".git", "vendor", "node_modules", "_build":
			return true
		}
	}
	return false
}

func uniqueStrings(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

func readGoFiles(root string, rels []string) ([]goFile, error) {
	out := make([]goFile, 0, len(rels))
	for _, rel := range rels {
		src, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			return nil, err
		}
		out = append(out, goFile{Path: filepath.ToSlash(rel), Src: src})
	}
	return out, nil
}

func groupByPackage(files []goFile) [][]goFile {
	type key struct {
		dir string
		pkg string
	}
	groups := map[key][]goFile{}
	var order []key
	for _, gf := range files {
		pkgName := packageNameOf(gf.Src)
		if pkgName == "" {
			pkgName = filepath.Base(filepath.Dir(gf.Path))
		}
		k := key{dir: filepath.ToSlash(filepath.Dir(gf.Path)), pkg: pkgName}
		if _, ok := groups[k]; !ok {
			order = append(order, k)
		}
		groups[k] = append(groups[k], gf)
	}
	out := make([][]goFile, 0, len(order))
	for _, k := range order {
		out = append(out, groups[k])
	}
	return out
}

func packageNameOf(src []byte) string {
	for line := range bytes.SplitSeq(src, []byte("\n")) {
		trim := bytes.TrimSpace(line)
		if bytes.HasPrefix(trim, []byte("package ")) {
			fields := bytes.Fields(trim)
			if len(fields) >= 2 {
				return string(fields[1])
			}
		}
	}
	return ""
}
