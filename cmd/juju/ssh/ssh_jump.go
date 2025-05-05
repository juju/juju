// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// This is the provider servicing the SSH jump implementation.
// the connection is transparently proxied to the target machine
// via the controller.

package ssh

import (
	"fmt"
	"net"
	"os"

	"github.com/gliderlabs/ssh"
	"github.com/juju/errors"
	"github.com/juju/retry"
	"github.com/juju/utils/v3"
	utilsssh "github.com/juju/utils/v3/ssh"
	gossh "golang.org/x/crypto/ssh"

	"github.com/juju/juju/api/client/application"
	"github.com/juju/juju/api/client/client"
	"github.com/juju/juju/api/client/sshclient"
	controllerapi "github.com/juju/juju/api/controller/controller"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	jujussh "github.com/juju/juju/network/ssh"
	"github.com/juju/juju/rpc/params"
)

const jumpUser = "admin"
const finalDestinationUser = "ubuntu"

type hostKeys struct {
	jumpHostKey     string
	jumpHostname    string
	virtualHostKey  string
	virtualHostname string
}

// SSHAPIJump is an interface for the SSH API client used in the SSH jump provider.
type SSHAPIJump interface {
	VirtualHostname(target string, container *string) (string, error)
	PublicHostKeyForTarget(virtualHostname string) (params.PublicSSHHostKeyResult, error)
	Close() error
}

// sshJump implements the sshProvider interface.
type sshJump struct {
	leaderResolver

	modelType              model.ModelType
	controllersAddresses   []string
	container              string
	target                 string
	args                   []string
	tempKnownHostsPath     string
	sshClient              SSHAPIJump
	controllerClient       SSHControllerAPI
	hostChecker            jujussh.ReachableChecker
	publicKeyRetryStrategy retry.CallArgs
	jumpHostPort           int
}

// initRun initializes the SSH proxy for a model command.
func (p *sshJump) initRun(cmd ModelCommand) error {
	if err := p.ensureAPIClient(cmd); err != nil {
		return errors.Trace(err)
	}
	controllerConfig, err := p.controllerClient.ControllerConfig()
	if err != nil {
		return errors.Trace(err)
	}
	p.jumpHostPort = controllerConfig.SSHServerPort()

	details, err := cmd.ControllerDetails()
	if err != nil {
		return errors.Trace(err)
	}
	for _, detail := range details.APIEndpoints {
		host, _, err := net.SplitHostPort(detail)
		if err != nil {
			continue
		}
		p.controllersAddresses = append(p.controllersAddresses, host)
	}
	_, modelDetails, err := cmd.ModelDetails()
	if err != nil {
		return errors.Trace(err)
	}
	p.modelType = modelDetails.ModelType
	return nil
}

// cleanupRun performs cleanup after the SSH proxy run.
func (p *sshJump) cleanupRun() {
	if p.tempKnownHostsPath != "" {
		_ = os.Remove(p.tempKnownHostsPath)
		p.tempKnownHostsPath = ""
	}
	if p.sshClient != nil {
		_ = p.sshClient.Close()
		p.sshClient = nil
	}
	if p.leaderAPI != nil {
		_ = p.leaderAPI.Close()
		p.leaderAPI = nil
	}
}

// setLeaderAPI sets the leader API for the SSH proxy.
func (p *sshJump) setLeaderAPI(api LeaderAPI) {
	p.leaderAPI = api
}

// setHostChecker sets the host checker for the SSH proxy.
func (p *sshJump) setHostChecker(checker jujussh.ReachableChecker) {
	if checker == nil {
		checker = defaultReachableChecker()
	}
	p.hostChecker = checker
}

// getTarget returns the current target of the SSH proxy.
func (p *sshJump) getTarget() string {
	return p.target
}

// resolveTarget resolves the target for the SSH proxy.
func (p *sshJump) resolveTarget(target string) (*resolvedTarget, error) {
	user, entity := splitUserTarget(target)
	if user == "" {
		user = jumpUser
	}
	resolvedTargetName, err := p.maybeResolveLeaderUnit(entity)
	if err != nil {
		return nil, errors.Trace(err)
	}
	var container *string
	if p.modelType == model.CAAS {
		if p.container == "" {
			tmpContainer := charmContainerName
			container = &tmpContainer
		} else {
			container = &p.container
		}
	}
	virtualHostname, err := p.sshClient.VirtualHostname(resolvedTargetName, container)
	if err != nil {
		return nil, errors.Trace(err)
	}
	jumpHostKey, virtualHostKey, err := p.getKeysWithRetry(virtualHostname)
	if err != nil {
		return nil, errors.Trace(err)
	}
	availableAddresses := network.NewMachineHostPorts(p.jumpHostPort, p.controllersAddresses...).HostPorts()
	address, err := p.hostChecker.FindHost(availableAddresses, []string{jumpHostKey})
	if err != nil {
		return nil, errors.Trace(err)
	}
	err = p.generateTemporaryKnownHosts(
		hostKeys{
			jumpHostKey:     jumpHostKey,
			jumpHostname:    address.Host(),
			virtualHostKey:  virtualHostKey,
			virtualHostname: virtualHostname,
		},
	)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &resolvedTarget{
		user: finalDestinationUser,
		host: virtualHostname,
		via: &resolvedTarget{
			user: user,
			host: address.Host(),
		},
	}, nil
}

