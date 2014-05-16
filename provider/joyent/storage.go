// Copyright 2013 Joyent Inc.
// Licensed under the AGPLv3, see LICENCE file for details.

package joyent

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/juju/errors"
	"launchpad.net/juju-core/environs/storage"
	"launchpad.net/juju-core/utils"

	"github.com/joyent/gocommon/client"
	je "github.com/joyent/gocommon/errors"
	"github.com/joyent/gomanta/manta"
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

func newStorage(cfg *environConfig, name string) (storage.Storage, error) {
	creds, err := credentials(cfg)
	if err != nil {
		return nil, err
	}
	client := client.NewClient(cfg.mantaUrl(), "", creds, &logger)

	if name == "" {
		name = cfg.controlDir()
	}

	return &JoyentStorage{
		ecfg:          cfg,
		containerName: name,
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

// createContainer makes the environment's control container, the
// place where bootstrap information and deployed charms
// are stored. To avoid two round trips on every PUT operation,
// we do this only once for each environ.
func (s *JoyentStorage) createContainer() error {
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

// DeleteContainer deletes the named container from the storage account.
func (s *JoyentStorage) DeleteContainer(containerName string) error {
	err := s.manta.DeleteDirectory(containerName)
	if err == nil && strings.EqualFold(s.containerName, containerName) {
		s.madeContainer = false
	}
	if je.IsResourceNotFound(err) {
		return errors.NewNotFound(err, fmt.Sprintf("cannot delete %s, not found", containerName))
	}
	return err
}

func (s *JoyentStorage) List(prefix string) ([]string, error) {
	content, err := list(s, s.containerName)
	if err != nil {
		return nil, err
	}

	var names []string
	for _, item := range content {
		name := strings.TrimPrefix(item, s.containerName+"/")
		if prefix != "" {
			if strings.HasPrefix(name, prefix) {
				names = append(names, name)
			}
		} else {
			names = append(names, name)
		}
	}
	return names, nil
}

func list(s *JoyentStorage, path string) ([]string, error) {
	// TODO - we don't want to create the container here, but instead handle
	// any 404 and return as if no files exist.
	if err := s.createContainer(); err != nil {
		return nil, fmt.Errorf("cannot make Manta control container: %v", err)
	}
	// use empty opts, i.e. default values
	// -- might be added in the provider config?
	contents, err := s.manta.ListDirectory(path, manta.ListDirectoryOpts{})
	if err != nil {
		return nil, err
	}
	var names []string
	for _, item := range contents {
		if strings.EqualFold(item.Type, "directory") {
			items, err := list(s, path+"/"+item.Name)
			if err != nil {
				return nil, err
			}
			names = append(names, items...)
		} else {
			names = append(names, path+"/"+item.Name)
		}
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
		return nil, errors.NewNotFound(err, fmt.Sprintf("cannot find %s", name))
	}
	r := byteCloser{bytes.NewReader(b)}
	return r, nil
}

func (s *JoyentStorage) Put(name string, r io.Reader, length int64) error {
	if err := s.createContainer(); err != nil {
		return fmt.Errorf("cannot make Manta control container: %v", err)
	}
	if strings.Contains(name, "/") {
		var parents []string
		dirs := strings.Split(name, "/")
		for i, _ := range dirs {
			if i < (len(dirs) - 1) {
				parents = append(parents, strings.Join(dirs[:(i+1)], "/"))
			}
		}
		for _, dir := range parents {
			err := s.manta.PutDirectory(path.Join(s.containerName, dir))
			if err != nil {
				return fmt.Errorf("cannot create parent directory %q in control container %q: %v", dir, s.containerName, err)
			}
		}
	}
	object, err := ioutil.ReadAll(r)
	if err != nil {
		return fmt.Errorf("failed to read object %q: %v", name, err)
	}
	err = s.manta.PutObject(s.containerName, name, object)
	if err != nil {
		return fmt.Errorf("cannot write file %q to control container %q: %v", name, s.containerName, err)
	}
	return nil
}

func (s *JoyentStorage) Remove(name string) error {
	err := s.manta.DeleteObject(s.containerName, name)
	if err != nil {
		if je.IsResourceNotFound(err) {
			// gojoyent returns an error if file doesn't exist
			// just log a warning
			logger.Warningf("cannot delete %s from %s, already deleted", name, s.containerName)
		} else {
			return err
		}
	}

	if strings.Contains(name, "/") {
		var parents []string
		dirs := strings.Split(name, "/")
		for i := (len(dirs) - 1); i >= 0; i-- {
			if i < (len(dirs) - 1) {
				parents = append(parents, strings.Join(dirs[:(i+1)], "/"))
			}
		}

		for _, dir := range parents {
			err := s.manta.DeleteDirectory(path.Join(s.containerName, dir))
			if err != nil {
				if je.IsBadRequest(err) {
					// check if delete request returned a bad request error, i.e. directory is not empty
					// just log a warning
					logger.Warningf("cannot delete %s, not empty", dir)
				} else if je.IsResourceNotFound(err) {
					// check if delete request returned a resource not found error, i.e. directory was already deleted
					// just log a warning
					logger.Warningf("cannot delete %s, already deleted", dir)
				} else {
					return fmt.Errorf("cannot delete parent directory %q in control container %q: %v", dir, s.containerName, err)
				}
			}
		}
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
