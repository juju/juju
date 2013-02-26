package maas

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"io/ioutil"
	"launchpad.net/gomaasapi"
	"launchpad.net/juju-core/environs"
	"net/url"
	"path/filepath"
	"sync"
)

type maasStorage struct {
	// Mutex protects the "*Unlocked" fields.
	sync.Mutex

	// Prefix for all files relevant to this Storage.  Immutable, so
	// no need to lock.
	namingPrefix string

	// The Environ that this Storage is for.
	environUnlocked *maasEnviron

	// Reference to the URL on the API where files are stored.
	maasClientUnlocked gomaasapi.MAASObject
}

var _ environs.Storage = (*maasStorage)(nil)

// composeNamingPrefix generates a consistent naming prefix for all files
// stored by this environment.
func composeNamingPrefix(env *maasEnviron) string {
	// Slashes are problematic as path separators: file names are
	// sometimes included in URL paths, where we'd need to escape them
	// but standard URL-escaping won't do so.  We can't escape them before
	// they go into the URLs because subsequent URL escaping would escape
	// the percentage signs in the escaping itself.
	// Use a different separator instead.
	separator := ".-."
	return "juju" + separator + env.Name() + separator
}

func NewStorage(env *maasEnviron) environs.Storage {
	env.ecfgMutex.Lock()
	defer env.ecfgMutex.Unlock()

	filesClient := env.maasClientUnlocked.GetSubObject("files")
	storage := new(maasStorage)
	storage.environUnlocked = env
	storage.maasClientUnlocked = filesClient
	storage.namingPrefix = composeNamingPrefix(env)
	return storage
}

// addressFileObject returns the URL where the named file's details can be
// retrieved from the MAAS API.
func (stor *maasStorage) addressFileObject(filename string) gomaasapi.MAASObject {
	fullName := filepath.Join(stor.namingPrefix, filename)
	return stor.maasClientUnlocked.GetSubObject(fullName)
}

// composeAnonymousFileURL returns the URL where the named file's contents can
// be downloaded, anonymously, from the MAAS API.
func (stor *maasStorage) composeAnonymousFileURL(filename string) url.URL {
	stor.Lock()
	defer stor.Unlock()

	result := *stor.maasClientUnlocked.URL()
	query := result.Query()
	query.Add("filename", filepath.Join(stor.namingPrefix, filename))
	result.RawQuery = query.Encode()
	return result
}

func (stor *maasStorage) Get(name string) (io.ReadCloser, error) {
	fileObj, err := stor.addressFileObject(name).Get()
	if err != nil {
		serverErr, ok := err.(gomaasapi.ServerError)
		if ok && serverErr.StatusCode == 404 {
			msg := fmt.Errorf("file %s not found", name)
			return nil, environs.NotFoundError{msg}
		}
		return nil, fmt.Errorf("could not access file '%s': %v", name, err)
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

func (*maasStorage) List(prefix string) ([]string, error) {
	panic("Not implemented.")
	// TODO: List in "alphabetical" order.  Slashes are not special; treat as letters.
}

func (*maasStorage) URL(name string) (string, error) {
	panic("Not implemented.")
	// TODO: Return URL for the given storage file.
}

func (*maasStorage) Put(name string, r io.Reader, length int64) error {
	panic("Not implemented.")
}

func (*maasStorage) Remove(name string) error {
	panic("Not implemented.")
}
