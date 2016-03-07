// Copyright 2013-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package configstore

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/retry"
	"github.com/juju/utils/clock"
	"github.com/juju/utils/fslock"

	"github.com/juju/juju/juju/osenv"
)

var logger = loggo.GetLogger("juju.environs.configstore")

type configSource string

const (
	lockName = "env.lock"

	sourceCreated configSource = "created"
	sourceCache   configSource = "cache"
	sourceMem     configSource = "mem"
)

// A second should be way more than enough to write or read any files. But
// some disks are very slow when under load, so lets give the disk a
// reasonable time to get the lock.
var lockTimeout = 5 * time.Second

// Default returns disk-based environment config storage
// rooted at JujuXDGDataHome.
var Default = func() (Storage, error) {
	return NewDisk(osenv.JujuXDGDataHome())
}

type diskStore struct {
	dir string
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
	serverUUID      string
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
		dir: dir,
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
func (d *diskStore) CreateInfo(serverUUID, envName string) EnvironInfo {
	return &environInfo{
		environmentDir: d.dir,
		source:         sourceCreated,
		name:           envName,
		serverUUID:     serverUUID,
	}
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
	defer unlockEnvironmentLock(lock)

	info, err := d.readCacheFile(envName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	info.environmentDir = d.dir
	return info, nil
}

func cacheFilename(dir string) string {
	return filepath.Join(dir, "controller-config.yaml")
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

// SetBootstrapConfig implements EnvironInfo.SetBootstrapConfig.
func (info *environInfo) SetBootstrapConfig(attrs map[string]interface{}) {
	info.mu.Lock()
	defer info.mu.Unlock()
	if info.source != sourceCreated {
		panic("bootstrap config set on model info that has not just been created")
	}
	info.bootstrapConfig = attrs
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
	defer unlockEnvironmentLock(lock)

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
	info.path = filename
	info.source = sourceCache
	return nil
}

// Destroy implements EnvironInfo.Destroy.
func (info *environInfo) Destroy() error {
	info.mu.Lock()
	defer info.mu.Unlock()
	lock, err := acquireEnvironmentLock(info.environmentDir, "destroying")
	if err != nil {
		return errors.Annotatef(err, "cannot destroy model info")
	}
	defer unlockEnvironmentLock(lock)

	if info.initialized() {
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
		return errors.Errorf("unknown source %q for model info", info.source)
	}
	return nil
}

func acquireEnvironmentLock(dir, operation string) (*fslock.Lock, error) {
	lock, err := fslock.NewLock(dir, lockName, fslock.Defaults())
	if err != nil {
		return nil, errors.Trace(err)
	}
	message := fmt.Sprintf("pid: %d, operation: %s", os.Getpid(), operation)
	err = lock.LockWithTimeout(lockTimeout, message)
	if err == nil {
		return lock, nil
	}
	if errors.Cause(err) != fslock.ErrTimeout {
		return nil, errors.Trace(err)
	}

	logger.Warningf("breaking configstore lock, lock dir: %s", filepath.Join(dir, lockName))
	logger.Warningf("  lock holder message: %s", lock.Message())

	// If we are unable to acquire the lock within the lockTimeout,
	// consider it broken for some reason, and break it.
	err = lock.BreakLock()
	if err != nil {
		return nil, errors.Annotate(err, "unable to break the configstore lock")
	}

	err = lock.LockWithTimeout(lockTimeout, message)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return lock, nil
}

// It appears that sometimes the lock is not cleared when we expect it to be.
// Capture and log any errors from the Unlock method and retry a few times.
func unlockEnvironmentLock(lock *fslock.Lock) {
	err := retry.Call(retry.CallArgs{
		Func: lock.Unlock,
		NotifyFunc: func(err error, attempt int) {
			logger.Debugf("failed to unlock configstore lock: %s", err)
		},
		Attempts: 10,
		Delay:    50 * time.Millisecond,
		Clock:    clock.WallClock,
	})
	if err != nil {
		logger.Errorf("unable to unlock configstore lock: %s", err)
	}
}
