// Copyright 2013 Joyent Inc.
// Licensed under the AGPLv3, see LICENCE file for details.

package joyent

import (
	"io"

	"launchpad.net/juju-core/environs/storage"
	"launchpad.net/juju-core/utils"
	"fmt"
)

type environStorage struct {
	ecfg *environConfig
}

var _ storage.Storage = (*environStorage)(nil)

func newStorage(ecfg *environConfig) (storage.Storage, error) {
	return &environStorage{
		ecfg:			ecfg,
		containerName: 	ecfg.controlDir(),
		manta:        	manta.New(nil)}, nil
}

func (s *environStorage) List(prefix string) ([]string, error) {
	return nil, errNotImplemented
}

func (s *environStorage) URL(name string) (string, error) {
	return "", errNotImplemented
}

func (s *environStorage) Get(name string) (io.ReadCloser, error) {
	return nil, errNotImplemented
}

func (s *environStorage) Put(name string, r io.Reader, length int64) error {
	if err := s.makeContainer(s.containerName); err != nil {
		return fmt.Errorf("cannot make Manta control container: %v", err)
	}
	//obj := r.Read()
	err := s.manta.PutObject(s.containerName, name, r)
	if err != nil {
		return fmt.Errorf("cannot write file %q to control container %q: %v", name, s.containerName, err)
	}
	return nil
}

func (s *environStorage) Remove(name string) error {
	return errNotImplemented
}

func (s *environStorage) RemoveAll() error {
	return errNotImplemented
}

func (s *environStorage) DefaultConsistencyStrategy() utils.AttemptStrategy {
	return utils.AttemptStrategy{}
}

func (s *environStorage) ShouldRetry(err error) bool {
	return false
}
