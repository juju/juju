// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"io"

	"github.com/juju/juju/environs/storage"
	"github.com/juju/utils"
)

type environStorage struct {
	ecfg *environConfig
}

var _ storage.Storage = (*environStorage)(nil)

func newStorage(ecfg *environConfig) (storage.Storage, error) {
	return &environStorage{ecfg}, nil
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
	return errNotImplemented
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