// generateTemporaryKnownHosts generates a temporary known hosts file for the SSH jump.
// TODO(alek8): The current solution does not allow users to specify their own knownhosts lines manually.
func (p *sshJump) generateTemporaryKnownHosts(hostKeys hostKeys) error {
	knownHosts := newKnownHostsBuilder()

	knownHosts.add(fmt.Sprintf("[%s]:%d", hostKeys.jumpHostname, p.jumpHostPort), []string{hostKeys.jumpHostKey})
	knownHosts.add(hostKeys.virtualHostname, []string{hostKeys.virtualHostKey})

	f, err := os.CreateTemp("", "ssh_known_hosts")
	if err != nil {
		return errors.Annotate(err, "creating known hosts file")
	}
	defer func() { _ = f.Close() }()
	p.tempKnownHostsPath = f.Name() // Record for later deletion
	if err := knownHosts.write(f); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// getKeysWithRetry retrieves the public SSH host keys for the jump and target server, and converts them
// to the SSH authrorized key format.
// The reason we need a retry strategy is because the machine might not have been provisioned yet.
func (p *sshJump) getKeysWithRetry(virtualHostname string) (string, string, error) {
	var hostKeysResult params.PublicSSHHostKeyResult
	strategy := p.publicKeyRetryStrategy
	strategy.IsFatalError = func(err error) bool {
		return !errors.Is(err, errors.NotFound)
	}
	strategy.Func = func() error {
		hostKeys, err := p.sshClient.PublicHostKeyForTarget(virtualHostname)
		if err != nil {
			return errors.Annotatef(err, "retrieving SSH host keys for %q", virtualHostname)
		}
		hostKeysResult = hostKeys
		return nil
	}
	err := retry.Call(strategy)
	if err != nil {
		return "", "", err
	}
	jumpPublicKey, err := ssh.ParsePublicKey(hostKeysResult.JumpServerPublicKey)
	if err != nil {
		return "", "", errors.Annotate(err, "parsing jump server public key")
	}
	publicKeyVirtualHostname, err := ssh.ParsePublicKey(hostKeysResult.PublicKey)
	if err != nil {
		return "", "", errors.Annotate(err, "parsing virtual hostname public key")
	}
	return string(gossh.MarshalAuthorizedKey(jumpPublicKey)), string(gossh.MarshalAuthorizedKey(publicKeyVirtualHostname)), nil
}

// maybePopulateTargetViaField is here to satisfy the interface.
// It is not implemented for the SSH jump provider.
func (p *sshJump) maybePopulateTargetViaField(target *resolvedTarget, fetchStatus func(*client.StatusArgs) (*params.FullStatus, error)) error {
	return errors.Errorf("not implemented for ssh jump provider.")
}

func (p *sshJump) getSSHOptions(pty bool, targets ...*resolvedTarget) (*utilsssh.Options, error) {
	var options utilsssh.Options
	if pty {
		options.EnablePTY()
	}
	// -o ProxyCommand is a substitute for the -J option.
	// Due to a limitation in the github.com/juju/utils/v3/ssh pkg.
	options.SetProxyCommand(
		"ssh",
		"-W",
		"%h:%p",
		"-p",
		fmt.Sprint(p.jumpHostPort),
		"-o",
		"UserKnownHostsFile "+utils.CommandString(p.tempKnownHostsPath),
		fmt.Sprintf("%s@%s", targets[0].via.user, targets[0].via.host),
	)
	options.SetStrictHostKeyChecking(utilsssh.StrictHostChecksYes)
	options.SetKnownHostsFile(p.tempKnownHostsPath)
	return &options, nil
}

// ssh performs the SSH operation for the given target.
func (p *sshJump) ssh(ctx Context, enablePty bool, target *resolvedTarget) error {
	options, err := p.getSSHOptions(enablePty, target)
	if err != nil {
		return err
	}
	// set the default command to "exec sh" if no arguments are provided
	// and the model type is CAAS.
	if len(p.args) == 0 && p.modelType == model.CAAS {
		p.args = []string{"exec", "sh"}
	}
	cmd := utilsssh.Command(target.userHost(), p.args, options)
	cmd.Stdin = ctx.GetStdin()
	cmd.Stdout = ctx.GetStdout()
	cmd.Stderr = ctx.GetStderr()
	return cmd.Run()
}

// copy performs a copy operation using the SSH proxy.
func (p *sshJump) copy(ctx Context) error {
	// Perform the copy operation.
	return errors.NotImplemented
}

// setTarget sets the target for the SSH proxy.
func (p *sshJump) setTarget(target string) {
	p.target = target
}

// getArgs returns the arguments for the SSH proxy.
func (p *sshJump) getArgs() []string {
	return p.args
}

// setArgs sets the arguments for the SSH proxy.
func (p *sshJump) setArgs(args []string) {
	p.args = args
}

func (p *sshJump) setPublicKeyRetryStrategy(retryStrategy retry.CallArgs) {
	p.publicKeyRetryStrategy = retryStrategy
}

// setRetryStrategy is just here to satisfy the interface. The retry strategy was needed
// when connecting to machine because they might not have a provisioned IP at the moment
// we were trying to connect to them.
// We don't carry the same issue because we always dial the controller.
func (p *sshJump) setRetryStrategy(strategy retry.CallArgs) {}

func (p *sshJump) ensureAPIClient(mc ModelCommand) error {
	if p.sshClient != nil && p.controllerClient != nil && p.leaderAPI != nil {
		return nil
	}
	conn, err := mc.NewAPIRoot()
	if err != nil {
		return errors.Trace(err)
	}
	if p.leaderAPI == nil {
		p.leaderAPI = application.NewClient(conn)
	}
	if p.sshClient == nil {
		p.sshClient = sshclient.NewFacade(conn)
	}
	controllerConnection, err := mc.NewControllerAPIRoot()
	if err != nil {
		return errors.Trace(err)
	}
	if p.controllerClient == nil {
		p.controllerClient = controllerapi.NewClient(controllerConnection)
	}
	return nil
}
