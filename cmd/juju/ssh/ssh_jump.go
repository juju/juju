// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// This is the provider servicing the SSH jump implementation.
// The connection is transparently proxied to the target machine
// via the controller.

package ssh

import (
	"context"
	"net"
	"os"
	"strconv"

	"github.com/juju/errors"
	"github.com/juju/retry"
	"github.com/juju/utils/v4/ssh"
	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"

	"github.com/juju/juju/api/client/application"
	"github.com/juju/juju/api/client/client"
	"github.com/juju/juju/api/client/sshclient"
	controllerapi "github.com/juju/juju/api/controller/controller"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	jujussh "github.com/juju/juju/internal/network/ssh"
	"github.com/juju/juju/rpc/params"
)

// finalDestinationUser is the user used on the terminating target.
const finalDestinationUser = "ubuntu"

// SSHAPIJump is the SSH API client used by the SSH jump provider.
type SSHAPIJump interface {
	// VirtualHostname returns the virtual hostname for an SSH target.
	VirtualHostname(ctx context.Context, target string, container *string) (string, error)
	// PublicHostKeyForTarget returns the public host key for a virtual hostname.
	PublicHostKeyForTarget(ctx context.Context, virtualHostname string) (params.PublicSSHHostKeyResult, error)
	// Close releases resources associated with the SSH API client.
	Close() error
}

// sshJump implements the sshProvider interface by proxying the SSH connection
// to the target through the controller's embedded SSH jump server.
type sshJump struct {
	leaderResolver

	modelType            model.ModelType
	controllersAddresses []string
	container            string
	target               string
	args                 []string
	noHostKeyChecks      bool

	// jumpUser is the Juju user used to authenticate against the jump server.
	jumpUser string

	// jumpServerHostKey is the public host key of the jump server.
	jumpServerHostKey []byte

	// knownHostsPath is a temporary known_hosts file pinning the jump server
	// and target host keys.
	knownHostsPath string

	sshClient        SSHAPIJump
	controllerClient SSHControllerAPI
	hostChecker      jujussh.ReachableChecker

	jumpHostPort int
}

