// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backup

// This is a separate package due to import cycles with state.

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const StorageRoot = "/backup"

// BackupStorage is the API for the storage used by backups.
type BackupStorage interface {
	Add(info *BackupInfo, archive io.Reader) error
	Info(name string) (*BackupInfo, error)
	Archive(name string) (io.ReadCloser, error)
	URL(name string) (string, error)
}

type DocStorage interface {
	AddDoc(id string, doc interface{}) error
	Doc(id string, doc interface{}) error
}

type FileStorage interface {
	AddFile(id string, file io.Reader, size int64) error
	File(id string) (io.ReadCloser, error)
	URL(id string) (string, error)
}

//---------------------------
// backup storage implementation

// NewBackupStorage returns a new backup storage based on the state.
func NewBackupStorage(docs DocStorage, files FileStorage) BackupStorage {
	bstor := backupStorage{
		docs:  docs,
		files: files,
	}
	return &bstor
}

type backupStorage struct {
	docs  DocStorage
	files FileStorage
}

func (s *backupStorage) Info(name string) (*BackupInfo, error) {
	var info BackupInfo
	err := s.docs.Doc(name, &info)
	if err != nil {
		return nil, err
	}
	return &info, nil
}

func (s *backupStorage) Archive(name string) (io.ReadCloser, error) {
	return s.files.File(name)
}

func (s *backupStorage) URL(name string) (string, error) {
	return s.files.URL(name)
}

func (s *backupStorage) Add(info *BackupInfo, archive io.Reader) error {
	err := s.docs.AddDoc(info.Name, info)
	if err != nil {
		return err
	}

	if archive != nil {
		err := s.files.AddFile(info.Name, archive, info.Size)
		if err != nil {
			return err
		}
	}

	return nil
}

//---------------------------
// file-based storage

type fileStorage struct {
	dirname string
	info    map[string]*BackupInfo
}

func NewFileStorage(dirname string) (*fileStorage, error) {
	stor := fileStorage{
		dirname: dirname,
		info:    make(map[string]*BackupInfo),
	}
	if err := os.MkdirAll(dirname, 0777); err != nil {
		return nil, err
	}
	return &stor, nil
}

func (s *fileStorage) Add(info *BackupInfo, archive io.Reader) error {
	s.info[info.Name] = info

	filename := filepath.Join(s.dirname, info.Name)
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = io.Copy(file, archive)
	if err != nil {
		return err
	}
	return nil
}

func (s *fileStorage) Info(name string) (*BackupInfo, error) {
	info, ok := s.info[name]
	if !ok {
		return nil, fmt.Errorf("not found: %q", name)
	}
	return info, nil
}

func (s *fileStorage) Archive(name string) (io.ReadCloser, error) {
	filename := filepath.Join(s.dirname, name)
	archive, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	return archive, nil
}

func (s *fileStorage) URL(name string) (string, error) {
	return "", fmt.Errorf("URL not supported")
}
