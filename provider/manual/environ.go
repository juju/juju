// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import (
	"bytes"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils"
	"github.com/juju/utils/arch"
	"github.com/juju/utils/featureflag"
	"github.com/juju/utils/ssh"
	"github.com/juju/version"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/manual"
	"github.com/juju/juju/environs/manual/sshprovisioner"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju/names"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/common"
)

const (
	// BootstrapInstanceId is the instance ID used
	// for the manual provider's bootstrap instance.
	BootstrapInstanceId instance.Id = "manual:"
)

var (
	logger = loggo.GetLogger("juju.provider.manual")
)

type manualEnviron struct {
	host string
	user string
	mu   sync.Mutex
	cfg  *environConfig
	// hw and series are detected by running a script on the
	// target machine. We cache these, as they should not change.
	hw     *instance.HardwareCharacteristics
	series string
}

var errNoStartInstance = errors.New("manual provider cannot start instances")
var errNoStopInstance = errors.New("manual provider cannot stop instances")

// MaintainInstance is specified in the InstanceBroker interface.
func (*manualEnviron) MaintainInstance(ctx context.ProviderCallContext, args environs.StartInstanceParams) error {
	return nil
}

func (*manualEnviron) StartInstance(ctx context.ProviderCallContext, args environs.StartInstanceParams) (*environs.StartInstanceResult, error) {
	return nil, errNoStartInstance
}

func (*manualEnviron) StopInstances(context.ProviderCallContext, ...instance.Id) error {
	return errNoStopInstance
}

func (e *manualEnviron) AllInstances(ctx context.ProviderCallContext) ([]instance.Instance, error) {
	return e.Instances(ctx, []instance.Id{BootstrapInstanceId})
}

func (e *manualEnviron) envConfig() (cfg *environConfig) {
	e.mu.Lock()
	cfg = e.cfg
	e.mu.Unlock()
	return cfg
}

func (e *manualEnviron) Config() *config.Config {
	return e.envConfig().Config
}

// PrepareForBootstrap is part of the Environ interface.
func (e *manualEnviron) PrepareForBootstrap(ctx environs.BootstrapContext) error {
	if err := ensureBootstrapUbuntuUser(ctx, e.host, e.user, e.envConfig()); err != nil {
		return err
	}
	return nil
}

// Create is part of the Environ interface.
func (e *manualEnviron) Create(context.ProviderCallContext, environs.CreateParams) error {
	return nil
}

// Bootstrap is part of the Environ interface.
func (e *manualEnviron) Bootstrap(ctx environs.BootstrapContext, callCtx context.ProviderCallContext, args environs.BootstrapParams) (*environs.BootstrapResult, error) {
	provisioned, err := sshprovisioner.CheckProvisioned(e.host)
	if err != nil {
		return nil, errors.Annotate(err, "failed to check provisioned status")
	}
	if provisioned {
		return nil, manual.ErrProvisioned
	}
	hw, series, err := e.seriesAndHardwareCharacteristics()
	if err != nil {
		return nil, err
	}
	finalize := func(ctx environs.BootstrapContext, icfg *instancecfg.InstanceConfig, _ environs.BootstrapDialOpts) error {
		icfg.Bootstrap.BootstrapMachineInstanceId = BootstrapInstanceId
		icfg.Bootstrap.BootstrapMachineHardwareCharacteristics = hw
		if err := instancecfg.FinishInstanceConfig(icfg, e.Config()); err != nil {
			return err
		}
		return common.ConfigureMachine(ctx, ssh.DefaultClient, e.host, icfg, nil)
	}

	result := &environs.BootstrapResult{
		Arch:                    *hw.Arch,
		Series:                  series,
		CloudBootstrapFinalizer: finalize,
	}
	return result, nil
}

