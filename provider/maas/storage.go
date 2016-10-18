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
	"strings"
	"sync"

	"github.com/juju/errors"
	"github.com/juju/gomaasapi"
	"github.com/juju/utils"

	"github.com/juju/juju/environs/storage"
)

type maas1Storage struct {
	// The Environ that this Storage is for.
	environ *maasEnviron

	// Reference to the URL on the API where files are stored.
	maasClient gomaasapi.MAASObject
}

var _ storage.Storage = (*maas1Storage)(nil)

func NewStorage(env *maasEnviron) storage.Storage {
	if env.usingMAAS2() {
		return &maas2Storage{
			environ:        env,
			maasController: env.maasController,
		}
	} else {
		return &maas1Storage{
			environ:    env,
			maasClient: env.getMAASClient().GetSubObject("files"),
		}
	}
}

// addressFileObject creates a MAASObject pointing to a given file.
// Takes out a lock on the storage object to get a consistent view.
func (stor *maas1Storage) addressFileObject(name string) gomaasapi.MAASObject {
	return stor.maasClient.GetSubObject(name)
}

// All filenames need to be namespaced so they are private to this environment.
// This prevents different environments from interfering with each other.
// We're using the agent name UUID here.
func prefixWithPrivateNamespace(env *maasEnviron, name string) string {
	return env.uuid + "-" + name
}

func (stor *maas1Storage) prefixWithPrivateNamespace(name string) string {
	return prefixWithPrivateNamespace(stor.environ, name)
}

// retrieveFileObject retrieves the information of the named file,
// including its download URL and its contents, as a MAASObject.
//
// This may return many different errors, but specifically, it returns
// an error that satisfies errors.IsNotFound if the file did not
// exist.
//
// The function takes out a lock on the storage object.
func (stor *maas1Storage) retrieveFileObject(name string) (gomaasapi.MAASObject, error) {
	obj, err := stor.addressFileObject(name).Get()
	if err != nil {
		noObj := gomaasapi.MAASObject{}
		serverErr, ok := errors.Cause(err).(gomaasapi.ServerError)
		if ok && serverErr.StatusCode == 404 {
			return noObj, errors.NotFoundf("file '%s' not found", name)
		}
		msg := fmt.Errorf("could not access file '%s': %v", name, err)
		return noObj, msg
	}
	return obj, nil
}

// Get is specified in the StorageReader interface.
func (stor *maas1Storage) Get(name string) (io.ReadCloser, error) {
	name = stor.prefixWithPrivateNamespace(name)
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
func (stor *maas1Storage) extractFilenames(listResult gomaasapi.JSONObject) ([]string, error) {
	privatePrefix := stor.prefixWithPrivateNamespace("")
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
		// When listing files we need to return them without our special prefix.
		result[index] = strings.TrimPrefix(filename, privatePrefix)
	}
	sort.Strings(result)
	return result, nil
}

// List is specified in the StorageReader interface.
func (stor *maas1Storage) List(prefix string) ([]string, error) {
	prefix = stor.prefixWithPrivateNamespace(prefix)
	params := make(url.Values)
	params.Add("prefix", prefix)
	obj, err := stor.maasClient.CallGet("list", params)
	if err != nil {
		return nil, err
	}
	return stor.extractFilenames(obj)
}

// URL is specified in the StorageReader interface.
func (stor *maas1Storage) URL(name string) (string, error) {
	name = stor.prefixWithPrivateNamespace(name)
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

// DefaultConsistencyStrategy is specified in the StorageReader interface.
func (stor *maas1Storage) DefaultConsistencyStrategy() utils.AttemptStrategy {
	// This storage backend has immediate consistency, so there's no
	// need to wait.  One attempt should do.
	return utils.AttemptStrategy{}
}

// ShouldRetry is specified in the StorageReader interface.
func (stor *maas1Storage) ShouldRetry(err error) bool {
	return false
}

// Put is specified in the StorageWriter interface.
func (stor *maas1Storage) Put(name string, r io.Reader, length int64) error {
	name = stor.prefixWithPrivateNamespace(name)
	data, err := ioutil.ReadAll(io.LimitReader(r, length))
	if err != nil {
		return err
	}
	params := url.Values{"filename": {name}}
	files := map[string][]byte{"file": data}
	_, err = stor.maasClient.CallPostFiles("add", params, files)
	return err
}

// Remove is specified in the StorageWriter interface.
func (stor *maas1Storage) Remove(name string) error {
	name = stor.prefixWithPrivateNamespace(name)
	// The only thing that can go wrong here, really, is that the file
	// does not exist.  But deletion is idempotent: deleting a file that
	// is no longer there anyway is success, not failure.
	stor.maasClient.GetSubObject(name).Delete()
	return nil
}

// RemoveAll is specified in the StorageWriter interface.
func (stor *maas1Storage) RemoveAll() error {
	return removeAll(stor)
}

func removeAll(stor storage.Storage) error {
	names, err := storage.List(stor, "")
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