// initRun initializes the SSH jump provider for a model command.
func (p *sshJump) initRun(ctx context.Context, mc ModelCommand) error {
	if err := p.ensureAPIClient(ctx, mc); err != nil {
		return errors.Trace(err)
	}
	controllerConfig, err := p.controllerClient.ControllerConfig(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	p.jumpHostPort = controllerConfig.SSHServerPort()

	p.jumpServerHostKey, err = p.controllerClient.SSHServerHostKey(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	details, err := mc.ControllerDetails()
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

	_, modelDetails, err := mc.ModelDetails(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	p.modelType = modelDetails.ModelType

	account, err := mc.CurrentAccountDetails()
	if err != nil {
		return errors.Trace(err)
	}
	p.jumpUser = account.User
	return nil
}

// cleanupRun performs cleanup after the SSH jump run.
func (p *sshJump) cleanupRun() {
	if p.knownHostsPath != "" {
		_ = os.Remove(p.knownHostsPath)
		p.knownHostsPath = ""
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

// setLeaderAPI sets the leader API for the SSH jump provider.
func (p *sshJump) setLeaderAPI(_ context.Context, api LeaderAPI) {
	p.leaderAPI = api
}

// setHostChecker sets the host checker for the SSH jump provider.
func (p *sshJump) setHostChecker(checker jujussh.ReachableChecker) {
	if checker == nil {
		checker = defaultReachableChecker()
	}
	p.hostChecker = checker
}

// getTarget returns the current target of the SSH jump provider.
func (p *sshJump) getTarget() string {
	return p.target
}

// setTarget sets the target for the SSH jump provider.
func (p *sshJump) setTarget(target string) {
	p.target = target
}

// getArgs returns the arguments for the SSH jump provider.
func (p *sshJump) getArgs() []string {
	return p.args
}

// setArgs sets the arguments for the SSH jump provider.
func (p *sshJump) setArgs(args []string) {
	p.args = args
}

// resolveTarget resolves the target for the SSH jump provider into a virtual
// hostname to be reached through the controller jump server.
func (p *sshJump) resolveTarget(ctx context.Context, target string) (*resolvedTarget, error) {
	user, target := splitUserTarget(target)
	resolvedTargetName, err := p.maybeResolveLeaderUnit(ctx, target)
	if err != nil {
		return nil, errors.Trace(err)
	}
	var container *string
	if p.modelType == model.CAAS {
		if p.container == "" {
			logger.Debugf(ctx, "no container specified for SSH jump target; using %q", charmContainerName)
			tmpContainer := charmContainerName
			container = &tmpContainer
		} else {
			container = &p.container
		}
	}
	virtualHostname, err := p.sshClient.VirtualHostname(ctx, resolvedTargetName, container)
	if err != nil {
		return nil, errors.Trace(err)
	}
	targetKeys, err := p.getKeys(ctx, virtualHostname)
	if err != nil {
		return nil, errors.Trace(err)
	}
	// Probe the configured jump endpoints once to select a reachable endpoint
	// whose host key matches the controller key. The proxy command can then use
	// that verified endpoint directly.
	availableAddresses := network.NewMachineHostPorts(p.jumpHostPort, p.controllersAddresses...).HostPorts()
	jumpServerHostKey, err := gossh.ParsePublicKey(p.jumpServerHostKey)
	if err != nil {
		return nil, errors.Annotate(err, "parsing controller SSH server host key")
	}
	address, err := p.hostChecker.FindHost(availableAddresses, []string{string(gossh.MarshalAuthorizedKey(jumpServerHostKey))})
	if err != nil {
		return nil, errors.Trace(err)
	}
	if err := p.generateKnownHosts(address.Host(), virtualHostname, targetKeys.PublicKey); err != nil {
		return nil, errors.Trace(err)
	}
	resolved := &resolvedTarget{
		user: finalDestinationUser,
		host: virtualHostname,
		via: &resolvedTarget{
			user: p.jumpUser,
			host: address.Host(),
		},
	}
	if user != "" {
		resolved.user = user
	}
	return resolved, nil
}

// generateKnownHosts writes a temporary known_hosts file pinning the jump server
// host key (for the jump address and port) and the target host key (for the
// virtual hostname).
func (p *sshJump) generateKnownHosts(jumpHost, virtualHostname string, targetHostKey []byte) error {
	jumpAddress := knownhosts.Normalize(net.JoinHostPort(jumpHost, strconv.Itoa(p.jumpHostPort)))
	jumpLine, err := knownHostsLine(jumpAddress, p.jumpServerHostKey)
	if err != nil {
		return errors.Trace(err)
	}
	targetLine, err := knownHostsLine(virtualHostname, targetHostKey)
	if err != nil {
		return errors.Trace(err)
	}
	var f *os.File
	if p.knownHostsPath == "" {
		f, err = os.CreateTemp("", "ssh_known_hosts")
		if err != nil {
			return errors.Annotate(err, "creating known hosts file")
		}
		// This needs to be set here because it's used to cleanup the file.
		p.knownHostsPath = f.Name()
	} else {
		f, err = os.OpenFile(p.knownHostsPath, os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			return errors.Annotate(err, "opening known hosts file")
		}
	}
	defer f.Close()
	if _, err := f.WriteString(jumpLine + targetLine); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// knownHostsLine formats a known_hosts entry for the given host from a wire
// format public key.
func knownHostsLine(host string, wireKey []byte) (string, error) {
	pubKey, err := gossh.ParsePublicKey(wireKey)
	if err != nil {
		return "", errors.Annotatef(err, "parsing host key for %q", host)
	}
	// MarshalAuthorizedKey terminates each known_hosts entry with a newline.
	return host + " " + string(gossh.MarshalAuthorizedKey(pubKey)), nil
}

// getKeys retrieves the target host key.
func (p *sshJump) getKeys(ctx context.Context, virtualHostname string) (params.PublicSSHHostKeyResult, error) {
	hostKeys, err := p.sshClient.PublicHostKeyForTarget(ctx, virtualHostname)
	if err != nil {
		return params.PublicSSHHostKeyResult{}, errors.Annotatef(err, "retrieving SSH host key for %q", virtualHostname)
	}
	return hostKeys, nil
}

// maybePopulateTargetViaField is a no-op: the via field is set during target
// resolution.
func (p *sshJump) maybePopulateTargetViaField(_ context.Context, _ *resolvedTarget, _ func(context.Context, *client.StatusArgs) (*params.FullStatus, error)) error {
	return nil
}

// getSSHOptions returns SSH options for the jump server and target.
func (p *sshJump) getSSHOptions(enablePty bool, targets ...*resolvedTarget) (*ssh.Options, error) {
	if len(targets) == 0 {
		return nil, errors.New("at least one SSH target is required")
	}
	strictHostKeyChecking := "yes"
	knownHostsPath := p.knownHostsPath
	if p.noHostKeyChecks {
		strictHostKeyChecking = "no"
		knownHostsPath = os.DevNull
	}

	var options ssh.Options
	// -o ProxyCommand is a substitute for the -J option, due to a limitation
	// in the github.com/juju/utils/v4/ssh package.
	options.SetProxyCommand(
		"ssh",
		"-o", "StrictHostKeyChecking="+strictHostKeyChecking,
		"-o", "UserKnownHostsFile="+knownHostsPath,
		"-W", "%h:%p",
		"-p", strconv.Itoa(p.jumpHostPort),
		targets[0].via.userHost(),
	)
	if p.noHostKeyChecks {
		options.SetStrictHostKeyChecking(ssh.StrictHostChecksNo)
	} else {
		options.SetStrictHostKeyChecking(ssh.StrictHostChecksYes)
	}
	options.SetKnownHostsFile(knownHostsPath)
	if enablePty {
		options.EnablePTY()
	}
	return &options, nil
}

// ssh performs the SSH operation for the given target.
func (p *sshJump) ssh(ctx Context, enablePty bool, target *resolvedTarget) error {
	options, err := p.getSSHOptions(enablePty, target)
	if err != nil {
		return err
	}
	// Set the default command to "exec sh" if no arguments are provided and the
	// model type is CAAS.
	args := p.args
	if len(args) == 0 && p.modelType == model.CAAS {
		args = []string{"exec", "sh"}
	}
	cmd := ssh.Command(target.userHost(), args, options)
	cmd.Stdin = ctx.GetStdin()
	cmd.Stdout = ctx.GetStdout()
	cmd.Stderr = ctx.GetStderr()
	return cmd.Run()
}

// copy transfers files through the controller SSH jump server.
func (p *sshJump) copy(ctx Context) error {
	if p.modelType == model.CAAS {
		return errors.New("--jump is not supported for scp to Kubernetes targets")
	}

	args, targets, err := expandSCPArgs(ctx, p.args, p.resolveTarget)
	if err != nil {
		return errors.Trace(err)
	}
	options, err := p.getSSHOptions(false, targets...)
	if err != nil {
		return errors.Trace(err)
	}
	return ssh.Copy(args, options)
}

func (p *sshJump) setPublicKeyRetryStrategy(_ retry.CallArgs) {}

// setRetryStrategy is a no-op: the jump provider always dials the controller.
func (p *sshJump) setRetryStrategy(_ retry.CallArgs) {}

func (p *sshJump) ensureAPIClient(ctx context.Context, mc ModelCommand) error {
	if p.sshClient != nil && p.controllerClient != nil && p.leaderAPI != nil {
		return nil
	}
	conn, err := mc.NewAPIRoot(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	if p.leaderAPI == nil {
		p.leaderAPI = application.NewClient(conn)
	}
	if p.sshClient == nil {
		p.sshClient = sshclient.NewFacade(conn)
	}
	controllerConnection, err := mc.NewControllerAPIRoot(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	if p.controllerClient == nil {
		p.controllerClient = controllerapi.NewClient(controllerConnection)
	}
	return nil
}
