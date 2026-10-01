// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package recovery

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path"
	"slices"
	"strings"

	"github.com/juju/juju/internal/errors"
)

const (
	// contentDir is the single top-level directory inside a backup
	// archive. It mirrors the layout constant in core/backups; the
	// recovery reader never consumes entries outside it.
	contentDir = "juju-backup"

	// metadataPath is the archive's metadata file: the manifest that
	// records the source agent version.
	metadataPath = contentDir + "/metadata.json"

	// controllerDumpPath is the controller database dump.
	controllerDumpPath = contentDir + "/dump/controller.yaml"

	// modelDumpDir holds one dump per model, named by model UUID.
	modelDumpDir = contentDir + "/dump/models/"

	// maxMetadataSize bounds the metadata read from an archive.
	maxMetadataSize = 4 << 20

	// maxDumpSize bounds a single database dump held in memory while
	// validating an archive.
	maxDumpSize = 1 << 30
)

// MaxDumpSize is the largest single database dump the recovery loader
// accepts: the loader rejects any dump exceeding it, so backup stages its
// dumps against the same bound and fails fast instead of producing an
// archive that no recovery can load.
const MaxDumpSize = maxDumpSize

// ArchiveContents holds the files extracted while streaming an archive:
// the metadata and the database dumps. The object blob bundle is
// streamed through the checksum but never held in memory.
type ArchiveContents struct {
	// Metadata is the archive's metadata.json manifest.
	Metadata []byte

	// ControllerDump is the controller database dump, YAML.
	ControllerDump []byte

	// ModelDumps holds one dump per model, keyed by model UUID.
	ModelDumps map[string][]byte
}

// readArchive streams archivePath once, computing the archive's SHA-256
// checksum and extracting only the metadata and database dumps. The
// object blob bundle is streamed through the hash but never held in
// memory.
//
// The reader rejects absolute paths, parent traversal, duplicate entries
// and non-regular files: an archive is operator-supplied but never
// trusted beyond its checksum.
func readArchive(ctx context.Context, archivePath, expectedSHA256 string) (*ArchiveContents, string, int64, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return nil, "", 0, errors.Errorf("opening archive: %w", err)
	}
	defer func() { _ = f.Close() }()

	hasher := sha256.New()
	gz, err := gzip.NewReader(io.TeeReader(f, hasher))
	if err != nil {
		return nil, "", 0, errors.Errorf("reading archive: %w", err)
	}

	contents := &ArchiveContents{ModelDumps: make(map[string][]byte)}
	seen := make(map[string]struct{})
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, "", 0, errors.Errorf("reading archive: %w", err)
		}
		if err := ctx.Err(); err != nil {
			return nil, "", 0, errors.Capture(err)
		}

		// Reject traversal on the raw entry name: cleaning alone would
		// silently neutralize it instead of failing the archive.
		if strings.HasPrefix(hdr.Name, "/") || slices.Contains(strings.Split(hdr.Name, "/"), "..") {
			return nil, "", 0, errors.Errorf("archive contains unsafe path %q", hdr.Name)
		}
		name := path.Clean(hdr.Name)
		if _, dup := seen[name]; dup {
			return nil, "", 0, errors.Errorf("archive contains duplicate path %q", name)
		}
		seen[name] = struct{}{}

		switch hdr.Typeflag {
		case tar.TypeDir:
			continue
		case tar.TypeReg, tar.TypeRegA:
		default:
			return nil, "", 0, errors.Errorf("archive entry %q is not a regular file", name)
		}

		var limit int64
		var store func([]byte)
		switch {
		case name == metadataPath:
			limit = maxMetadataSize
			store = func(b []byte) { contents.Metadata = b }
		case name == controllerDumpPath:
			limit = maxDumpSize
			store = func(b []byte) { contents.ControllerDump = b }
		case strings.HasPrefix(name, modelDumpDir) && strings.HasSuffix(name, ".yaml"):
			model := strings.TrimSuffix(strings.TrimPrefix(name, modelDumpDir), ".yaml")
			if model == "" || strings.Contains(model, "/") {
				return nil, "", 0, errors.Errorf("unexpected model dump path %q", name)
			}
			limit = maxDumpSize
			store = func(b []byte) { contents.ModelDumps[model] = b }
		default:
			// root.tar and anything else is hashed but not held.
			if _, err := io.Copy(io.Discard, tr); err != nil {
				return nil, "", 0, errors.Errorf("reading %q: %w", name, err)
			}
			continue
		}

		data, err := readLimited(tr, limit)
		if err != nil {
			return nil, "", 0, errors.Errorf("reading %q: %w", name, err)
		}
		store(data)
	}

	checksum := hex.EncodeToString(hasher.Sum(nil))
	if expectedSHA256 != "" && !strings.EqualFold(checksum, expectedSHA256) {
		return nil, "", 0, errors.Errorf(
			"archive checksum mismatch: expected sha256 %q, archive is %q", expectedSHA256, checksum)
	}
	if len(contents.Metadata) == 0 {
		return nil, "", 0, errors.Errorf("archive is missing %s", metadataPath)
	}
	if len(contents.ControllerDump) == 0 {
		return nil, "", 0, errors.Errorf("archive is missing %s", controllerDumpPath)
	}

	stat, err := os.Stat(archivePath)
	if err != nil {
		return nil, "", 0, errors.Capture(err)
	}
	return contents, checksum, stat.Size(), nil
}

// readLimited reads r fully, failing when it holds more than limit bytes.
func readLimited(r io.Reader, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, errors.Capture(err)
	}
	if int64(len(data)) > limit {
		return nil, errors.Errorf("file exceeds %d bytes", limit)
	}
	return data, nil
}
