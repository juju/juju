// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"

	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/errors"
)

func repackageCharmWithRevision(
	archive *charm.CharmArchive,
	charmRevision int,
) (*charm.CharmArchive, bytes.Buffer, string, error) {
	// Create a temp dir to contain the extracted charm dir.
	tempDir, err := os.MkdirTemp("", "charm-download")
	if err != nil {
		return archive, bytes.Buffer{}, "", errors.Errorf("creating temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)
	extractPath := filepath.Join(tempDir, "extracted")

	// Expand and repack it with the specified revision
	archive.SetRevision(charmRevision)
	if err := archive.ExpandTo(extractPath); err != nil {
		return archive, bytes.Buffer{}, "", errors.Errorf("extracting uploaded charm: %w", err)
	}

	charmDir, err := charm.ReadCharmDir(extractPath)
	if err != nil {
		return archive, bytes.Buffer{}, "", errors.Errorf("reading extracted charm: %w", err)
	}

	// Bundle the charm and calculate its sha256 hash at the same time.
	var repackagedArchiveBuf bytes.Buffer
	hash := sha256.New()
	err = charmDir.ArchiveTo(io.MultiWriter(hash, &repackagedArchiveBuf))
	if err != nil {
		return archive, bytes.Buffer{}, "", errors.Errorf("repackaging uploaded charm: %w", err)
	}
	repackagedSHA256 := hex.EncodeToString(hash.Sum(nil))

	return archive, repackagedArchiveBuf, repackagedSHA256, nil
}
