// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package null

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"path"
	"strings"
	"sync"

	"launchpad.net/loggo"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/cloudinit"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/httpstorage"
	"launchpad.net/juju-core/environs/manual"
	"launchpad.net/juju-core/environs/simplestreams"
	"launchpad.net/juju-core/environs/sshstorage"
	"launchpad.net/juju-core/environs/storage"
	envtools "launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/provider/common"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/tools"
	"launchpad.net/juju-core/utils/ssh"
	"launchpad.net/juju-core/worker/localstorage"
	"launchpad.net/juju-core/worker/terminationworker"
)

const (
	// TODO(axw) make this configurable?
	dataDir = "/var/lib/juju"

	// storageSubdir is the subdirectory of
	// dataDir in which storage will be located.
	storageSubdir = "storage"

	// storageTmpSubdir is the subdirectory of
	// dataDir in which temporary storage will
	// be located.
	storageTmpSubdir = "storage-tmp"
)

var logger = loggo.GetLogger("juju.provider.null")

type nullEnviron struct {
	cfg                   *environConfig
	cfgmutex              sync.Mutex
	bootstrapStorage      storage.Storage
	bootstrapStorageMutex sync.Mutex
	ubuntuUserInited      bool
	ubuntuUserInitMutex   sync.Mutex
}

var _ environs.BootstrapStorager = (*nullEnviron)(nil)
var _ envtools.SupportsCustomSources = (*nullEnviron)(nil)

var errNoStartInstance = errors.New("null provider cannot start instances")
var errNoStopInstance = errors.New("null provider cannot stop instances")

func (*nullEnviron) StartInstance(constraints.Value, tools.List, *cloudinit.MachineConfig) (instance.Instance, *instance.HardwareCharacteristics, error) {
	return nil, nil, errNoStartInstance
}

func (*nullEnviron) StopInstances([]instance.Instance) error {
	return errNoStopInstance
}

func (e *nullEnviron) AllInstances() ([]instance.Instance, error) {
	return e.Instances([]instance.Id{manual.BootstrapInstanceId})
}

func (e *nullEnviron) envConfig() (cfg *environConfig) {
	e.cfgmutex.Lock()
	cfg = e.cfg
	e.cfgmutex.Unlock()
	return cfg
}

func (e *nullEnviron) Config() *config.Config {
	return e.envConfig().Config
}

func (e *nullEnviron) Name() string {
	return e.envConfig().Name()
}

var initUbuntuUser = manual.InitUbuntuUser

func (e *nullEnviron) ensureBootstrapUbuntuUser(ctx environs.BootstrapContext) error {
	e.ubuntuUserInitMutex.Lock()
	defer e.ubuntuUserInitMutex.Unlock()
	if e.ubuntuUserInited {
		return nil
	}
	cfg := e.envConfig()
	err := initUbuntuUser(cfg.bootstrapHost(), cfg.bootstrapUser(), cfg.AuthorizedKeys(), ctx.Stdin(), ctx.Stdout())
	if err != nil {
		logger.Errorf("initializing ubuntu user: %v", err)
		return err
	}
	logger.Infof("initialized ubuntu user")
	e.ubuntuUserInited = true
	return nil
}

func (e *nullEnviron) Bootstrap(ctx environs.BootstrapContext, cons constraints.Value) error {
	if err := e.ensureBootstrapUbuntuUser(ctx); err != nil {
		return err
	}
	envConfig := e.envConfig()
	host := envConfig.bootstrapHost()
	hc, series, err := manual.DetectSeriesAndHardwareCharacteristics(host)
	if err != nil {
		return err
	}
	selectedTools, err := common.EnsureBootstrapTools(e, series, hc.Arch)
	if err != nil {
		return err
	}
	return manual.Bootstrap(manual.BootstrapArgs{
		Context:                 ctx,
		Host:                    host,
		DataDir:                 dataDir,
		Environ:                 e,
		PossibleTools:           selectedTools,
		Series:                  series,
		HardwareCharacteristics: &hc,
	})
}

func (e *nullEnviron) StateInfo() (*state.Info, *api.Info, error) {
	return common.StateInfo(e)
}

func (e *nullEnviron) SetConfig(cfg *config.Config) error {
	e.cfgmutex.Lock()
	defer e.cfgmutex.Unlock()
	envConfig, err := nullProvider{}.validate(cfg, e.cfg.Config)
	if err != nil {
		return err
	}
	e.cfg = envConfig
	return nil
}

// Implements environs.Environ.
//
// This method will only ever return an Instance for the Id
// environ/manual.BootstrapInstanceId. If any others are
// specified, then ErrPartialInstances or ErrNoInstances
// will result.
func (e *nullEnviron) Instances(ids []instance.Id) (instances []instance.Instance, err error) {
	instances = make([]instance.Instance, len(ids))
	var found bool
	for i, id := range ids {
		if id == manual.BootstrapInstanceId {
			instances[i] = nullBootstrapInstance{e.envConfig().bootstrapHost()}
			found = true
		} else {
			err = environs.ErrPartialInstances
		}
	}
	if !found {
		err = environs.ErrNoInstances
	}
	return instances, err
}

