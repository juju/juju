// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package schema

import (
	"io/fs"
	"path/filepath"
	"slices"
	"strings"

	"github.com/juju/juju/core/database/schema"
)

const (
	pathFileSuffix = ".PATCH.sql"
)

type ReadFileDirFS interface {
	fs.ReadFileFS
	fs.ReadDirFS
}

func readPatches(fs ReadFileDirFS, baseDir string) ([]func() schema.Patch, error) {
	entries, err := fs.ReadDir(baseDir)
	if err != nil {
		return nil, err
	}

	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		names = append(names, entry.Name())
	}

	slices.Sort(names)

	var patches []func() schema.Patch
	for _, name := range names {
		if strings.HasSuffix(name, pathFileSuffix) {
			continue
		}

		data, err := fs.ReadFile(filepath.Join(baseDir, name))
		if err != nil {
			return nil, err
		}

		fn := func() schema.Patch {
			return schema.MakePatch(string(data))
		}

		patches = append(patches, fn)
	}
	return patches, nil
}

func readPostPatches(fs fs.ReadFileFS, baseDir string, postPatchFiles []string) ([]func() schema.Patch, error) {
	patches := make([]func() schema.Patch, len(postPatchFiles))
	for i, name := range postPatchFiles {
		data, err := fs.ReadFile(filepath.Join(baseDir, name))
		if err != nil {
			return nil, err
		}

		fn := func() schema.Patch {
			return schema.MakePatch(string(data))
		}

		patches[i] = fn
	}
	return patches, nil
}
