// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package recovery

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path"
	"slices"
	"strings"

	corebackups "github.com/juju/juju/core/backups"
	"github.com/juju/juju/internal/errors"
)

const (
	// contentDir is the single top-level directory inside a backup
	// archive. It mirrors the layout constant in core/backups; the
	// recovery reader never consumes entries outside it.
	contentDir = "juju-backup"

	// metadataPath is the archive's metadata file: the provenance
	// record holding the source agent version.
	metadataPath = contentDir + "/metadata.json"

	// manifestPath is the archive's content manifest: the index of
	// the archive's components with their sizes and SHA-256 hashes.
	manifestPath = contentDir + "/manifest.json"

	// controllerDumpPath is the controller database dump.
	controllerDumpPath = contentDir + "/dump/controller.yaml"

	// modelDumpDir holds one dump per model, named by model UUID.
	modelDumpDir = contentDir + "/dump/models/"

	// maxMetadataSize bounds the metadata read from an archive.
	maxMetadataSize = 4 << 20

	// maxDumpSize bounds a single database dump read from an archive.
	maxDumpSize = 1 << 30
)

// MaxDumpSize is the largest single database dump the recovery loader
// accepts: the loader rejects any dump exceeding it, so backup stages its
// dumps against the same bound and fails fast instead of producing an
// archive that no recovery can load.
const MaxDumpSize = maxDumpSize

// ArchiveContents holds the files extracted while streaming an archive.
// Model dumps are staged on disk; the object blob bundle is streamed
// through the checksum but never stored. Close must be called once the
// contents have been consumed to remove the staged dumps.
type ArchiveContents struct {
	// Metadata is the archive's metadata.json provenance record.
	Metadata []byte

	// Manifest is the archive's content manifest, JSON. It is empty
	// for archives written before the manifest was introduced: the
	// manifest is cross-checked when present, never required.
	Manifest []byte

	// ControllerDump is the controller database dump, YAML.
	ControllerDump []byte

	// ModelDumps holds the path of each staged dump, keyed by model UUID.
	// The files are private to this archive and remain until Close.
	ModelDumps map[string]string

	stagingDir string
}

// readArchive streams archivePath once, computing the archive's SHA-256
// checksum and extracting only the metadata and database dumps. The
// object blob bundle is streamed through the hash but never held in
// memory.
//
// The reader rejects absolute paths, parent traversal, duplicate entries
// and non-regular files: an archive is operator-supplied but never
// trusted beyond its checksum. When the archive carries a content
// manifest, it is cross-checked against the entries actually read.
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
	defer func() { _ = gz.Close() }()

	stagingDir, err := os.MkdirTemp("", "juju-recovery-dump-")
	if err != nil {
		return nil, "", 0, errors.Errorf("creating recovery workspace: %w", err)
	}
	contents := &ArchiveContents{
		ModelDumps: make(map[string]string),
		stagingDir: stagingDir,
	}
	completed := false
	defer func() {
		if !completed {
			_ = contents.Close()
		}
	}()
	seen := make(map[string]struct{})
	digests := make(map[string]entryDigest)
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
		case name == manifestPath:
			limit = maxMetadataSize
			store = func(b []byte) { contents.Manifest = b }
		case name == controllerDumpPath:
			limit = maxDumpSize
			store = func(b []byte) { contents.ControllerDump = b }
		case strings.HasPrefix(name, modelDumpDir) && strings.HasSuffix(name, ".yaml"):
			model := strings.TrimSuffix(strings.TrimPrefix(name, modelDumpDir), ".yaml")
			if model == "" || strings.Contains(model, "/") {
				return nil, "", 0, errors.Errorf("unexpected model dump path %q", name)
			}
			// Check the declared size before staging. The tar reader
			// exposes at most this entry's declared number of bytes.
			if hdr.Size > maxDumpSize {
				return nil, "", 0, errors.Errorf("reading %q: file exceeds %d bytes", name, maxDumpSize)
			}
			filename, digest, err := stageModelDump(ctx, stagingDir, tr)
			if err != nil {
				return nil, "", 0, errors.Errorf("reading %q: %w", name, err)
			}
			contents.ModelDumps[model] = filename
			digests[name] = digest
			continue
		default:
			// root.tar and anything else is hashed but not held.
			hasher := sha256.New()
			size, err := io.Copy(io.MultiWriter(io.Discard, hasher), tr)
			if err != nil {
				return nil, "", 0, errors.Errorf("reading %q: %w", name, err)
			}
			digests[name] = entryDigest{size: size, sha256: hex.EncodeToString(hasher.Sum(nil))}
			continue
		}

		data, err := readLimited(tr, limit)
		if err != nil {
			return nil, "", 0, errors.Errorf("reading %q: %w", name, err)
		}
		sum := sha256.Sum256(data)
		digests[name] = entryDigest{size: int64(len(data)), sha256: hex.EncodeToString(sum[:])}
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
	if err := checkManifest(contents.Manifest, digests); err != nil {
		return nil, "", 0, errors.Capture(err)
	}

	stat, err := os.Stat(archivePath)
	if err != nil {
		return nil, "", 0, errors.Capture(err)
	}
	completed = true
	return contents, checksum, stat.Size(), nil
}

