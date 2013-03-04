package maas

import (
	"fmt"
	"io"
	"launchpad.net/gomaasapi"
	"launchpad.net/juju-core/environs"
	"net/url"
	"sort"
	"strings"
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
	return "juju/" + env.Name() + "/"
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
		namingPrefix:       stor.namingPrefix,
		environUnlocked:    stor.environUnlocked,
		maasClientUnlocked: stor.maasClientUnlocked,
	}
}

func (*maasStorage) Get(name string) (io.ReadCloser, error) {
	panic("Not implemented.")
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
		if !strings.HasPrefix(filename, stor.namingPrefix) {
			msg := fmt.Errorf("unexpected filename '%s' lacks environment prefix '%s'", filename, stor.namingPrefix)
			return nil, msg
		}
		filename = filename[len(stor.namingPrefix):]
		result[index] = filename
	}
	sort.Strings(result)
	return result, nil
}

func (stor *maasStorage) List(prefix string) ([]string, error) {
	snapshot := stor.getSnapshot()
	params := make(url.Values)
	if len(prefix) > 0 {
		params.Add("prefix", snapshot.namingPrefix+prefix)
	}
	obj, err := snapshot.maasClientUnlocked.CallGet("list", params)
	if err != nil {
		return nil, err
	}
	return snapshot.extractFilenames(obj)
}

func (*maasStorage) URL(name string) (string, error) {
	panic("Not implemented.")
}

func (*maasStorage) Put(name string, r io.Reader, length int64) error {
	panic("Not implemented.")
}

func (*maasStorage) Remove(name string) error {
	panic("Not implemented.")
}
