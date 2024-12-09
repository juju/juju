// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"

	"github.com/juju/errors"

	"github.com/juju/juju/internal/charm"
)

func repackageCharmWithRevision(ctx context.Context, archive *charm.CharmArchive, charmRevision int) (*charm.CharmArchive, bytes.Buffer, string, error) {
	// Create a temp dir to contain the extracted charm dir.
	tempDir, err := os.MkdirTemp("", "charm-download")
	if err != nil {
		return archive, bytes.Buffer{}, "", errors.Annotate(err, "cannot create temp directory")
	}
	defer os.RemoveAll(tempDir)
	extractPath := filepath.Join(tempDir, "extracted")

	// Expand and repack it with the specified revision
	archive.SetRevision(charmRevision)
	if err := archive.ExpandTo(extractPath); err != nil {
		return archive, bytes.Buffer{}, "", errors.Annotate(err, "cannot extract uploaded charm")
	}

	charmDir, err := charm.ReadCharmDir(extractPath)
	if err != nil {
		return archive, bytes.Buffer{}, "", errors.Annotate(err, "cannot read extracted charm")
	}

	// Bundle the charm and calculate its sha256 hash at the same time.
	var repackagedArchiveBuf bytes.Buffer
	hash := sha256.New()
	err = charmDir.ArchiveTo(io.MultiWriter(hash, &repackagedArchiveBuf))
	if err != nil {
		return archive, bytes.Buffer{}, "", errors.Annotate(err, "cannot repackage uploaded charm")
	}
	repackagedSHA256 := hex.EncodeToString(hash.Sum(nil))

	return archive, repackagedArchiveBuf, repackagedSHA256, nil
}