// Close removes the staged model dumps. It is safe to call more than once.
func (c *ArchiveContents) Close() error {
	return errors.Capture(os.RemoveAll(c.stagingDir))
}

// stageModelDump streams one model dump to a private staging file while
// computing the digest used to check the manifest. It closes the file
// before returning, so the number of open files is independent of the
// number of models.
func stageModelDump(ctx context.Context, dir string, reader io.Reader) (string, entryDigest, error) {
	file, err := os.CreateTemp(dir, "model-*.yaml")
	if err != nil {
		return "", entryDigest{}, errors.Capture(err)
	}
	defer func() { _ = file.Close() }()
	hasher := sha256.New()
	size, err := io.Copy(io.MultiWriter(file, hasher), contextReader{ctx: ctx, reader: reader})
	if err != nil {
		return "", entryDigest{}, errors.Capture(err)
	}
	if err := file.Close(); err != nil {
		return "", entryDigest{}, errors.Capture(err)
	}
	return file.Name(), entryDigest{
		size:   size,
		sha256: hex.EncodeToString(hasher.Sum(nil)),
	}, nil
}

// contextReader checks cancellation between reads from a local file or
// archive entry. It owns neither the reader nor its lifetime.
type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
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

// entryDigest is one archive entry's uncompressed size and SHA-256
// content hash, hex encoded, computed while streaming the archive.
type entryDigest struct {
	size   int64
	sha256 string
}

// checkManifest cross-checks the archive's content manifest against the
// entries actually read: every manifest entry must exist in the archive
// with the recorded size and hash, and every archive entry but the
// manifest itself must be listed. A manifest that disagrees with the
// archive in either direction fails the archive: it is an index, never
// a source of truth. An absent manifest passes, so archives written
// before the manifest was introduced remain recoverable.
func checkManifest(data []byte, digests map[string]entryDigest) error {
	if len(data) == 0 {
		return nil
	}
	manifest, err := corebackups.NewManifestJSONReader(bytes.NewReader(data))
	if err != nil {
		return errors.Errorf("parsing %s: %w", manifestPath, err)
	}
	listed := make(map[string]struct{}, len(manifest.Files))
	for _, f := range manifest.Files {
		if _, dup := listed[f.Path]; dup {
			return errors.Errorf("%s lists %q twice", manifestPath, f.Path)
		}
		listed[f.Path] = struct{}{}
		switch f.Kind {
		case corebackups.ManifestKindMetadata,
			corebackups.ManifestKindFilesBundle,
			corebackups.ManifestKindControllerDump,
			corebackups.ManifestKindModelDump:
		default:
			// A component kind recovery does not recognise is one it
			// cannot account for; backup creation never writes one.
			return errors.Errorf("%s lists %q with unrecognised kind %q",
				manifestPath, f.Path, f.Kind)
		}
		if f.Kind == corebackups.ManifestKindModelDump &&
			f.Path != modelDumpDir+f.ModelUUID+".yaml" {
			return errors.Errorf(
				"%s lists model dump %q with mismatched model UUID %q",
				manifestPath, f.Path, f.ModelUUID)
		}
		d, ok := digests[f.Path]
		if !ok {
			return errors.Errorf(
				"%s lists %q, which is not in the archive", manifestPath, f.Path)
		}
		if f.Size != d.size || !strings.EqualFold(f.SHA256, d.sha256) {
			return errors.Errorf(
				"%s records size %d and sha256 %q for %q, but the archive entry has size %d and sha256 %q",
				manifestPath, f.Size, f.SHA256, f.Path, d.size, d.sha256)
		}
	}
	for name := range digests {
		if name == manifestPath {
			continue
		}
		if _, ok := listed[name]; !ok {
			return errors.Errorf(
				"archive entry %q is not listed in %s", name, manifestPath)
		}
	}
	return nil
}
