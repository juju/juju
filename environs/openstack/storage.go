package openstack

import (
	"fmt"
	"io"
	"launchpad.net/goose/errors"
	goosehttp "launchpad.net/goose/http"
	"launchpad.net/goose/swift"
	"launchpad.net/juju-core/environs"
	"net/http"
	"sync"
	"time"
)

// storage implements environs.Storage on an OpenStack container.
type storage struct {
	containerMutex sync.Mutex
	madeContainer  bool
	containerName   string
	swift *swift.Client
}

// makeContainer makes the environment's control container, the
// place where bootstrap information and deployed charms
// are stored. To avoid two round trips on every PUT operation,
// we do this only once for each environ.
func (s *storage) makeContainer(containerName string) error {
	s.containerMutex.Lock()
	defer s.containerMutex.Unlock()
	if s.madeContainer {
		return nil
	}
	// try to make the container - CreateContainer will succeed if the container already exists.
	// TODO(wallyworld) - add security specification?
	err := s.swift.CreateContainer(containerName)
	if err == nil {
		s.madeContainer = true
	}
	return err
}

func (s *storage) Put(file string, r io.Reader, length int64) error {
	if err := s.makeContainer(s.containerName); err != nil {
		return fmt.Errorf("cannot make Swift control container: %v", err)
	}
	// TODO(wallyuworld) - add security specification?
	err := s.swift.PutReader(s.containerName, file, r, length)
	if err != nil {
		return fmt.Errorf("cannot write file %q to control container %q: %v", file, s.containerName, err)
	}
	return nil
}

func (s *storage) Get(file string) (r io.ReadCloser, err error) {
	for a := shortAttempt.Start(); a.Next(); {
		r, err = s.swift.GetReader(s.containerName, file)
		if errors.IsNotFound(err) {
			continue
		}
	}
	err, _ = maybeNotFound(err)
	if err != nil {
		return nil, err
	}
	return r, nil
}

func (s *storage) URL(name string) (string, error) {
	// 10 years should be good enough.
	expires := time.Now().AddDate(1, 0, 0)
	return s.swift.SignedURL(s.containerName, name, expires)
}

func (s *storage) Remove(file string) error {
	err := s.swift.DeleteObject(s.containerName, file)
	// If we can't delete the object because the bucket doesn't
	// exist, then we don't care.
	err, ok := maybeNotFound(err)
	if ok {
		return nil
	}
	return err
}

func (s *storage) List(prefix string) ([]string, error) {
	contents, err := s.swift.List(s.containerName, prefix, "", "", 0)
	if err != nil {
		// If the container is not found, it's not an error
		// because it's only created when the first
		// file is put.
		err, ok := maybeNotFound(err)
		if ok {
			return nil, nil
		}
		return nil, err
	}
	var names []string
	for _, item := range contents {
		names = append(names, item.Name)
	}
	return names, nil
}

func (s *storage) deleteAll() error {
	names, err := s.List("")
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
			if err := s.Remove(name); err != nil {
				errc <- err
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

	s.containerMutex.Lock()
	defer s.containerMutex.Unlock()
	// Even DeleteContainer fails, it won't harm if we try again - the operation
	// might have succeeded even if we get an error.
	s.madeContainer = false
	err = s.swift.DeleteContainer(s.containerName)
	err, ok := maybeNotFound(err)
	if ok {
		return nil
	}
	return err
}

// maybeNotFound returns a environs.NotFoundError if the root cause of the specified error is due to a file or
// container not being found.
func maybeNotFound(err error) (error, bool) {
	if err == nil {
		return nil, false
	}
	error, ok := err.(errors.Error)
	if !ok {
		return err, false
	}
	var statusCode int
	if context, ok := error.Context().(goosehttp.ResponseData); ok {
		statusCode = context.StatusCode
	}
	if errors.IsNotFound(err) || statusCode == http.StatusPreconditionFailed {
		return &environs.NotFoundError{err}, true
	}
	return err, false
}
