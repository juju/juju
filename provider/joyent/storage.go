// Copyright 2013 Joyent Inc.
// Licensed under the AGPLv3, see LICENCE file for details.

package joyent

import (
	"io"
	"sync"
	"fmt"

	"launchpad.net/juju-core/environs/storage"
	"launchpad.net/juju-core/utils"

	"launchpad.net/gojoyent/manta"
)

type environStorage struct {
	sync.Mutex
	ecfg 			*environConfig
	madeContainer 	bool
	containerName 	string
	manta			*manta.Client
}

var _ storage.Storage = (*environStorage)(nil)

func newStorage(ecfg *environConfig) (storage.Storage, error) {
	return &environStorage{ecfg}, nil
}

// makeContainer makes the environment's control container, the
// place where bootstrap information and deployed charms
// are stored. To avoid two round trips on every PUT operation,
// we do this only once for each environ.
func (s *environStorage) makeContainer(containerName string) error {
	s.Lock()
	defer s.Unlock()
	if s.madeContainer {
		return nil
	}
	// try to make the container
	err := s.manta.PutDirectory(containerName)
	if err == nil {
		s.madeContainer = true
	}
	return err
}

func (s *environStorage) List(prefix string) ([]string, error) {
	// use empty opts, i.e. default values
	// -- might be added in the provider config?
	contents, err := s.manta.ListDirectory(prefix, manta.ListDirectoryOpts{})
	if err != nil {
		return nil, err
	}
	var names []string
	for _, item := range contents {
		names = append(names, item.Name)
	}
	return names, nil
}

func (s *environStorage) URL(name string) (string, error) {
	return "", errNotImplemented
}

func (s *environStorage) Get(name string) (io.ReadCloser, error) {
	r, err := s.manta.GetObject(s.containerName, name)
	if err != nil {
		return nil, err
	}
	return r, nil
}

func (s *environStorage) Put(name string, r io.Reader, length int64) error {
	if err := s.makeContainer(s.containerName); err != nil {
		return fmt.Errorf("cannot make Manta control container: %v", err)
	}
	obj := r.Read()
	err := s.manta.PutObject(s.containerName, name, obj)
	if err != nil {
		return fmt.Errorf("cannot write file %q to control container %q: %v", name, s.containerName, err)
	}
	return nil
}

func (s *environStorage) Remove(name string) error {
	err := s.manta.DeleteObject(s.containerName, name)
	if err != nil {
		return err
	}
	return nil
}

func (s *environStorage) RemoveAll() error {
	names, err := storage.List(s, "")
	if err != nil {
		return err
	}
	// Remove all the objects in parallel so that we incur less round-trips.
	// If we're in danger of having hundreds of objects,
	// we'll want to change this to limit the number
	// of concurrent operations.
	var wg sync.WaitGroup
	wg.Add(len(names))
	errc := make(chan error, len(names))
	for _, name := range names {
		name := name
		go func() {
			defer wg.Done()
			if err := s.Remove(name); err != nil {
				errc <- err
			}
		}()
	}
	wg.Wait()
	select {
	case err := <-errc:
		return fmt.Errorf("cannot delete all provider state: %v", err)
	default:
	}

	s.Lock()
	defer s.Unlock()
	// Even DeleteContainer fails, it won't harm if we try again - the
	// operation might have succeeded even if we get an error.
	s.madeContainer = false
	if err = s.manta.DeleteDirectory(s.containerName); err != nil {
		return err
	}
	return nil
}

func (s *environStorage) DefaultConsistencyStrategy() utils.AttemptStrategy {
	return utils.AttemptStrategy{}
}

func (s *environStorage) ShouldRetry(err error) bool {
	return false
}
