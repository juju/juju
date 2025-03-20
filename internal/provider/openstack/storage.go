// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack

import (
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/go-goose/goose/v5/swift"
	jujuerrors "github.com/juju/errors"
	"github.com/juju/utils/v4"

	"github.com/juju/juju/environs/storage"
)

// openstackstorage implements storage.Storage on an OpenStack container.
type openstackstorage struct {
	sync.Mutex
	madeContainer bool
	containerName string
	containerACL  swift.ACL
	swift         *swift.Client
}

// makeContainer makes the environment's control container, the
// place where bootstrap information and deployed charms
// are stored. To avoid two round trips on every PUT operation,
// we do this only once for each environ.
func (s *openstackstorage) makeContainer(containerName string, containerACL swift.ACL) error {
	s.Lock()
	defer s.Unlock()
	if s.madeContainer {
		return nil
	}
	// try to make the container - CreateContainer will succeed if the container already exists.
	err := s.swift.CreateContainer(containerName, containerACL)
	if err == nil {
		s.madeContainer = true
	}
	return err
}

func (s *openstackstorage) Put(file string, r io.Reader, length int64) error {
	if err := s.makeContainer(s.containerName, s.containerACL); err != nil {
		return fmt.Errorf("cannot make Swift control container: %v", err)
	}
	err := s.swift.PutReader(s.containerName, file, r, length)
	if err != nil {
		return fmt.Errorf("cannot write file %q to control container %q: %v", file, s.containerName, err)
	}
	return nil
}

func (s *openstackstorage) Get(file string) (io.ReadCloser, error) {
	r, _, err := s.swift.GetReader(s.containerName, file)
	if err, _ := maybeNotFound(err); err != nil {
		return nil, err
	}
	return r, nil
}

func (s *openstackstorage) URL(name string) (string, error) {
	// 10 years should be good enough.
	// TODO(perrito666) 2016-05-02 lp:1558657
	expires := time.Now().AddDate(10, 0, 0)
	return s.swift.SignedURL(s.containerName, name, expires)
}

var storageAttempt = utils.AttemptStrategy{
	// It seems Nova needs more time than EC2.
	Total: 10 * time.Second,
	Delay: 200 * time.Millisecond,
}

// DefaultConsistencyStrategy is specified in the StorageReader interface.
func (s *openstackstorage) DefaultConsistencyStrategy() utils.AttemptStrategy {
	return storageAttempt
}

// ShouldRetry is specified in the StorageReader interface.
func (s *openstackstorage) ShouldRetry(err error) bool {
	_, retry := maybeNotFound(err)
	return retry
}

func (s *openstackstorage) Remove(file string) error {
	// Prevent a blank containerName combo being used, which
	// can result in erroneously deleting the Swift account itself.
	if s.containerName == "" {
		return jujuerrors.Errorf("cannot remove %q: swift container name is empty", file)
	}
	err := s.swift.DeleteObject(s.containerName, file)
	// If we can't delete the object because the bucket doesn't
	// exist, then we don't care.
	if err, ok := maybeNotFound(err); !ok {
		return err
	}
	return nil
}

func (s *openstackstorage) List(prefix string) ([]string, error) {
	contents, err := s.swift.List(s.containerName, prefix, "", "", 0)
	if err != nil {
		// If the container is not found, it's not an error
		// because it's only created when the first
		// file is put.
		if err, ok := maybeNotFound(err); !ok {
			return nil, err
		}
		return nil, nil
	}
	var names []string
	for _, item := range contents {
		names = append(names, item.Name)
	}
	return names, nil
}

// Spawn this many goroutines to issue requests for deleting items from the
// server. If only Openstack had a delete many request.
const maxConcurrentDeletes = 8

// RemoveAll is specified in the StorageWriter interface.
func (s *openstackstorage) RemoveAll() error {
	names, err := storage.List(s, "")
	if err != nil {
		return err
	}
	// Remove all the objects in parallel so as to minimize round-trips.
	// Start with a goroutine feeding all the names that need to be
	// deleted.
	toDelete := make(chan string)
	go func() {
		for _, name := range names {
			toDelete <- name
		}
		close(toDelete)
	}()
	// Now spawn up to N routines to actually issue the requests.
	maxRoutines := len(names)
	if maxConcurrentDeletes < maxRoutines {
		maxRoutines = maxConcurrentDeletes
	}
	var wg sync.WaitGroup
	wg.Add(maxRoutines)
	// Make a channel long enough to buffer all possible errors.
	errc := make(chan error, len(names))
	for i := 0; i < maxRoutines; i++ {
		go func() {
			for name := range toDelete {
				if err := s.Remove(name); err != nil {
					errc <- err
				}
			}
			wg.Done()
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
	err = s.swift.DeleteContainer(s.containerName)
	err, ok := maybeNotFound(err)
	if ok {
		return nil
	}
	return err
}

// maybeNotFound returns a errors.NotFoundError if the root cause of the specified error is due to a file or
// container not being found.
func maybeNotFound(err error) (error, bool) {
	if err != nil && isNotFoundError(err) {
		return jujuerrors.NewNotFound(err, ""), true
	}
	return err, false
}
