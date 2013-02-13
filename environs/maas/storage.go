package maas

import (
	"io"
	"launchpad.net/gomaasapi"
	"launchpad.net/juju-core/environs"
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

// Compose a prefix for all files stored by this environment.
func composeNamingPrefix(env *maasEnviron) string {
	prefix := filepath.Join("juju/", env.Name())
	return filepath.ToSlash(prefix)
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

func (*maasStorage) Get(name string) (io.ReadCloser, error) {
	panic("Not implemented.")
	// TODO: If the name does not exist, return *NotFoundError.
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