var newSSHStorage = func(sshHost, storageDir, storageTmpdir string) (storage.Storage, error) {
	return sshstorage.NewSSHStorage(sshstorage.NewSSHStorageParams{
		Host:       sshHost,
		StorageDir: storageDir,
		TmpDir:     storageTmpdir,
	})
}

// Implements environs.BootstrapStorager.
func (e *nullEnviron) EnableBootstrapStorage(ctx environs.BootstrapContext) error {
	e.bootstrapStorageMutex.Lock()
	defer e.bootstrapStorageMutex.Unlock()
	if e.bootstrapStorage != nil {
		return nil
	}
	if err := e.ensureBootstrapUbuntuUser(ctx); err != nil {
		return err
	}
	cfg := e.envConfig()
	storageDir := e.StorageDir()
	storageTmpdir := path.Join(dataDir, storageTmpSubdir)
	bootstrapStorage, err := newSSHStorage("ubuntu@"+cfg.bootstrapHost(), storageDir, storageTmpdir)
	if err != nil {
		return err
	}
	e.bootstrapStorage = bootstrapStorage
	return nil
}

// GetToolsSources returns a list of sources which are
// used to search for simplestreams tools metadata.
func (e *nullEnviron) GetToolsSources() ([]simplestreams.DataSource, error) {
	// Add the simplestreams source off private storage.
	return []simplestreams.DataSource{
		storage.NewStorageSimpleStreamsDataSource(e.Storage(), storage.BaseToolsPath),
	}, nil
}

func (e *nullEnviron) Storage() storage.Storage {
	e.bootstrapStorageMutex.Lock()
	defer e.bootstrapStorageMutex.Unlock()
	if e.bootstrapStorage != nil {
		return e.bootstrapStorage
	}
	caCertPEM, authkey := e.StorageCACert(), e.StorageAuthKey()
	if caCertPEM != nil && authkey != "" {
		storage, err := httpstorage.ClientTLS(e.envConfig().storageAddr(), caCertPEM, authkey)
		if err != nil {
			// Should be impossible, since ca-cert will always be validated.
			logger.Errorf("initialising HTTPS storage failed: %v", err)
		} else {
			return storage
		}
	} else {
		logger.Errorf("missing CA cert or auth-key")
	}
	return nil
}

var runSSHCommand = func(host string, command []string) (stderr string, err error) {
	cmd := ssh.Command(host, command, nil)
	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf
	err = cmd.Run()
	return stderrBuf.String(), err
}

func (e *nullEnviron) Destroy() error {
	stderr, err := runSSHCommand(
		"ubuntu@"+e.envConfig().bootstrapHost(),
		[]string{"sudo", "pkill", fmt.Sprintf("-%d", terminationworker.TerminationSignal), "jujud"},
	)
	if err != nil {
		if stderr := strings.TrimSpace(stderr); len(stderr) > 0 {
			err = fmt.Errorf("%v (%v)", err, stderr)
		}
	}
	return err
}

func (e *nullEnviron) OpenPorts(ports []instance.Port) error {
	return nil
}

func (e *nullEnviron) ClosePorts(ports []instance.Port) error {
	return nil
}

func (e *nullEnviron) Ports() ([]instance.Port, error) {
	return []instance.Port{}, nil
}

func (*nullEnviron) Provider() environs.EnvironProvider {
	return nullProvider{}
}

func (e *nullEnviron) StorageAddr() string {
	return e.envConfig().storageListenAddr()
}

func (e *nullEnviron) StorageDir() string {
	return path.Join(dataDir, storageSubdir)
}

func (e *nullEnviron) SharedStorageAddr() string {
	return ""
}

func (e *nullEnviron) SharedStorageDir() string {
	return ""
}

func (e *nullEnviron) StorageCACert() []byte {
	if bytes, ok := e.envConfig().CACert(); ok {
		return bytes
	}
	return nil
}

func (e *nullEnviron) StorageCAKey() []byte {
	if bytes, ok := e.envConfig().CAPrivateKey(); ok {
		return bytes
	}
	return nil
}

func (e *nullEnviron) StorageHostnames() []string {
	cfg := e.envConfig()
	hostnames := []string{cfg.bootstrapHost()}
	if ip := net.ParseIP(cfg.storageListenIPAddress()); ip != nil {
		if !ip.IsUnspecified() {
			hostnames = append(hostnames, ip.String())
		}
	}
	return hostnames
}

func (e *nullEnviron) StorageAuthKey() string {
	return e.envConfig().storageAuthKey()
}

var _ localstorage.LocalTLSStorageConfig = (*nullEnviron)(nil)
