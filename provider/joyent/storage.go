// Copyright 2013 Joyent Inc.
// Licensed under the AGPLv3, see LICENCE file for details.

package joyent

import (
	"bytes"
	"fmt"
	"io"
	"sync"
	"time"

	"launchpad.net/juju-core/environs/storage"
	"launchpad.net/juju-core/utils"

	"launchpad.net/gojoyent/client"
	"launchpad.net/gojoyent/jpc"
	"launchpad.net/gojoyent/manta"
)

type JoyentStorage struct {
	sync.Mutex
	ecfg          *environConfig
	madeContainer bool
	containerName string
	manta         *manta.Client
}

type byteCloser struct {
	io.Reader
}

func (byteCloser) Close() error {
	return nil
}

var _ storage.Storage = (*JoyentStorage)(nil)

func NewStorage(env *JoyentEnviron) storage.Storage {
	if stor, err := newStorage(env.ecfg); err == nil {
		return stor
	}
	return nil
}

func getCredentials(ecfg *environConfig) *jpc.Credentials {
	auth := jpc.Auth{User: ecfg.mantaUser(), KeyFile: ecfg.keyFile(), Algorithm: ecfg.algorithm()}

	return &jpc.Credentials{
		UserAuthentication: auth,
		MantaKeyId:         ecfg.mantaKeyId(),
		MantaEndpoint:      jpc.Endpoint{URL: ecfg.mantaUrl()},
	}
}

func newStorage(ecfg *environConfig) (storage.Storage, error) {
	client := client.NewClient(ecfg.mantaUrl(), "", getCredentials(ecfg), &logger)

	return &JoyentStorage{
		ecfg:          ecfg,
		containerName: ecfg.controlDir(),
		manta:         manta.New(client)}, nil
}

func (s *JoyentStorage) GetContainerName() string {
	return s.containerName
}

func (s *JoyentStorage) GetMantaUrl() string {
	return s.ecfg.mantaUrl()
}

func (s *JoyentStorage) GetMantaUser() string {
	return s.ecfg.mantaUser()
}

// CreateContainer makes the environment's control container, the
// place where bootstrap information and deployed charms
// are stored. To avoid two round trips on every PUT operation,
// we do this only once for each environ.
func (s *JoyentStorage) CreateContainer() error {
	s.Lock()
	defer s.Unlock()
	if s.madeContainer {
		return nil
	}
	// try to make the container
	err := s.manta.PutDirectory(s.containerName)
	if err == nil {
		s.madeContainer = true
	}
	return err
}

// deleteContainer deletes the named container from the storage account.
func (s *JoyentStorage) DeleteContainer(containerName string) error {
	err := s.manta.DeleteDirectory(containerName)
	if err == nil {
		s.madeContainer = false
	}
	return err
}

func (s *JoyentStorage) List(prefix string) ([]string, error) {
	// use empty opts, i.e. default values
	// -- might be added in the provider config?
	contents, err := s.manta.ListDirectory(s.containerName, manta.ListDirectoryOpts{})
	if err != nil {
		return nil, err
	}
	var names []string
	for _, item := range contents {
		names = append(names, item.Name)
	}
	return names, nil
}

//return something that a random wget can retrieve the object at, without any credentials
func (s *JoyentStorage) URL(name string) (string, error) {
	path := fmt.Sprintf("/%s/stor/%s/%s", s.ecfg.mantaUser(), s.containerName, name)
	return s.manta.SignURL(path, time.Now().AddDate(10, 0, 0))
}

func (s *JoyentStorage) Get(name string) (io.ReadCloser, error) {
	b, err := s.manta.GetObject(s.containerName, name)
	if err != nil {
		return nil, err
	}
	r := byteCloser{bytes.NewReader(b)}
	return r, nil
}

func (s *JoyentStorage) Put(name string, r io.Reader, length int64) error {
	if err := s.CreateContainer(); err != nil {
		return fmt.Errorf("cannot make Manta control container: %v", err)
	}
	err := s.manta.PutObject(s.containerName, name, r)
	if err != nil {
		return fmt.Errorf("cannot write file %q to control container %q: %v", name, s.containerName, err)
	}
	return nil
}

func (s *JoyentStorage) Remove(name string) error {
	err := s.manta.DeleteObject(s.containerName, name)
	if err != nil {
		return err
	}
	return nil
}

func (s *JoyentStorage) RemoveAll() error {
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

func (s *JoyentStorage) DefaultConsistencyStrategy() utils.AttemptStrategy {
	return utils.AttemptStrategy{}
}

func (s *JoyentStorage) ShouldRetry(err error) bool {
	return false
}
