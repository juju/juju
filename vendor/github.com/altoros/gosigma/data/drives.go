// Copyright 2014 ALTOROS
// Licensed under the AGPLv3, see LICENSE file for details.

package data

import (
	"bytes"
	"encoding/json"
	"io"
)

// LibraryDrive contains properties of cloud drive instance specific for CloudSigma
// media library drives
type LibraryDrive struct {
	Arch      string `json:"arch,omitempty"`
	ImageType string `json:"image_type,omitempty"`
	OS        string `json:"os,omitempty"`
	Paid      bool   `json:"paid,omitempty"`
}

// Drive contains properties of cloud drive instance
type Drive struct {
	Resource
	LibraryDrive
	Affinities      []string          `json:"affinities,omitempty"`
	AllowMultimount bool              `json:"allow_multimount,omitempty"`
	Jobs            []Resource        `json:"jobs,omitempty"`
	Media           string            `json:"media,omitempty"`
	Meta            map[string]string `json:"meta,omitempty"`
	Name            string            `json:"name,omitempty"`
	Owner           *Resource         `json:"owner,omitempty"`
	Size            uint64            `json:"size,omitempty"`
	Status          string            `json:"status,omitempty"`
	StorageType     string            `json:"storage_type,omitempty"`
}

// Drives holds collection of Drive objects
type Drives struct {
	Meta    Meta    `json:"meta"`
	Objects []Drive `json:"objects"`
}

// ReadDrives reads and unmarshalls information about cloud drive instances from JSON stream
func ReadDrives(r io.Reader) ([]Drive, error) {
	var drives Drives
	if err := ReadJSON(r, &drives); err != nil {
		return nil, err
	}
	return drives.Objects, nil
}

// ReadDrive reads and unmarshalls information about single cloud drive instance from JSON stream
func ReadDrive(r io.Reader) (*Drive, error) {
	var drive Drive
	if err := ReadJSON(r, &drive); err != nil {
		return nil, err
	}
	return &drive, nil
}

// WriteDrive marshals single drive object to JSON stream
func WriteDrive(obj *Drive) (io.Reader, error) {
	bb, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(bb), nil
}
