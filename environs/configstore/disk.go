// Copyright 2013-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package configstore

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/featureflag"
	"github.com/juju/utils/fslock"
	"github.com/juju/utils/set"
	goyaml "gopkg.in/yaml.v1"

	"github.com/juju/juju/feature"
	"github.com/juju/juju/juju/osenv"
)

var logger = loggo.GetLogger("juju.environs.configstore")

type configSource string

const (
	lockName = "env.lock"

	sourceCreated configSource = "created"
	sourceJenv    configSource = "jenv"
	sourceCache   configSource = "cache"
	sourceMem     configSource = "mem"
)

// A second should be way more than enough to write or read any files.
var lockTimeout = time.Second

// Default returns disk-based environment config storage
// rooted at JujuHome.
var Default = func() (Storage, error) {
	return NewDisk(osenv.JujuHome())
}

type diskStore struct {
	dir string
}

// EnvironInfoData is the serialisation structure for the original JENV file.
type EnvironInfoData struct {
	User            string
	Password        string
	EnvironUUID     string                 `json:"environ-uuid,omitempty" yaml:"environ-uuid,omitempty"`
	ServerUUID      string                 `json:"server-uuid,omitempty" yaml:"server-uuid,omitempty"`
	StateServers    []string               `json:"state-servers" yaml:"state-servers"`
	ServerHostnames []string               `json:"server-hostnames,omitempty" yaml:"server-hostnames,omitempty"`
	CACert          string                 `json:"ca-cert" yaml:"ca-cert"`
	Config          map[string]interface{} `json:"bootstrap-config,omitempty" yaml:"bootstrap-config,omitempty"`
}

type environInfo struct {
	mu sync.Mutex

	// environmentDir is the directory where the files are written.
	environmentDir string

	// path is the location of the file that we read to load the info.
	path string

	// source identifies how this instance was created
	source configSource

	name            string
	user            string
	credentials     string
	environmentUUID string
	serverUUID      string
	apiEndpoints    []string
	apiHostnames    []string
	caCert          string
	bootstrapConfig map[string]interface{}
}

// NewDisk returns a ConfigStorage implementation that stores configuration in
// the given directory. The parent of the directory must already exist; the
// directory itself is created if it doesn't already exist.
func NewDisk(dir string) (Storage, error) {
	if _, err := os.Stat(dir); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	d := &diskStore{
		dir: filepath.Join(dir, "environments"),
	}
	if err := d.mkEnvironmentsDir(); err != nil {
		return nil, err
	}
	return d, nil
}

func (d *diskStore) mkEnvironmentsDir() error {
	err := os.Mkdir(d.dir, 0700)
	if os.IsExist(err) {
		return nil
	}
	logger.Debugf("Made dir %v", d.dir)
	return err
}

// CreateInfo implements Storage.CreateInfo.
func (d *diskStore) CreateInfo(envName string) EnvironInfo {
	return &environInfo{
		environmentDir: d.dir,
		source:         sourceCreated,
		name:           envName,
	}
}

// List implements Storage.List
func (d *diskStore) List() ([]string, error) {
	var envs []string

	// Awkward -  list both jenv files and cache entries.
	cache, err := readCacheFile(cacheFilename(d.dir))
	if err != nil {
		return nil, errors.Trace(err)
	}
	for name := range cache.Environment {
		envs = append(envs, name)
	}

	files, err := filepath.Glob(d.dir + "/*" + jenvExtension)
	if err != nil {
		return nil, err
	}
	for _, file := range files {
		fName := filepath.Base(file)
		name := fName[:len(fName)-len(jenvExtension)]
		envs = append(envs, name)
	}
	return envs, nil
}

// ListSystems implements Storage.ListSystems
func (d *diskStore) ListSystems() ([]string, error) {
	// List both jenv files and cache entries.  Record
	// results in a set to avoid repeat entries.
	servers := set.NewStrings()
	cache, err := readCacheFile(cacheFilename(d.dir))
	if err != nil {
		return nil, errors.Trace(err)
	}
	for name := range cache.Server {
		servers.Add(name)
	}

	files, err := filepath.Glob(d.dir + "/*" + jenvExtension)
	if err != nil {
		return nil, err
	}
	for _, file := range files {
		fName := filepath.Base(file)
		name := fName[:len(fName)-len(jenvExtension)]
		env, err := d.ReadInfo(name)
		if err != nil {
			return nil, err
		}

		// If ServerUUID is not set, it is an old env and is a
		// server by default.  Otherwise, if the server and env
		// UUIDs match, it is a server.
		api := env.APIEndpoint()
		if api.ServerUUID == "" || api.ServerUUID == api.EnvironUUID {
			servers.Add(name)
		}
	}
	return servers.SortedValues(), nil
}

