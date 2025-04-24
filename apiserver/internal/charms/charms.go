// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms

import (
	"archive/zip"
	"context"
	"io"
	"os"
	"path"
	"path/filepath"

	"github.com/juju/errors"

	"github.com/juju/juju/core/arch"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/domain/application/architecture"
	"github.com/juju/juju/domain/application/charm"
	objectstoreerrors "github.com/juju/juju/domain/objectstore/errors"
	internalcharm "github.com/juju/juju/internal/charm"
	"github.com/juju/juju/rpc/params"
)

// ReadObjectStore represents an object store that can only be read from.
type ReadObjectStore interface {
	// Get returns an io.ReadCloser for data at path, namespaced to the
	// model.
	Get(context.Context, string) (io.ReadCloser, int64, error)
}

// ReadCharmFromStorage fetches the charm at the specified path from the store
// and copies it to a temp directory in dataDir.
func ReadCharmFromStorage(ctx context.Context, objectStore ReadObjectStore, dataDir, storagePath string) (string, error) {
	// Use the storage to retrieve and save the charm archive.
	reader, _, err := objectStore.Get(ctx, storagePath)
	if err != nil {
		if errors.Is(err, objectstoreerrors.ErrNotFound) {
			return "", errors.NewNotFound(err, "charm not found in model storage")
		}
		return "", errors.Annotate(err, "cannot get charm from model storage")
	}
	defer reader.Close()

	// Ensure the working directory exists.
	tmpDir := filepath.Join(dataDir, "charm-get-tmp")
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		return "", errors.Annotate(err, "cannot create charms tmp directory")
	}

	charmFile, err := os.CreateTemp(tmpDir, "charm")
	if err != nil {
		return "", errors.Annotate(err, "cannot create charm archive file")
	}
	defer charmFile.Close()

	if _, err = io.Copy(charmFile, reader); err != nil {
		return "", errors.Annotate(err, "error processing charm archive download")
	}
	return charmFile.Name(), nil
}

// CharmArchiveEntry retrieves the specified entry from the zip archive.
func CharmArchiveEntry(charmPath, entryPath string) ([]byte, error) {
	// TODO(fwereade) 2014-01-27 bug #1285685
	// This doesn't handle symlinks helpfully, and should be talking in
	// terms of bundles rather than zip readers; but this demands thought
	// and design and is not amenable to a quick fix.
	zipReader, err := zip.OpenReader(charmPath)
	if err != nil {
		return nil, errors.Annotatef(err, "unable to read charm")
	}
	defer zipReader.Close()
	for _, file := range zipReader.File {
		if path.Clean(file.Name) != entryPath {
			continue
		}
		fileInfo := file.FileInfo()
		if fileInfo.IsDir() {
			return nil, &params.Error{
				Message: "directory listing not allowed",
				Code:    params.CodeForbidden,
			}
		}
		contents, err := file.Open()
		if err != nil {
			return nil, errors.Annotatef(err, "unable to read file %q", entryPath)
		}
		defer contents.Close()
		return io.ReadAll(contents)
	}
	return nil, errors.NotFoundf("charm file")
}

// ValidateCharmOrigin validates the Source of the charm origin for args received
// in a facade. This may evolve over time to include more pieces.
func ValidateCharmOrigin(o *params.CharmOrigin) error {
	switch {
	case o == nil:
		return errors.BadRequestf("charm origin source required")
	case corecharm.CharmHub.Matches(o.Source):
		// If either the charm origin ID or Hash is set before a charm is
		// downloaded, charm download will fail for charms with a forced series.
		// The logic (refreshConfig) in sending the correct request to charmhub
		// will break.
		if (o.ID != "" && o.Hash == "") ||
			(o.ID == "" && o.Hash != "") {
			return errors.BadRequestf("programming error, both CharmOrigin ID and Hash must be set or neither. See CharmHubRepository GetDownloadURL.")
		}
	case corecharm.Local.Matches(o.Source):
	default:
		return errors.BadRequestf("%q not a valid charm origin source", o.Source)
	}
	return nil
}

// CharmLocatorFromURL returns a CharmLocator using the charm name, revision
// and source (which is extracted from the schema) of the provided URL.
func CharmLocatorFromURL(url string) (charm.CharmLocator, error) {
	u, err := internalcharm.ParseURL(url)
	if err != nil {
		return charm.CharmLocator{}, errors.Trace(err)
	}
	source, err := charm.ParseCharmSchema(internalcharm.Schema(u.Schema))
	if err != nil {
		return charm.CharmLocator{}, errors.Trace(err)
	}
	locator := charm.CharmLocator{
		Name:     u.Name,
		Revision: u.Revision,
		Source:   source,
	}
	if u.Architecture != "" {
		arch, err := decodeArchitecture(u.Architecture)
		if err != nil {
			return charm.CharmLocator{}, errors.Trace(err)
		}
		locator.Architecture = arch
	}
	return locator, nil
}

func decodeArchitecture(a arch.Arch) (architecture.Architecture, error) {
	switch a {
	case arch.AMD64:
		return architecture.AMD64, nil
	case arch.ARM64:
		return architecture.ARM64, nil
	case arch.PPC64EL:
		return architecture.PPC64EL, nil
	case arch.S390X:
		return architecture.S390X, nil
	case arch.RISCV64:
		return architecture.RISCV64, nil
	default:
		return -1, errors.BadRequestf("unsupported architecture %q", a)
	}
}

// CharmURLFromLocator returns the charm URL for the current charm.
func CharmURLFromLocator(name string, locator charm.CharmLocator) (string, error) {
	schema, err := convertSource(locator.Source)
	if err != nil {
		return "", errors.Trace(err)
	}

	architecture, err := convertApplication(locator.Architecture)
	if err != nil {
		return "", errors.Trace(err)
	}

	url := internalcharm.URL{
		Schema:       schema,
		Name:         name,
		Revision:     locator.Revision,
		Architecture: architecture,
	}
	return url.String(), nil
}
