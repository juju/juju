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
	// Slashes are problematic as path separators: file names are
	// sometimes included in URL paths, where we'd need to escape them
	// but standard URL-escaping won't do so.  We can't escape them before
	// they go into the URLs because subsequent URL escaping would escape
	// the percentage signs in the escaping itself.
	// Use a different separator instead.
	const separator = "__"
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

// retrieveFileObject retrieves the information of the named file, including
// its download URL and its contents, as a MAASObject.
func (stor *maasStorage) retrieveFileObject(name string) (gomaasapi.MAASObject, error) {
	snapshot := stor.getSnapshot()
	fullName := snapshot.namingPrefix + name
	obj, err := snapshot.maasClientUnlocked.GetSubObject(fullName).Get()
	if err != nil {
		noObj := gomaasapi.MAASObject{}
		serverErr, ok := err.(gomaasapi.ServerError)
		if ok && serverErr.StatusCode == 404 {
			msg := fmt.Errorf("file '%s' not found", name)
			return noObj, environs.NotFoundError{msg}
		}
		msg := fmt.Errorf("could not access file '%s': %v", name, err)
		return noObj, msg
	}
	return obj, nil
}

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

func (stor *maasStorage) URL(name string) (string, error) {
	fileObj, err := stor.retrieveFileObject(name)
	if err != nil {
		return "", err
	}
	url, err := fileObj.GetField("anon_resource_uri")
	if err != nil {
		msg := fmt.Errorf("could not get file's download URL (may be an outdated MAAS): %s", err)
		return "", msg
	}

	return url, nil
}

func (*maasStorage) Put(name string, r io.Reader, length int64) error {
	panic("Not implemented.")
}

func (*maasStorage) Remove(name string) error {
	panic("Not implemented.")
}
