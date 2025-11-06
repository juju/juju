// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package schema

import (
	"embed"
	"io/fs"
	"slices"
	"strings"

	"github.com/juju/juju/core/database/schema"
)

const (
	patchName = ".patch.sql"
)

func readPatches(entries []fs.DirEntry, fs embed.FS, fileName func(string) string) ([]func() schema.Patch, []func() schema.Patch) {
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		names = append(names, entry.Name())
	}

	slices.Sort(names)

	var (
		patches     []func() schema.Patch
		postPatches []func() schema.Patch
	)
	for _, name := range names {
		data, err := fs.ReadFile(fileName(name))
		if err != nil {
			panic(err)
		}

		fn := func() schema.Patch {
			return schema.MakePatch(string(data))
		}

		if strings.HasSuffix(strings.ToLower(name), patchName) {
			postPatches = append(postPatches, fn)
			continue
		}
		patches = append(patches, fn)
	}
	return patches, postPatches
}
