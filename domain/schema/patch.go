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
	postPatchFileSuffix = ".PATCH.sql"
)

// ReadFileDirFS is the interface implemented by a file system that
// provides an optimized implementation of [ReadFile] and [ReadDir].
type ReadFileDirFS interface {
	fs.ReadFileFS
	fs.ReadDirFS
}

// readPatches reads all the patch files in the given baseDir from the provided
// file system. We exclude any post patch files, which are identified by their
// file name suffix .PATCH.sql.
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
		if strings.HasSuffix(name, postPatchFileSuffix) {
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

// readPostPatches reads the specified post patch files from the given baseDir.
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
