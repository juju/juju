// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"bytes"
	"io"
	"io/ioutil"

	"github.com/juju/errors"

	"github.com/juju/juju/state/storage"
)

type MapStorage struct {
	Map map[string][]byte
}

var _ storage.Storage = (*MapStorage)(nil)

func (s *MapStorage) Get(path string) (r io.ReadCloser, length int64, err error) {
	data, ok := s.Map[path]
	if !ok {
		return nil, -1, errors.NotFoundf("%s", path)
	}
	return ioutil.NopCloser(bytes.NewReader(data)), int64(len(data)), nil
}

func (s *MapStorage) Put(path string, r io.Reader, length int64) error {
	if s.Map == nil {
		s.Map = make(map[string][]byte)
	}
	buf := make([]byte, int(length))
	_, err := io.ReadFull(r, buf)
	if err != nil {
		return err
	}
	s.Map[path] = buf
	return nil
}

func (s *MapStorage) Remove(path string) error {
	if _, ok := s.Map[path]; !ok {
		return errors.NotFoundf("%s", path)
	}
	delete(s.Map, path)
	return nil
}
