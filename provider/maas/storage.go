// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"fmt"
	"sync"

	"github.com/juju/juju/environs/storage"
)

func NewStorage(env *maasEnviron) storage.Storage {
	return &maasStorage{
		environ:        env,
		maasController: env.maasController,
	}
}

func removeAll(store storage.Storage) error {
	names, err := storage.List(store, "")
	if err != nil {
		return err
	}
	// Remove all the objects in parallel so that we incur fewer round-trips.
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
			if err := store.Remove(name); err != nil {
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
	return nil
}
