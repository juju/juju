// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"sort"
	"sync"

	"launchpad.net/gomaasapi"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/utils"
)

type maasStorage struct {
	// Mutex protects the "*Unlocked" fields.
	sync.Mutex

	// The Environ that this Storage is for.
	environUnlocked *maasEnviron

	// Reference to the URL on the API where files are stored.
	maasClientUnlocked gomaasapi.MAASObject
}

var _ environs.Storage = (*maasStorage)(nil)

func NewStorage(env *maasEnviron) environs.Storage {
	storage := new(maasStorage)
	storage.environUnlocked = env
	storage.maasClientUnlocked = env.getMAASClient().GetSubObject("files")
	return storage
}

// getSnapshot returns a consistent copy of a maasStorage.  Use this if you
// need a consistent view of the object's entire state, without having to
// lock the object the whole time.
//
// An easy mistake to make with "defer" is to keep holding a lock without
// realizing it, while you go on to block on http requests or other slow
// things that don't actually require the lock.  In most cases you can just
// create a snapshot first (releasing the lock immediately) and then do the
// rest of the work with the snapshot.
func (stor *maasStorage) getSnapshot() *maasStorage {
	stor.Lock()
	defer stor.Unlock()

	return &maasStorage{
		environUnlocked:    stor.environUnlocked,
		maasClientUnlocked: stor.maasClientUnlocked,
	}
}

// addressFileObject creates a MAASObject pointing to a given file.
// Takes out a lock on the storage object to get a consistent view.
func (stor *maasStorage) addressFileObject(name string) gomaasapi.MAASObject {
	stor.Lock()
	defer stor.Unlock()
	return stor.maasClientUnlocked.GetSubObject(name)
}

// retrieveFileObject retrieves the information of the named file, including
// its download URL and its contents, as a MAASObject.
//
// This may return many different errors, but specifically, it returns
// an error that satisfies errors.IsNotFoundError if the file did not exist.
//
// The function takes out a lock on the storage object.
func (stor *maasStorage) retrieveFileObject(name string) (gomaasapi.MAASObject, error) {
	obj, err := stor.addressFileObject(name).Get()
	if err != nil {
		noObj := gomaasapi.MAASObject{}
		serverErr, ok := err.(gomaasapi.ServerError)
		if ok && serverErr.StatusCode == 404 {
			return noObj, errors.NotFoundf("file '%s' not found", name)
		}
		msg := fmt.Errorf("could not access file '%s': %v", name, err)
		return noObj, msg
	}
	return obj, nil
}

// Get is specified in the StorageReader interface.
func (stor *maasStorage) Get(name string) (io.ReadCloser, error) {
	fileObj, err := stor.retrieveFileObject(name)
	if err != nil {
		return nil, err
	}
	data, err := fileObj.GetField("content")
	if err != nil {
		return nil, fmt.Errorf("could not extract file content for %s: %v", name, err)
	}
	buf, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return nil, fmt.Errorf("bad data in file '%s': %v", name, err)
	}
	return ioutil.NopCloser(bytes.NewReader(buf)), nil
}

// extractFilenames returns the filenames from a "list" operation on the
// MAAS API, sorted by name.
func (stor *maasStorage) extractFilenames(listResult gomaasapi.JSONObject) ([]string, error) {
	list, err := listResult.GetArray()
	if err != nil {
		return nil, err
	}
	result := make([]string, len(list))
	for index, entry := range list {
		file, err := entry.GetMap()
		if err != nil {
			return nil, err
		}
		filename, err := file["filename"].GetString()
		if err != nil {
			return nil, err
		}
		result[index] = filename
	}
	sort.Strings(result)
	return result, nil
}

// List is specified in the StorageReader interface.
func (stor *maasStorage) List(prefix string) ([]string, error) {
	params := make(url.Values)
	params.Add("prefix", prefix)
	snapshot := stor.getSnapshot()
	obj, err := snapshot.maasClientUnlocked.CallGet("list", params)
	if err != nil {
		return nil, err
	}
	return snapshot.extractFilenames(obj)
}

// URL is specified in the StorageReader interface.
func (stor *maasStorage) URL(name string) (string, error) {
	fileObj, err := stor.retrieveFileObject(name)
	if err != nil {
		return "", err
	}
	uri, err := fileObj.GetField("anon_resource_uri")
	if err != nil {
		msg := fmt.Errorf("could not get file's download URL (may be an outdated MAAS): %s", err)
		return "", msg
	}

	partialURL, err := url.Parse(uri)
	if err != nil {
		return "", err
	}
	fullURL := fileObj.URL().ResolveReference(partialURL)
	return fullURL.String(), nil
}

// ConsistencyStrategy is specified in the StorageReader interface.
func (stor *maasStorage) ConsistencyStrategy() utils.AttemptStrategy {
	// This storage backend has immediate consistency, so there's no
	// need to wait.  One attempt should do.
	return utils.AttemptStrategy{}
}

// Put is specified in the StorageWriter interface.
func (stor *maasStorage) Put(name string, r io.Reader, length int64) error {
	data, err := ioutil.ReadAll(io.LimitReader(r, length))
	if err != nil {
		return err
	}
	params := url.Values{"filename": {name}}
	files := map[string][]byte{"file": data}
	snapshot := stor.getSnapshot()
	_, err = snapshot.maasClientUnlocked.CallPostFiles("add", params, files)
	return err
}

// Remove is specified in the StorageWriter interface.
func (stor *maasStorage) Remove(name string) error {
	// The only thing that can go wrong here, really, is that the file
	// does not exist.  But deletion is idempotent: deleting a file that
	// is no longer there anyway is success, not failure.
	stor.getSnapshot().maasClientUnlocked.GetSubObject(name).Delete()
	return nil
}

// RemoveAll is specified in the StorageWriter interface.
func (stor *maasStorage) RemoveAll() error {
	names, err := stor.List("")
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
			if err := stor.Remove(name); err != nil {
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