// ControllerInstances is specified in the Environ interface.
func (e *manualEnviron) ControllerInstances(ctx context.ProviderCallContext, controllerUUID string) ([]instance.Id, error) {
	if !isRunningController() {
		// Not running inside the controller, so we must
		// verify the host.
		if err := e.verifyBootstrapHost(); err != nil {
			return nil, err
		}
	}
	return []instance.Id{BootstrapInstanceId}, nil
}

func (e *manualEnviron) verifyBootstrapHost() error {
	// First verify that the environment is bootstrapped by checking
	// if the agents directory exists. Note that we cannot test the
	// root data directory, as that is created in the process of
	// initialising sshstorage.
	agentsDir := path.Join(agent.DefaultPaths.DataDir, "agents")
	const noAgentDir = "no-agent-dir"
	stdin := fmt.Sprintf(
		"test -d %s || echo %s",
		utils.ShQuote(agentsDir),
		noAgentDir,
	)
	out, _, err := runSSHCommand(
		"ubuntu@"+e.host,
		[]string{"/bin/bash"},
		stdin,
	)
	if err != nil {
		return err
	}
	if out = strings.TrimSpace(out); len(out) > 0 {
		if out == noAgentDir {
			return environs.ErrNotBootstrapped
		}
		err := errors.Errorf("unexpected output: %q", out)
		logger.Infof(err.Error())
		return err
	}
	return nil
}

// AdoptResources is part of the Environ interface.
func (e *manualEnviron) AdoptResources(ctx context.ProviderCallContext, controllerUUID string, fromVersion version.Number) error {
	// This provider doesn't track instance -> controller.
	return nil
}

func (e *manualEnviron) SetConfig(cfg *config.Config) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	_, err := ManualProvider{}.validate(cfg, e.cfg.Config)
	if err != nil {
		return err
	}
	e.cfg = newModelConfig(cfg, cfg.UnknownAttrs())
	return nil
}

