// Copyright 2013 Canonical Ltd.
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
	"github.com/juju/utils/fslock"
	goyaml "gopkg.in/yaml.v1"

	"github.com/juju/juju/juju/osenv"
)

var logger = loggo.GetLogger("juju.environs.configstore")

const lockName = "env.lock"

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
	User         string
	Password     string
	EnvironUUID  string                 `json:"environ-uuid,omitempty" yaml:"environ-uuid,omitempty"`
	StateServers []string               `json:"state-servers" yaml:"state-servers"`
	CACert       string                 `json:"ca-cert" yaml:"ca-cert"`
	Config       map[string]interface{} `json:"bootstrap-config,omitempty" yaml:"bootstrap-config,omitempty"`
}

type environInfo struct {
	mu sync.Mutex

	// environmentDir is the directory where the files are written.
	environmentDir string

	// path is the location of the file that we read to load the info.
	path string

	// initialized signifies whether the info has been written.
	initialized bool

	// created signifies whether the info was returned from
	// a CreateInfo call.
	created bool

	name            string
	user            string
	credentials     string
	environmentUUID string
	apiEndpoints    []string
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
		created:        true,
		name:           envName,
	}
}

// List implements Storage.List
func (d *diskStore) List() ([]string, error) {

	// awkward -  list both jenv files and connection files.

	var envs []string
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

// ReadInfo implements Storage.ReadInfo.
func (d *diskStore) ReadInfo(envName string) (EnvironInfo, error) {
	// TODO: first try the new format, and if it doesn't exist, read the old format.
	// NOTE: any reading or writing from the directory should be done with a fslock
	// to make sure we have a consistent read or write.  Also worth noting, we should
	// use a very short timeout.

	lock, err := fslock.NewLock(d.dir, lockName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	err = lock.LockWithTimeout(lockTimeout, "reading")
	if err != nil {
		return nil, errors.Annotatef(err, "cannot read info")
	}
	defer lock.Unlock()

	info, err := d.readConnectionFile(envName)
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

func (d *diskStore) readConnectionFile(envName string) (*environInfo, error) {
	return nil, errors.NotFoundf("connection file")
}

// Initialized implements EnvironInfo.Initialized.
func (info *environInfo) Initialized() bool {
	info.mu.Lock()
	defer info.mu.Unlock()
	return info.initialized
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
		CACert:      info.caCert,
		EnvironUUID: info.environmentUUID,
	}
}

// SetBootstrapConfig implements EnvironInfo.SetBootstrapConfig.
func (info *environInfo) SetBootstrapConfig(attrs map[string]interface{}) {
	info.mu.Lock()
	defer info.mu.Unlock()
	if !info.created {
		panic("bootstrap config set on environment info that has not just been created")
	}
	info.bootstrapConfig = attrs
}

// SetAPIEndpoint implements EnvironInfo.SetAPIEndpoint.
func (info *environInfo) SetAPIEndpoint(endpoint APIEndpoint) {
	info.mu.Lock()
	defer info.mu.Unlock()
	info.apiEndpoints = endpoint.Addresses
	info.caCert = endpoint.CACert
	info.environmentUUID = endpoint.EnvironUUID
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
	lock, err := fslock.NewLock(info.environmentDir, lockName)
	if err != nil {
		return errors.Trace(err)
	}
	err = lock.LockWithTimeout(lockTimeout, "writing")
	if err != nil {
		return errors.Annotatef(err, "cannot write info")
	}
	defer lock.Unlock()

	if err := info.writeJENVFile(); err != nil {
		return errors.Trace(err)
	}

	info.initialized = true
	return nil
}

// Destroy implements EnvironInfo.Destroy.
func (info *environInfo) Destroy() error {
	info.mu.Lock()
	defer info.mu.Unlock()
	if info.initialized {
		err := os.Remove(info.path)
		if os.IsNotExist(err) {
			return errors.New("environment info has already been removed")
		}
		return err
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
	info.caCert = values.CACert
	info.apiEndpoints = values.StateServers
	info.bootstrapConfig = values.Config

	info.initialized = true
	return &info, nil
}

// Kept primarily for testing purposes now.
func (info *environInfo) writeJENVFile() error {

	infoData := EnvironInfoData{
		User:         info.user,
		Password:     info.credentials,
		EnvironUUID:  info.environmentUUID,
		StateServers: info.apiEndpoints,
		CACert:       info.caCert,
		Config:       info.bootstrapConfig,
	}

	data, err := goyaml.Marshal(infoData)
	if err != nil {
		return errors.Annotate(err, "cannot marshal environment info")
	}
	// We now use a fslock to sync reads and writes across the environment,
	// so we don't need to use a temporary file any more.

	flags := os.O_WRONLY
	if info.created {
		flags |= os.O_CREATE | os.O_EXCL
	} else {
		flags |= os.O_TRUNC
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
