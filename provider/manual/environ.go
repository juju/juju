// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import (
	"bytes"
	"fmt"
	"net"
	"path"
	"strings"
	"sync"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/httpstorage"
	"github.com/juju/juju/environs/manual"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/environs/sshstorage"
	"github.com/juju/juju/environs/storage"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju/arch"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/utils/ssh"
	"github.com/juju/juju/worker/localstorage"
	"github.com/juju/juju/worker/terminationworker"
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

var logger = loggo.GetLogger("juju.provider.manual")

type manualEnviron struct {
	common.SupportsUnitPlacementPolicy

	cfg                 *environConfig
	cfgmutex            sync.Mutex
	storage             storage.Storage
	ubuntuUserInited    bool
	ubuntuUserInitMutex sync.Mutex
}

var _ envtools.SupportsCustomSources = (*manualEnviron)(nil)

var errNoStartInstance = errors.New("manual provider cannot start instances")
var errNoStopInstance = errors.New("manual provider cannot stop instances")

func (*manualEnviron) StartInstance(args environs.StartInstanceParams) (instance.Instance, *instance.HardwareCharacteristics, []network.Info, error) {
	return nil, nil, nil, errNoStartInstance
}

func (*manualEnviron) StopInstances(...instance.Id) error {
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

// SupportedArchitectures is specified on the EnvironCapability interface.
func (e *manualEnviron) SupportedArchitectures() ([]string, error) {
	return arch.AllSupportedArches, nil
}

// SupportNetworks is specified on the EnvironCapability interface.
func (e *manualEnviron) SupportNetworks() bool {
	return false
}

func (e *manualEnviron) Bootstrap(ctx environs.BootstrapContext, args environs.BootstrapParams) ([]network.Address, error) {
	// Set "use-sshstorage" to false, so agents know not to use sshstorage.
	cfg, err := e.Config().Apply(map[string]interface{}{"use-sshstorage": false})
	if err != nil {
		return nil, err
	}
	if err := e.SetConfig(cfg); err != nil {
		return nil, err
	}
	envConfig := e.envConfig()
	// TODO(axw) consider how we can use placement to override bootstrap-host.
	host := envConfig.bootstrapHost()
	hc, series, err := manual.DetectSeriesAndHardwareCharacteristics(host)
	if err != nil {
		return nil, err
	}
	selectedTools, err := common.EnsureBootstrapTools(ctx, e, series, hc.Arch)
	if err != nil {
		return nil, err
	}
	return manual.Bootstrap(manual.BootstrapArgs{
		Context:                 ctx,
		Host:                    host,
		DataDir:                 agent.DefaultDataDir,
		Environ:                 e,
		PossibleTools:           selectedTools,
		Series:                  series,
		HardwareCharacteristics: &hc,
	})
}

// StateServerInstances is specified in the Environ interface.
func (e *manualEnviron) StateServerInstances() ([]instance.Id, error) {
	// First verify that the environment is bootstrapped by checking
	// if the agents directory exists. Note that we cannot test the
	// root data directory, as that is created in the process of
	// initialising sshstorage.
	agentsDir := path.Join(agent.DefaultDataDir, "agents")
	const noAgentDir = "no-agent-dir"
	stdin := fmt.Sprintf(
		"test -d %s || echo %s",
		utils.ShQuote(agentsDir),
		noAgentDir,
	)
	out, err := runSSHCommand(
		"ubuntu@"+e.cfg.bootstrapHost(),
		[]string{"/bin/bash"},
		stdin,
	)
	if err != nil {
		return nil, err
	}
	if out = strings.TrimSpace(out); len(out) > 0 {
		if out == noAgentDir {
			return nil, environs.ErrNotBootstrapped
		}
		return nil, errors.LoggedErrorf(logger, "unexpected output: %q", out)
	}
	return []instance.Id{manual.BootstrapInstanceId}, nil
}

func (e *manualEnviron) SetConfig(cfg *config.Config) error {
	e.cfgmutex.Lock()
	defer e.cfgmutex.Unlock()
	_, err := manualProvider{}.validate(cfg, e.cfg.Config)
	if err != nil {
		return err
	}
	envConfig := newEnvironConfig(cfg, cfg.UnknownAttrs())
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
			storageTmpdir := path.Join(agent.DefaultDataDir, storageTmpSubdir)
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

// AllocateAddress requests a new address to be allocated for the
// given instance on the given network. This is not supported on the
// manual provider.
func (*manualEnviron) AllocateAddress(_ instance.Id, _ network.Id) (network.Address, error) {
	return network.Address{}, errors.NotSupportedf("AllocateAddress")
}

// ListNetworks returns basic information about all networks known
// by the provider for the environment. They may be unknown to juju
// yet (i.e. when called initially or when a new network was created).
// This is not implemented by the manual provider yet.
func (*manualEnviron) ListNetworks() ([]network.BasicInfo, error) {
	return nil, errors.NotImplementedf("ListNetworks")
}

var newSSHStorage = func(sshHost, storageDir, storageTmpdir string) (storage.Storage, error) {
	logger.Debugf("using ssh storage at host %q dir %q", sshHost, storageDir)
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

var runSSHCommand = func(host string, command []string, stdin string) (stdout string, err error) {
	cmd := ssh.Command(host, command, nil)
	cmd.Stdin = strings.NewReader(stdin)
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf
	if err := cmd.Run(); err != nil {
		if stderr := strings.TrimSpace(stderrBuf.String()); len(stderr) > 0 {
			err = errors.Annotate(err, stderr)
		}
		return "", err
	}
	return stdoutBuf.String(), nil
}

func (e *manualEnviron) Destroy() error {
	script := `
set -x
pkill -%d jujud && exit
stop %s
rm -f /etc/init/juju*
rm -f /etc/rsyslog.d/*juju*
rm -fr %s %s
exit 0
`
	script = fmt.Sprintf(
		script,
		terminationworker.TerminationSignal,
		mongo.ServiceName(""),
		utils.ShQuote(agent.DefaultDataDir),
		utils.ShQuote(agent.DefaultLogDir),
	)
	_, err := runSSHCommand(
		"ubuntu@"+e.envConfig().bootstrapHost(),
		[]string{"sudo", "/bin/bash"}, script,
	)
	return err
}

func (*manualEnviron) PrecheckInstance(series string, _ constraints.Value, placement string) error {
	return errors.New(`use "juju add-machine ssh:[user@]<host>" to provision machines`)
}

var unsupportedConstraints = []string{
	constraints.CpuPower,
	constraints.InstanceType,
	constraints.Tags,
}

// ConstraintsValidator is defined on the Environs interface.
func (e *manualEnviron) ConstraintsValidator() (constraints.Validator, error) {
	validator := constraints.NewValidator()
	validator.RegisterUnsupported(unsupportedConstraints)
	return validator, nil
}

func (e *manualEnviron) OpenPorts(ports []network.Port) error {
	return nil
}

func (e *manualEnviron) ClosePorts(ports []network.Port) error {
	return nil
}

func (e *manualEnviron) Ports() ([]network.Port, error) {
	return []network.Port{}, nil
}

func (*manualEnviron) Provider() environs.EnvironProvider {
	return manualProvider{}
}

func (e *manualEnviron) StorageAddr() string {
	return e.envConfig().storageListenAddr()
}

func (e *manualEnviron) StorageDir() string {
	return path.Join(agent.DefaultDataDir, storageSubdir)
}

func (e *manualEnviron) SharedStorageAddr() string {
	return ""
}

func (e *manualEnviron) SharedStorageDir() string {
	return ""
}

func (e *manualEnviron) StorageCACert() string {
	if cert, ok := e.envConfig().CACert(); ok {
		return cert
	}
	return ""
}

func (e *manualEnviron) StorageCAKey() string {
	if key, ok := e.envConfig().CAPrivateKey(); ok {
		return key
	}
	return ""
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