// ReadInfo implements Storage.ReadInfo.
func (d *diskStore) ReadInfo(envName string) (EnvironInfo, error) {
	// NOTE: any reading or writing from the directory should be done with a
	// fslock to make sure we have a consistent read or write.  Also worth
	// noting, we should use a very short timeout.
	lock, err := acquireEnvironmentLock(d.dir, "reading")
	if err != nil {
		return nil, errors.Annotatef(err, "cannot read info")
	}
	defer lock.Unlock()

	info, err := d.readCacheFile(envName)
	if err != nil {
		if errors.IsNotFound(err) {
			info, err = d.readJENVFile(envName)
		}
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	info.environmentDir = d.dir
	return info, nil
}

func cacheFilename(dir string) string {
	return filepath.Join(dir, "cache.yaml")
}

func (d *diskStore) readCacheFile(envName string) (*environInfo, error) {
	cache, err := readCacheFile(cacheFilename(d.dir))
	if err != nil {
		return nil, errors.Trace(err)
	}
	info, err := cache.readInfo(envName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return info, nil
}

// Initialized implements EnvironInfo.Initialized.
func (info *environInfo) Initialized() bool {
	info.mu.Lock()
	defer info.mu.Unlock()
	return info.initialized()
}

func (info *environInfo) initialized() bool {
	return info.source != sourceCreated
}

// BootstrapConfig implements EnvironInfo.BootstrapConfig.
func (info *environInfo) BootstrapConfig() map[string]interface{} {
	info.mu.Lock()
	defer info.mu.Unlock()
	return info.bootstrapConfig
}

// APICredentials implements EnvironInfo.APICredentials.
func (info *environInfo) APICredentials() APICredentials {
	info.mu.Lock()
	defer info.mu.Unlock()
	return APICredentials{
		User:     info.user,
		Password: info.credentials,
	}
}

// APIEndpoint implements EnvironInfo.APIEndpoint.
func (info *environInfo) APIEndpoint() APIEndpoint {
	info.mu.Lock()
	defer info.mu.Unlock()
	return APIEndpoint{
		Addresses:   info.apiEndpoints,
		Hostnames:   info.apiHostnames,
		CACert:      info.caCert,
		EnvironUUID: info.environmentUUID,
		ServerUUID:  info.serverUUID,
	}
}

// SetBootstrapConfig implements EnvironInfo.SetBootstrapConfig.
func (info *environInfo) SetBootstrapConfig(attrs map[string]interface{}) {
	info.mu.Lock()
	defer info.mu.Unlock()
	if info.source != sourceCreated {
		panic("bootstrap config set on environment info that has not just been created")
	}
	info.bootstrapConfig = attrs
}

// SetAPIEndpoint implements EnvironInfo.SetAPIEndpoint.
func (info *environInfo) SetAPIEndpoint(endpoint APIEndpoint) {
	info.mu.Lock()
	defer info.mu.Unlock()
	info.apiEndpoints = endpoint.Addresses
	info.apiHostnames = endpoint.Hostnames
	info.caCert = endpoint.CACert
	info.environmentUUID = endpoint.EnvironUUID
	info.serverUUID = endpoint.ServerUUID
}

// SetAPICredentials implements EnvironInfo.SetAPICredentials.
func (info *environInfo) SetAPICredentials(creds APICredentials) {
	info.mu.Lock()
	defer info.mu.Unlock()
	info.user = creds.User
	info.credentials = creds.Password
}

// Location returns the location of the environInfo in human readable format.
func (info *environInfo) Location() string {
	info.mu.Lock()
	defer info.mu.Unlock()
	return fmt.Sprintf("file %q", info.path)
}

// Write implements EnvironInfo.Write.
func (info *environInfo) Write() error {
	info.mu.Lock()
	defer info.mu.Unlock()
	lock, err := acquireEnvironmentLock(info.environmentDir, "writing")
	if err != nil {
		return errors.Annotatef(err, "cannot write info")
	}
	defer lock.Unlock()

	// In order to write out the environment info to the cache
	// file we need to make sure the server UUID is set. Sufficiently
	// up to date servers will write the server UUID to the JENV
	// file as connections are made to the API server. It is possible
	// that for an old JENV file, the first update (on API connection)
	// may write a JENV file, and the subsequent update will create the
	// entry in the cache file.
	// If the source was the cache file, then always write there to
	// avoid stale data in the cache file.
	if info.source == sourceCache ||
		(featureflag.Enabled(feature.JES) && info.serverUUID != "") {
		if err := info.ensureNoJENV(); info.source == sourceCreated && err != nil {
			return errors.Trace(err)
		}
		logger.Debugf("writing cache file")
		filename := cacheFilename(info.environmentDir)
		cache, err := readCacheFile(filename)
		if err != nil {
			return errors.Trace(err)
		}
		if err := cache.updateInfo(info); err != nil {
			return errors.Trace(err)
		}
		if err := writeCacheFile(filename, cache); err != nil {
			return errors.Trace(err)
		}
		oldPath := info.path
		info.path = filename
		// If source was jenv file, delete the jenv.
		if info.source == sourceJenv {
			err := os.Remove(oldPath)
			if err != nil {
				return errors.Trace(err)
			}
		}
		info.source = sourceCache
	} else {
		logger.Debugf("writing jenv file")
		if err := info.writeJENVFile(); err != nil {
			return errors.Trace(err)
		}
		info.source = sourceJenv
	}
	return nil
}

// Destroy implements EnvironInfo.Destroy.
func (info *environInfo) Destroy() error {
	info.mu.Lock()
	defer info.mu.Unlock()
	lock, err := acquireEnvironmentLock(info.environmentDir, "destroying")
	if err != nil {
		return errors.Annotatef(err, "cannot destroy environment info")
	}
	defer lock.Unlock()

	if info.initialized() {
		if info.source == sourceJenv {
			err := os.Remove(info.path)
			if os.IsNotExist(err) {
				return errors.New("environment info has already been removed")
			}
			return err
		}
		if info.source == sourceCache {
			filename := cacheFilename(info.environmentDir)
			cache, err := readCacheFile(filename)
			if err != nil {
				return errors.Trace(err)
			}
			if err := cache.removeInfo(info); err != nil {
				return errors.Trace(err)
			}
			if err := writeCacheFile(filename, cache); err != nil {
				return errors.Trace(err)
			}
			return nil
		}
		return errors.Errorf("unknown source %q for environment info", info.source)
	}
	return nil
}

const jenvExtension = ".jenv"

func jenvFilename(basedir, envName string) string {
	return filepath.Join(basedir, envName+jenvExtension)
}

func (d *diskStore) readJENVFile(envName string) (*environInfo, error) {
	path := jenvFilename(d.dir, envName)
	data, err := ioutil.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errors.NotFoundf("environment %q", envName)
		}
		return nil, err
	}
	var info environInfo
	info.path = path
	if len(data) == 0 {
		return &info, nil
	}
	var values EnvironInfoData
	if err := goyaml.Unmarshal(data, &values); err != nil {
		return nil, errors.Annotatef(err, "error unmarshalling %q", path)
	}
	info.name = envName
	info.user = values.User
	info.credentials = values.Password
	info.environmentUUID = values.EnvironUUID
	info.serverUUID = values.ServerUUID
	info.caCert = values.CACert
	info.apiEndpoints = values.StateServers
	info.apiHostnames = values.ServerHostnames
	info.bootstrapConfig = values.Config

	info.source = sourceJenv
	return &info, nil
}

