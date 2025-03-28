// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"path"
	"strings"
	"time"

	"github.com/juju/collections/set"

	"github.com/juju/juju/core/backups"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/internal/errors"
)

// File represents a file during testing.
type File struct {
	// Name is the path to which the file will be identified in the archive.
	Name string
	// Content is the data that will be written to the archive for the file.
	Content string
	// IsDir determines if the file is a regular file or a directory.
	IsDir bool
}

// AddToArchive adds the file to the tar archive.
func (f *File) AddToArchive(archive *tar.Writer) error {
	hdr := &tar.Header{
		Name: f.Name,
	}
	if f.IsDir {
		hdr.Typeflag = tar.TypeDir
		hdr.Mode = 0777
	} else {
		hdr.Size = int64(len(f.Content))
		hdr.Mode = 0666
	}

	if err := archive.WriteHeader(hdr); err != nil {
		return errors.Capture(err)
	}

	if !f.IsDir {
		if _, err := archive.Write([]byte(f.Content)); err != nil {
			return errors.Capture(err)
		}
	}

	return nil
}

// NewArchive returns a new archive file containing the files.
func NewArchive(meta *backups.Metadata, files, dump []File) (*bytes.Buffer, error) {
	topFiles, err := internalTopFiles(files, dump)
	if err != nil {
		return nil, errors.Capture(err)
	}

	if meta != nil {
		metaFile, err := meta.AsJSONBuffer()
		if err != nil {
			return nil, errors.Capture(err)
		}
		topFiles = append(topFiles,
			File{
				Name:    "juju-backup/metadata.json",
				Content: metaFile.(*bytes.Buffer).String(),
			},
		)
	}
	return internalCompress(topFiles)
}

func internalTopFiles(files, dump []File) ([]File, error) {
	dirs := set.NewStrings()
	var sysFiles []File
	for _, file := range files {
		var parent string
		for _, p := range strings.Split(path.Dir(file.Name), "/") {
			if parent == "" {
				parent = p
			} else {
				parent = path.Join(parent, p)
			}
			if !dirs.Contains(parent) {
				sysFiles = append(sysFiles, File{
					Name:  parent,
					IsDir: true,
				})
				dirs.Add(parent)
			}
		}
		if file.IsDir {
			if !dirs.Contains(file.Name) {
				sysFiles = append(sysFiles, file)
				dirs.Add(file.Name)
			}
		} else {
			sysFiles = append(sysFiles, file)
		}
	}

	var rootFile bytes.Buffer
	if err := writeToTar(&rootFile, sysFiles); err != nil {
		return nil, errors.Capture(err)
	}

	topFiles := []File{{
		Name:  "juju-backup",
		IsDir: true,
	}}

	topFiles = append(topFiles, File{
		Name:  "juju-backup/dump",
		IsDir: true,
	})
	for _, dumpFile := range dump {
		topFiles = append(topFiles, File{
			Name:    "juju-backup/dump/" + dumpFile.Name,
			Content: dumpFile.Content,
			IsDir:   dumpFile.IsDir,
		})
	}

	topFiles = append(topFiles,
		File{
			Name:    "juju-backup/root.tar",
			Content: rootFile.String(),
		},
	)
	return topFiles, nil
}

// NewArchiveV0 returns a new archive file containing the files, in v0 format.
func NewArchiveV0(meta *backups.Metadata, files, dump []File) (*bytes.Buffer, error) {
	topFiles, err := internalTopFiles(files, dump)
	if err != nil {
		return nil, errors.Capture(err)
	}
	if meta != nil {
		metaFile, err := asJSONBufferV0(meta)
		if err != nil {
			return nil, errors.Capture(err)
		}
		topFiles = append(topFiles,
			File{
				Name:    "juju-backup/metadata.json",
				Content: metaFile.(*bytes.Buffer).String(),
			},
		)
	}
	return internalCompress(topFiles)
}

func internalCompress(topFiles []File) (*bytes.Buffer, error) {
	var arFile bytes.Buffer
	compressed := gzip.NewWriter(&arFile)
	defer compressed.Close()
	if err := writeToTar(compressed, topFiles); err != nil {
		return nil, errors.Capture(err)
	}
	return &arFile, nil
}

func asJSONBufferV0(m *backups.Metadata) (io.Reader, error) {
	var outfile bytes.Buffer
	if err := json.NewEncoder(&outfile).Encode(flatV0(m)); err != nil {
		return nil, errors.Capture(err)
	}
	return &outfile, nil
}

type flatMetadataV0 struct {
	ID string

	// file storage

	Checksum       string
	ChecksumFormat string
	Size           int64
	Stored         time.Time

	// backup

	Started     time.Time
	Finished    time.Time
	Notes       string
	Environment string
	Machine     string
	Hostname    string
	Version     semversion.Number
	Base        string
}

func flatV0(m *backups.Metadata) flatMetadataV0 {
	flat := flatMetadataV0{
		ID:             m.ID(),
		Checksum:       m.Checksum(),
		ChecksumFormat: m.ChecksumFormat(),
		Size:           m.Size(),
		Started:        m.Started,
		Notes:          m.Notes,
		Environment:    m.Origin.Model,
		Machine:        m.Origin.Machine,
		Hostname:       m.Origin.Hostname,
		Version:        m.Origin.Version,
		Base:           m.Origin.Base,
	}
	stored := m.Stored()
	if stored != nil {
		flat.Stored = *stored
	}

	if m.Finished != nil {
		flat.Finished = *m.Finished
	}
	return flat
}

// NewArchiveBasic returns a new archive file with a few files provided.
func NewArchiveBasic(meta *backups.Metadata) (*bytes.Buffer, error) {
	files := []File{
		{
			Name:    "var/lib/juju/tools/1.21-alpha2.1-trusty-amd64/jujud",
			Content: "<some binary data goes here>",
		},
		{
			Name:    "var/lib/juju/system-identity",
			Content: "<an ssh key goes here>",
		},
	}
	dump := []File{
		{
			Name:    "juju/machines.bson",
			Content: "<BSON data goes here>",
		},
		{
			Name:    "oplog.bson",
			Content: "<BSON data goes here>",
		},
	}

	arFile, err := NewArchive(meta, files, dump)
	if err != nil {
		return nil, errors.Capture(err)
	}
	return arFile, nil
}

func writeToTar(archive io.Writer, files []File) error {
	tarw := tar.NewWriter(archive)
	defer tarw.Close()

	for _, file := range files {
		if err := file.AddToArchive(tarw); err != nil {
			return errors.Capture(err)
		}
	}
	return nil
}
