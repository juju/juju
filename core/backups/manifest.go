// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"bytes"
	"encoding/json"
	"io"
	"path"
	"strings"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/errors"
)

// manifestFormatVersion is the current manifest format. Version 1 is
// the initial content index: per-component path, kind, size and SHA-256.
const manifestFormatVersion = 1

// ManifestEntryKind classifies the archive component a manifest entry
// records.
type ManifestEntryKind string

const (
	// ManifestKindMetadata is the archive's metadata.json.
	ManifestKindMetadata ManifestEntryKind = "metadata"
	// ManifestKindFilesBundle is the root.tar bundle of data-directory
	// files.
	ManifestKindFilesBundle ManifestEntryKind = "files-bundle"
	// ManifestKindControllerDump is the controller database dump.
	ManifestKindControllerDump ManifestEntryKind = "controller-dump"
	// ManifestKindModelDump is a model database dump.
	ManifestKindModelDump ManifestEntryKind = "model-dump"
)

// ManifestEntry records one archive component: its path inside the
// archive, what it is, and its uncompressed size and SHA-256 content
// hash, hex encoded.
type ManifestEntry struct {
	Path      string            `json:"path"`
	Kind      ManifestEntryKind `json:"kind"`
	Size      int64             `json:"size"`
	SHA256    string            `json:"sha256"`
	ModelUUID string            `json:"model-uuid,omitempty"`
}

// Manifest is the content index of a backup archive. Unlike
// metadata.json — the archive's provenance record — the manifest
// inventories the archive's components with their sizes and content
// hashes, so a reader can verify each component independently and
// localise corruption without trusting the archive as a whole.
//
// The manifest lives inside the archive: it inherits the archive's
// trust level and can never carry the outer archive checksum, which
// remains the operator-supplied trust anchor.
type Manifest struct {
	FormatVersion int64           `json:"format-version"`
	Files         []ManifestEntry `json:"files"`
}

// NewManifest builds the manifest for the given entries.
func NewManifest(entries []ManifestEntry) *Manifest {
	return &Manifest{
		FormatVersion: manifestFormatVersion,
		Files:         entries,
	}
}

// AsJSONBuffer returns a bytes.Buffer containing the JSON-ified
// manifest.
func (m *Manifest) AsJSONBuffer() (io.Reader, error) {
	var outfile bytes.Buffer
	if err := json.NewEncoder(&outfile).Encode(m); err != nil {
		return nil, errors.Capture(err)
	}
	return &outfile, nil
}

// NewManifestJSONReader extracts a manifest from its JSON file.
func NewManifestJSONReader(in io.Reader) (*Manifest, error) {
	data, err := io.ReadAll(in)
	if err != nil {
		return nil, errors.Capture(err)
	}
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, errors.Capture(err)
	}
	return &manifest, nil
}

// ClassifyManifestPath returns the manifest kind for a canonical
// archive path, and the model UUID for model dumps. Any path outside
// the known archive components is an error: the archive carries nothing
// the writer cannot account for.
func ClassifyManifestPath(archivePath string) (ManifestEntryKind, string, error) {
	canonical := NewCanonicalArchivePaths()
	switch archivePath {
	case canonical.MetadataFile:
		return ManifestKindMetadata, "", nil
	case canonical.FilesBundle:
		return ManifestKindFilesBundle, "", nil
	case path.Join(canonical.DBDumpDir, "controller.yaml"):
		return ManifestKindControllerDump, "", nil
	}
	const modelDumpPrefix = contentDir + "/" + dbDumpDir + "/models/"
	model := strings.TrimSuffix(
		strings.TrimPrefix(archivePath, modelDumpPrefix), ".yaml")
	if strings.HasPrefix(archivePath, modelDumpPrefix) &&
		strings.HasSuffix(archivePath, ".yaml") &&
		model != "" && !strings.Contains(model, "/") {
		return ManifestKindModelDump, model, nil
	}
	return "", "", errors.Errorf(
		"archive path %q is not a known component: %w",
		archivePath, coreerrors.NotValid)
}