func (info *environInfo) ensureNoJENV() error {
	path := jenvFilename(info.environmentDir, info.name)
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err == nil {
		return ErrEnvironInfoAlreadyExists
	}
	return err
}

// Kept primarily for testing purposes now.
func (info *environInfo) writeJENVFile() error {

	infoData := EnvironInfoData{
		User:            info.user,
		Password:        info.credentials,
		EnvironUUID:     info.environmentUUID,
		ServerUUID:      info.serverUUID,
		StateServers:    info.apiEndpoints,
		ServerHostnames: info.apiHostnames,
		CACert:          info.caCert,
		Config:          info.bootstrapConfig,
	}

	data, err := goyaml.Marshal(infoData)
	if err != nil {
		return errors.Annotate(err, "cannot marshal environment info")
	}
	// We now use a fslock to sync reads and writes across the environment,
	// so we don't need to use a temporary file any more.

	flags := os.O_WRONLY
	if info.initialized() {
		flags |= os.O_TRUNC
	} else {
		flags |= os.O_CREATE | os.O_EXCL
	}
	path := jenvFilename(info.environmentDir, info.name)
	logger.Debugf("writing jenv file to %s", path)
	file, err := os.OpenFile(path, flags, 0600)
	if os.IsExist(err) {
		return ErrEnvironInfoAlreadyExists
	}

	_, err = file.Write(data)
	file.Close()
	info.path = path
	return errors.Annotate(err, "cannot write file")
}

func acquireEnvironmentLock(dir, operation string) (*fslock.Lock, error) {
	lock, err := fslock.NewLock(dir, lockName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	message := fmt.Sprintf("pid: %d, operation: %s", os.Getpid(), operation)
	err = lock.LockWithTimeout(lockTimeout, message)
	if err != nil {
		logger.Warningf("configstore lock held, lock dir: %s", filepath.Join(dir, lockName))
		logger.Warningf("  lock holder message: %s", lock.Message())
		return nil, errors.Trace(err)
	}
	return lock, nil
}
