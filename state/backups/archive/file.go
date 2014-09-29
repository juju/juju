// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package archive

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"io/ioutil"

	"github.com/juju/errors"

	"github.com/juju/juju/state/backups/metadata"
)

// Extract pulls a file out of the archive file.
func Extract(arFile io.Reader, filename string) (io.Reader, error) {
	compressed, err := gzip.NewReader(arFile)
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer compressed.Close()
	tarball := tar.NewReader(compressed)

	for {
		hdr, err := tarball.Next()
		if err != nil {
			if err == io.EOF {
				// We reached the end of the archive.
				break
			}
			return nil, errors.Trace(err)
		}
		if hdr.Name == filename {
			var found bytes.Buffer
			_, err := found.ReadFrom(tarball)
			if err != nil {
				return nil, errors.Trace(err)
			}
			return &found, nil
		}
	}
	return nil, errors.NotFoundf(filename)
}

// GetMetadata pulls the metadata from the file inside the archive.  If
// the archive does not have a metadata file, errors.NotFound is
// returned.
func GetMetadata(arFile io.Reader) (*metadata.Metadata, error) {
	metaFile, err := Extract(arFile, "metadata.json")
	if err != nil {
		return nil, errors.Trace(err)
	}

	data, err := ioutil.ReadAll(metaFile)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var meta metadata.Metadata
	err = json.Unmarshal(data, &meta)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &meta, nil
}
