// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package initsystems

import (
	"encoding/json"
	"os"
	"path"

	"github.com/juju/errors"
)

const (
	// TODO(ericsnow) Is 0600 more appropriate?
	defaultFilePermissions = 0644
	scriptPermissions      = 0755

	metadataFilename = "metadata.json"
)

// FileData holds the information about a "regular" file in a directory.
type FileData struct {
	// FileName is the base name of the file.
	FileName string

	// Mode contains the file's permissions.
	Mode os.FileMode

	// Data holds the data that should be written to disk.
	Data []byte
}

// newScriptData creates a new FileData based on the provided data and
// returns it.
func newScriptData(name, content string) FileData {
	return FileData{
		FileName: name,
		Mode:     scriptPermissions,
		Data:     []byte(content),
	}
}

// readFileData reads the file and creates a FileData from it.
func readFileData(dirName string, info os.FileInfo, readFile func(string) ([]byte, error)) (FileData, error) {
	var file FileData

	data, err := readFile(path.Join(dirName, info.Name()))
	if err != nil {
		return file, errors.Trace(err)
	}

	file = FileData{
		FileName: info.Name(),
		Mode:     info.Mode() & os.ModePerm,
		Data:     data,
	}
	return file, nil
}

// Write writes the file out to disk (or whatever is facilitated) using
// the writer returned by the provided createFile function. The file is
// written to the given directory, which should be an absolute, generic
// slash-delimited path.
func (fd FileData) Write(dirName string, createFile func(string, os.FileMode, []byte) error) error {
	filename := path.Join(dirName, fd.FileName)
	perm := fd.Mode & os.ModePerm
	if perm == 0 {
		perm = defaultFilePermissions
	}

	err := createFile(filename, perm, fd.Data)
	return errors.Trace(err)
}

type metadata struct {
	Conf

	// Name is the name of the service to which the conf file corresponds.
	Name string `json:"name"`

	// ConfName is the conf file name specified by the init system.
	ConfName string `json:"confname"`
}

func readMetadata(dirName string, readFile func(string) ([]byte, error)) (metadata, error) {
	var meta metadata

	data, err := readFile(path.Join(dirName, metadataFilename))
	if err != nil {
		return meta, errors.Trace(err)
	}

	err = json.Unmarshal(data, &meta)
	return meta, errors.Trace(err)
}

func (m *metadata) fileData() (FileData, error) {
	var file FileData

	data, err := json.MarshalIndent(m, "", " ")
	if err != nil {
		return file, errors.Trace(err)
	}

	file = FileData{
		FileName: metadataFilename,
		Data:     data,
	}
	return file, nil
}