// Instances implements environs.Environ.
//
// This method will only ever return an Instance for the Id
// BootstrapInstanceId. If any others are specified, then
// ErrPartialInstances or ErrNoInstances will result.
func (e *manualEnviron) Instances(ctx context.ProviderCallContext, ids []instance.Id) (instances []instance.Instance, err error) {
	instances = make([]instance.Instance, len(ids))
	var found bool
	for i, id := range ids {
		if id == BootstrapInstanceId {
			instances[i] = manualBootstrapInstance{e.host}
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

var runSSHCommand = func(host string, command []string, stdin string) (stdout, stderr string, err error) {
	cmd := ssh.Command(host, command, nil)
	cmd.Stdin = strings.NewReader(stdin)
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf
	if err := cmd.Run(); err != nil {
		if stderr := strings.TrimSpace(stderrBuf.String()); len(stderr) > 0 {
			err = errors.Annotate(err, stderr)
		}
		return "", "", err
	}
	return stdoutBuf.String(), stderrBuf.String(), nil
}

// Destroy implements the Environ interface.
func (e *manualEnviron) Destroy(ctx context.ProviderCallContext) error {
	// There is nothing we can do for manual environments,
	// except when destroying the controller as a whole
	// (see DestroyController below).
	return nil
}

// DestroyController implements the Environ interface.
func (e *manualEnviron) DestroyController(ctx context.ProviderCallContext, controllerUUID string) error {
	script := `
# Signal the jujud process to stop, then check it has done so before cleaning-up
# after it.
set -x
touch %[1]s

stopped=0
function wait_for_jujud {
    for i in {1..30}; do
        if pgrep jujud > /dev/null ; then
            sleep 1
        else
            echo jujud stopped
            stopped=1
            logger --id jujud stopped on attempt $i
            break
        fi
    done
}

# There might be no jujud at all (for example, after a failed deployment) so
# don't require pkill to succeed before looking for a jujud process.
# SIGABRT not SIGTERM, as abort lets the worker know it should uninstall itself,
# rather than terminate normally.
pkill -SIGABRT jujud
wait_for_jujud

[[ $stopped -ne 1 ]] && {
    # If jujud didn't stop nicely, we kill it hard here.
    %[2]spkill -SIGKILL jujud && wait_for_jujud
}
[[ $stopped -ne 1 ]] && {
    echo jujud removal failed
    logger --id $(ps -o pid,cmd,state -p $(pgrep jujud) | awk 'NR != 1 {printf("Process %%d (%%s) has state %%s\n", $1, $2, $3)}')
    exit 1
}
service %[3]s stop && logger --id stopped %[3]s
apt-get -y purge juju-mongo*
apt-get -y autoremove
rm -f /etc/init/juju*
rm -f /etc/systemd/system{,/multi-user.target.wants}/juju*
rm -fr %[4]s %[5]s
exit 0
`
	var diagnostics string
	if featureflag.Enabled(feature.DeveloperMode) {
		diagnostics = `
    echo "Dump engine report and goroutines for stuck jujud"
    source /etc/profile.d/juju-introspection.sh
    juju-engine-report
    juju-goroutines
`
	}
	script = fmt.Sprintf(
		script,
		// WARNING: this is linked with the use of uninstallFile in
		// the agent package. Don't change it without extreme care,
		// and handling for mismatches with already-deployed agents.
		utils.ShQuote(path.Join(
			agent.DefaultPaths.DataDir,
			agent.UninstallFile,
		)),
		diagnostics,
		mongo.ServiceName,
		utils.ShQuote(agent.DefaultPaths.DataDir),
		utils.ShQuote(agent.DefaultPaths.LogDir),
	)
	logger.Tracef("destroy controller script: %s", script)
	stdout, stderr, err := runSSHCommand(
		"ubuntu@"+e.host,
		[]string{"sudo", "/bin/bash"}, script,
	)
	logger.Debugf("script stdout: \n%s", stdout)
	logger.Debugf("script stderr: \n%s", stderr)
	return err
}

func (*manualEnviron) PrecheckInstance(context.ProviderCallContext, environs.PrecheckInstanceParams) error {
	return errors.New(`use "juju add-machine ssh:[user@]<host>" to provision machines`)
}

var unsupportedConstraints = []string{
	constraints.CpuPower,
	constraints.InstanceType,
	constraints.Tags,
	constraints.VirtType,
}

// ConstraintsValidator is defined on the Environs interface.
func (e *manualEnviron) ConstraintsValidator(ctx context.ProviderCallContext) (constraints.Validator, error) {
	validator := constraints.NewValidator()
	validator.RegisterUnsupported(unsupportedConstraints)
	if isRunningController() {
		validator.UpdateVocabulary(constraints.Arch, []string{arch.HostArch()})
	} else {
		// We're running outside of the Juju controller, so we must
		// SSH to the machine and detect its architecture.
		hw, _, err := e.seriesAndHardwareCharacteristics()
		if err != nil {
			return nil, errors.Trace(err)
		}
		validator.UpdateVocabulary(constraints.Arch, []string{*hw.Arch})
	}
	return validator, nil
}

func (e *manualEnviron) seriesAndHardwareCharacteristics() (_ *instance.HardwareCharacteristics, series string, _ error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.hw != nil {
		return e.hw, e.series, nil
	}
	hw, series, err := sshprovisioner.DetectSeriesAndHardwareCharacteristics(e.host)
	if err != nil {
		return nil, "", errors.Trace(err)
	}
	e.hw, e.series = &hw, series
	return e.hw, e.series, nil
}

func (e *manualEnviron) OpenPorts(ctx context.ProviderCallContext, rules []network.IngressRule) error {
	return nil
}

func (e *manualEnviron) ClosePorts(ctx context.ProviderCallContext, rules []network.IngressRule) error {
	return nil
}

func (e *manualEnviron) IngressRules(ctx context.ProviderCallContext) ([]network.IngressRule, error) {
	return nil, nil
}

func (*manualEnviron) Provider() environs.EnvironProvider {
	return ManualProvider{}
}

func isRunningController() bool {
	return filepath.Base(os.Args[0]) == names.Jujud
}
