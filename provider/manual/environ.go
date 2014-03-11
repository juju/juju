// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"path"
	"strings"
	"sync"

	"github.com/juju/loggo"

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

var logger = loggo.GetLogger("juju.provider.manual")

type manualEnviron struct {
	cfg                 *environConfig
	cfgmutex            sync.Mutex
	storage             storage.Storage
	ubuntuUserInited    bool
	ubuntuUserInitMutex sync.Mutex
}

var _ envtools.SupportsCustomSources = (*manualEnviron)(nil)

var errNoStartInstance = errors.New("manual provider cannot start instances")
var errNoStopInstance = errors.New("manual provider cannot stop instances")

func (*manualEnviron) StartInstance(constraints.Value, tools.List, *cloudinit.MachineConfig) (instance.Instance, *instance.HardwareCharacteristics, error) {
	return nil, nil, errNoStartInstance
}

func (*manualEnviron) StopInstances([]instance.Instance) error {
	return errNoStopInstance
}

func (e *manualEnviron) AllInstances() ([]instance.Instance, error) {
	return e.Instances([]instance.Id{manual.BootstrapInstanceId})
}

func (e *manualEnviron) envConfig() (cfg *environConfig) {
	e.cfgmutex.Lock()
	cfg = e.cfg
	e.cfgmutex.Unlock()
	return cfg
}

func (e *manualEnviron) Config() *config.Config {
	return e.envConfig().Config
}

func (e *manualEnviron) Name() string {
	return e.envConfig().Name()
}

func (e *manualEnviron) Bootstrap(ctx environs.BootstrapContext, cons constraints.Value) error {
	// Set "use-sshstorage" to false, so agents know not to use sshstorage.
	cfg, err := e.Config().Apply(map[string]interface{}{"use-sshstorage": false})
	if err != nil {
		return err
	}
	if err := e.SetConfig(cfg); err != nil {
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

func (e *manualEnviron) StateInfo() (*state.Info, *api.Info, error) {
	return common.StateInfo(e)
}

func (e *manualEnviron) SetConfig(cfg *config.Config) error {
	e.cfgmutex.Lock()
	defer e.cfgmutex.Unlock()
	envConfig, err := manualProvider{}.validate(cfg, e.cfg.Config)
	if err != nil {
		return err
	}
	// Set storage. If "use-sshstorage" is true then use the SSH storage.
	// Otherwise, use HTTP storage.
	//
	// We don't change storage once it's been set. Storage parameters
	// are fixed at bootstrap time, and it is not possible to change
	// them.
	if e.storage == nil {
		var stor storage.Storage
		if envConfig.useSSHStorage() {
			storageDir := e.StorageDir()
			storageTmpdir := path.Join(dataDir, storageTmpSubdir)
			stor, err = newSSHStorage("ubuntu@"+e.cfg.bootstrapHost(), storageDir, storageTmpdir)
			if err != nil {
				return fmt.Errorf("initialising SSH storage failed: %v", err)
			}
		} else {
			caCertPEM, ok := envConfig.CACert()
			if !ok {
				// should not be possible to validate base config
				return fmt.Errorf("ca-cert not set")
			}
			authkey := envConfig.storageAuthKey()
			stor, err = httpstorage.ClientTLS(envConfig.storageAddr(), caCertPEM, authkey)
			if err != nil {
				return fmt.Errorf("initialising HTTPS storage failed: %v", err)
			}
		}
		e.storage = stor
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
func (e *manualEnviron) Instances(ids []instance.Id) (instances []instance.Instance, err error) {
	instances = make([]instance.Instance, len(ids))
	var found bool
	for i, id := range ids {
		if id == manual.BootstrapInstanceId {
			instances[i] = manualBootstrapInstance{e.envConfig().bootstrapHost()}
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

// GetToolsSources returns a list of sources which are
// used to search for simplestreams tools metadata.
func (e *manualEnviron) GetToolsSources() ([]simplestreams.DataSource, error) {
	// Add the simplestreams source off private storage.
	return []simplestreams.DataSource{
		storage.NewStorageSimpleStreamsDataSource("cloud storage", e.Storage(), storage.BaseToolsPath),
	}, nil
}

func (e *manualEnviron) Storage() storage.Storage {
	e.cfgmutex.Lock()
	defer e.cfgmutex.Unlock()
	return e.storage
}

var runSSHCommand = func(host string, command []string) (stderr string, err error) {
	cmd := ssh.Command(host, command, nil)
	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf
	err = cmd.Run()
	return stderrBuf.String(), err
}

func (e *manualEnviron) Destroy() error {
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

func (*manualEnviron) PrecheckInstance(series string, cons constraints.Value) error {
	return errors.New(`use "juju add-machine ssh:[user@]<host>" to provision machines`)
}

func (e *manualEnviron) OpenPorts(ports []instance.Port) error {
	return nil
}

func (e *manualEnviron) ClosePorts(ports []instance.Port) error {
	return nil
}

func (e *manualEnviron) Ports() ([]instance.Port, error) {
	return []instance.Port{}, nil
}

func (*manualEnviron) Provider() environs.EnvironProvider {
	return manualProvider{}
}

func (e *manualEnviron) StorageAddr() string {
	return e.envConfig().storageListenAddr()
}

func (e *manualEnviron) StorageDir() string {
	return path.Join(dataDir, storageSubdir)
}

func (e *manualEnviron) SharedStorageAddr() string {
	return ""
}

func (e *manualEnviron) SharedStorageDir() string {
	return ""
}

func (e *manualEnviron) StorageCACert() []byte {
	if bytes, ok := e.envConfig().CACert(); ok {
		return bytes
	}
	return nil
}

func (e *manualEnviron) StorageCAKey() []byte {
	if bytes, ok := e.envConfig().CAPrivateKey(); ok {
		return bytes
	}
	return nil
}

func (e *manualEnviron) StorageHostnames() []string {
	cfg := e.envConfig()
	hostnames := []string{cfg.bootstrapHost()}
	if ip := net.ParseIP(cfg.storageListenIPAddress()); ip != nil {
		if !ip.IsUnspecified() {
			hostnames = append(hostnames, ip.String())
		}
	}
	return hostnames
}

func (e *manualEnviron) StorageAuthKey() string {
	return e.envConfig().storageAuthKey()
}

var _ localstorage.LocalTLSStorageConfig = (*manualEnviron)(nil)
