// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudsigma

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"strings"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/environs/filestorage"
	"github.com/juju/juju/environs/httpstorage"
	"github.com/juju/juju/environs/sshstorage"
	"github.com/juju/juju/environs/storage"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/loggo"
	"github.com/juju/utils"
)

const (
	// storageSubdir is the subdirectory of
	// dataDir in which storage will be located.
	storageSubdir = "storage"

	// storageTmpSubdir is the subdirectory of
	// dataDir in which temporary storage will
	// be located.
	storageTmpSubdir = "storage-tmp"
)

type environStorage struct {
	storage storage.Storage
	uuid    string
	tmp     bool
}

var _ storage.Storage = (*environStorage)(nil)

var newStorage = func(ecfg *environConfig, client *environClient) (*environStorage, error) {
	var stg storage.Storage
	var err error
	var tmp bool

	uuid, ip, ok := client.stateServerAddress()
	if ok {
		logger.Debugf("active state server: %q", ip)
		// create https storage client
		stg, err = newRemoteStorage(ecfg, ip)
	} else {
		// create tmp storage
		tmp = true
		path := path.Join(osenv.JujuHome(), "tmp")

		logger.Debugf("prepare %q for tmp storage", path)
		logger.Debugf("  removing all...")
		if err := os.RemoveAll(path); err != nil {
			return nil, fmt.Errorf("create tmp storage (RemoveAll): %v", err)
		}

		logger.Debugf("  creating path...")
		if err := os.MkdirAll(path, 0755); err != nil {
			return nil, fmt.Errorf("create tmp storage (MkdirAll): %v", err)
		}
		stg, err = filestorage.NewFileStorageWriter(path)
		logger.Debugf("using local tmp storage at: %q", path)
	}

	if err != nil {
		return nil, err
	}

	result := &environStorage{
		storage: stg,
		uuid:    uuid,
		tmp:     tmp,
	}

	client.storage = result

	return result, nil
}

func newRemoteStorage(ecfg *environConfig, ip string) (storage.Storage, error) {
	caCertPEM, ok := ecfg.CACert()
	if !ok {
		// should not be possible to validate base config
		return nil, fmt.Errorf("ca-cert not set")
	}

	addr := fmt.Sprintf("%s:%d", ip, ecfg.storagePort())
	logger.Debugf("using https storage at: %q", addr)

	authkey := ecfg.storageAuthKey()
	stg, err := httpstorage.ClientTLS(addr, caCertPEM, authkey)
	if err != nil {
		return nil, fmt.Errorf("initializing HTTPS storage failed: %v", err)
	}

	return stg, nil
}

func (s *environStorage) onStateInstanceStop(uuid string) {
	if s.uuid != uuid {
		return
	}
	s.storage = nil
	s.uuid = ""
	s.tmp = false
}

func (s *environStorage) List(prefix string) ([]string, error) {
	if s.storage == nil {
		return nil, fmt.Errorf("storage is not initialized")
	}
	list, err := s.storage.List(prefix)
	logger.Tracef("environStorage.List, prefix = %s, len = %d, err = %v", prefix, len(list), err)
	if err == nil && logger.LogLevel() <= loggo.TRACE {
		for _, name := range list {
			logger.Tracef("...%q", name)
		}
	}
	return list, err
}

func (s *environStorage) URL(name string) (string, error) {
	if s.storage == nil {
		return "", fmt.Errorf("storage is not initialized")
	}
	if s.tmp {
		return "%s/" + name, nil
	}
	url, err := s.storage.URL(name)
	logger.Tracef("environStorage.URL, name = %q, url = %q, err = %v", name, url, err)
	return url, err
}

func (s *environStorage) Get(name string) (io.ReadCloser, error) {
	if s.storage == nil {
		return nil, fmt.Errorf("storage is not initialized")
	}
	r, err := s.storage.Get(name)
	logger.Tracef("environStorage.Get, name = %q, err = %v", name, err)
	return r, err
}

func (s *environStorage) Put(name string, r io.Reader, length int64) error {
	if s.storage == nil {
		return fmt.Errorf("storage is not initialized")
	}
	err := s.storage.Put(name, r, length)
	logger.Tracef("environStorage.Put, name = %q, len = %d, err = %v", name, length, err)
	return err
}

func (s *environStorage) Remove(name string) error {
	if s.storage == nil {
		return fmt.Errorf("storage is not initialized")
	}
	err := s.storage.Remove(name)
	logger.Tracef("environStorage.Remove, name = %q, err = %v", name, err)
	return err
}

func (s *environStorage) RemoveAll() error {
	if s.storage == nil {
		// this method is called after state server destroy at destroy-environment
		return nil
	}
	err := s.storage.RemoveAll()
	logger.Tracef("environStorage.RemoveAll, err = %v", err)
	return err
}

func (s *environStorage) DefaultConsistencyStrategy() utils.AttemptStrategy {
	if s.storage == nil {
		return utils.AttemptStrategy{}
	}
	return s.storage.DefaultConsistencyStrategy()
}

func (s *environStorage) ShouldRetry(err error) bool {
	if s.storage == nil {
		return false
	}
	return s.storage.ShouldRetry(err)
}

var newSSHStorage = func(sshurl, dir, tmpdir string) (storage.Storage, error) {
	return sshstorage.NewSSHStorage(sshstorage.NewSSHStorageParams{
		Host:       sshurl,
		StorageDir: dir,
		TmpDir:     tmpdir,
	})
}

func (s *environStorage) MoveToSSH(user, host string) error {
	if s.storage == nil {
		return fmt.Errorf("storage is not initialized")
	}

	sshurl := user + "@" + host
	if !s.tmp {
		return fmt.Errorf("failed to move non-temporary storage to %q", sshurl)
	}

	storageDir := path.Join(agent.DefaultDataDir, storageSubdir)
	storageTmpdir := path.Join(agent.DefaultDataDir, storageTmpSubdir)

	logger.Debugf("using ssh storage at host %q dir %q", sshurl, storageDir)
	stor, err := newSSHStorage(sshurl, storageDir, storageTmpdir)
	if err != nil {
		return fmt.Errorf("initializing SSH storage failed: %v", err)
	}

	list, err := s.List("")
	if err != nil {
		return fmt.Errorf("listing tmp storage failed: %v", err)
	}

	logger.Tracef("list to move:\n%s", strings.Join(list, "\n"))

	for _, path := range list {
		r, err := s.Get(path)
		if err != nil {
			return fmt.Errorf("getting %q from tmp storage failed: %v", path, err)
		}
		defer r.Close()

		bb, err := ioutil.ReadAll(r)
		if err != nil {
			return fmt.Errorf("error MoveToSSH: reading %q from ssh storage failed: %v", path, err)
		}

		rb := bytes.NewReader(bb)
		length := len(bb)
		if err := stor.Put(path, rb, int64(length)); err != nil {
			return fmt.Errorf("error MoveToSSH: putting %q to ssh storage failed: %v", path, err)
		}
	}

	s.storage.RemoveAll()

	s.storage = stor
	s.tmp = false

	return nil
}
