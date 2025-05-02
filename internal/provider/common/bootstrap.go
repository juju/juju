// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"bufio"
	"context"
	stderrors "errors"
	"fmt"
	"io"
	"net"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils/v4"
	"github.com/juju/utils/v4/parallel"
	"github.com/juju/utils/v4/shell"
	"github.com/juju/utils/v4/ssh"

	"github.com/juju/juju/controller"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/network/firewall"
	"github.com/juju/juju/core/status"
	domainstorage "github.com/juju/juju/domain/storage"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/environs/models"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/internal/cloudconfig"
	"github.com/juju/juju/internal/cloudconfig/cloudinit"
	"github.com/juju/juju/internal/cloudconfig/instancecfg"
	"github.com/juju/juju/internal/cloudconfig/sshinit"
	internallogger "github.com/juju/juju/internal/logger"
	pkissh "github.com/juju/juju/internal/pki/ssh"
	jujussh "github.com/juju/juju/internal/ssh"
	"github.com/juju/juju/internal/storage"
	coretools "github.com/juju/juju/internal/tools"
)

var logger = internallogger.GetLogger("juju.provider.common")

// Bootstrap is a common implementation of the Bootstrap method defined on
// environs.Environ; we strongly recommend that this implementation be used
// when writing a new provider.
func Bootstrap(
	ctx environs.BootstrapContext,
	env environs.Environ,
	args environs.BootstrapParams,
) (*environs.BootstrapResult, error) {
	result, base, finalizer, err := BootstrapInstance(ctx, env, args)
	if err != nil {
		return nil, errors.Trace(err)
	}

	bsResult := &environs.BootstrapResult{
		Arch:                    *result.Hardware.Arch,
		Base:                    *base,
		CloudBootstrapFinalizer: finalizer,
	}
	return bsResult, nil
}

// BootstrapInstance creates a new instance with the series of its choice,
// constrained to those of the available tools, and
// returns the instance result, series, and a function that
// must be called to finalize the bootstrap process by transferring
// the tools and installing the initial Juju controller.
// This method is called by Bootstrap above, which implements environs.Bootstrap, but
// is also exported so that providers can manipulate the started instance.
func BootstrapInstance(
	bootstrapContext environs.BootstrapContext,
	env environs.Environ,
	args environs.BootstrapParams,
) (_ *environs.StartInstanceResult, resultBase *corebase.Base, _ environs.CloudBootstrapFinalizer, err error) {
	// TODO make safe in the case of racing Bootstraps
	// If two Bootstraps are called concurrently, there's
	// no way to make sure that only one succeeds.

	// First thing, ensure we have tools otherwise there's no point.
	requestedBootstrapBase, err := corebase.ValidateBase(
		args.SupportedBootstrapBases,
		args.BootstrapBase,
		config.PreferredBase(env.Config()),
	)
	if !args.Force && err != nil {
		// If the base isn't valid (i.e. non-ubuntu) then don't prompt users to use
		// the --force flag.
		if requestedBootstrapBase.OS != corebase.UbuntuOS {
			return nil, nil, nil, errors.NotValidf("non-ubuntu bootstrap base %q", requestedBootstrapBase.String())
		}
		return nil, nil, nil, errors.Annotatef(err, "use --force to override")
	}
	// The base we're attempting to bootstrap is empty, show a friendly
	// error message, rather than the more cryptic error messages that follow
	// onwards.
	if requestedBootstrapBase.Empty() {
		return nil, nil, nil, errors.NotValidf("bootstrap instance base")
	}
	availableTools, err := args.AvailableTools.Match(coretools.Filter{
		OSType: requestedBootstrapBase.OS,
	})
	if err != nil {
		return nil, nil, nil, err
	}

	// Filter image metadata to the selected base.
	var imageMetadata []*imagemetadata.ImageMetadata
	for _, m := range args.ImageMetadata {
		if m.Version != requestedBootstrapBase.Channel.Track {
			continue
		}
		imageMetadata = append(imageMetadata, m)
	}

	// Get the bootstrap SSH client. Do this early, so we know
	// not to bother with any of the below if we can't finish the job.
	client := ssh.DefaultClient
	if client == nil {
		// This should never happen: if we don't have OpenSSH, then
		// go.crypto/ssh should be used with an auto-generated key.
		return nil, nil, nil, fmt.Errorf("no SSH client available")
	}

	publicKey, err := simplestreams.UserPublicSigningKey()
	if err != nil {
		return nil, nil, nil, err
	}

	instanceConfig, err := instancecfg.NewBootstrapInstanceConfig(
		args.ControllerConfig, args.BootstrapConstraints, args.ModelConstraints, requestedBootstrapBase, publicKey,
		args.ExtraAgentValuesForTesting,
	)
	if err != nil {
		return nil, nil, nil, err
	}

	ak := jujussh.MakeAuthorizedKeysString(args.AuthorizedKeys)
	instanceConfig.AuthorizedKeys = ak

	envCfg := env.Config()
	instanceConfig.EnableOSRefreshUpdate = envCfg.EnableOSRefreshUpdate()
	instanceConfig.EnableOSUpgrade = envCfg.EnableOSUpgrade()
	instanceConfig.NetBondReconfigureDelay = envCfg.NetBondReconfigureDelay()
	instanceConfig.Tags = instancecfg.InstanceTags(envCfg.UUID(), args.ControllerConfig.ControllerUUID(), envCfg, true)

	// We're creating a new instance; inject host keys so that we can then
	// make an SSH connection with known keys.
	initialSSHHostKeys, err := generateSSHHostKeys()
	if err != nil {
		return nil, nil, nil, errors.Annotate(err, "generating SSH host keys")
	}
	instanceConfig.Bootstrap.InitialSSHHostKeys = initialSSHHostKeys

	cloudRegion := args.CloudName
	if args.CloudRegion != "" {
		cloudRegion += "/" + args.CloudRegion
	}
	bootstrapContext.Infof("Launching controller instance(s) on %s...", cloudRegion)
	// Print instance status reports status changes during provisioning.
	// Note the carriage returns, meaning subsequent prints are to the same
	// line of stderr, not a new line.
	lastLength := 0
	statusCleanedUp := false
	instanceStatus := func(ctx context.Context, settableStatus status.Status, info string, data map[string]interface{}) error {
		// The data arg is not expected to be used in this case, but
		// print it, rather than ignore it, if we get something.
		dataString := ""
		if len(data) > 0 {
			dataString = fmt.Sprintf(" %v", data)
		}
		length := len(info) + len(dataString)
		padding := ""
		if lastLength > length {
			padding = strings.Repeat(" ", lastLength-length)
		}
		lastLength = length
		statusCleanedUp = false
		fmt.Fprintf(bootstrapContext.GetStderr(), " - %s%s%s\r", info, dataString, padding)
		return nil
	}
	// Likely used after the final instanceStatus call to white-out the
	// current stderr line before the next use, removing any residual status
	// reporting output.
	statusCleanup := func() error {
		if statusCleanedUp {
			return nil
		}
		statusCleanedUp = true
		// The leading spaces account for the leading characters
		// emitted by instanceStatus above.
		padding := strings.Repeat(" ", lastLength)
		fmt.Fprintf(bootstrapContext.GetStderr(), "   %s\r", padding)
		return nil
	}

	var startInstanceArgs = environs.StartInstanceParams{
		ControllerUUID:  args.ControllerConfig.ControllerUUID(),
		Constraints:     args.BootstrapConstraints,
		Tools:           availableTools,
		InstanceConfig:  instanceConfig,
		Placement:       args.Placement,
		ImageMetadata:   imageMetadata,
		StatusCallback:  instanceStatus,
		CleanupCallback: statusCleanup,
	}

	// If a root disk constraint is specified, see if it matches any
	// storage pools scheduled to be added to the controller model and
	// set up the root disk accordingly.
	if args.BootstrapConstraints.HasRootDiskSource() {
		sp, ok := args.StoragePools[*args.BootstrapConstraints.RootDiskSource]
		if ok {
			pType, _ := sp[domainstorage.StorageProviderType].(string)
			startInstanceArgs.RootDisk = &storage.VolumeParams{
				Provider:   storage.ProviderType(pType),
				Attributes: sp,
			}
		}
	}

	zones, err := startInstanceZones(env, bootstrapContext, startInstanceArgs)
	if errors.Is(err, errors.NotImplemented) {
		// No zone support, so just call StartInstance with
		// a blank StartInstanceParams.AvailabilityZone.
		zones = []string{""}
		if args.BootstrapConstraints.HasZones() {
			logger.Debugf(bootstrapContext, "environ doesn't support zones: ignoring bootstrap zone constraints")
		}
	} else if err != nil {
		return nil, nil, nil, errors.Annotate(err, "cannot start bootstrap instance")
	} else if args.BootstrapConstraints.HasZones() {
		// TODO(hpidcock): bootstrap and worker/provisioner should probably derive
		// from the same logic regarding placement.
		var filteredZones []string
		for _, zone := range zones {
			for _, zoneConstraint := range *args.BootstrapConstraints.Zones {
				if zone == zoneConstraint {
					filteredZones = append(filteredZones, zone)
					break
				}
			}
		}
		if len(filteredZones) == 0 {
			return nil, nil, nil, errors.Errorf(
				"no available zones (%+q) matching bootstrap zone constraints (%+q)",
				zones,
				*args.BootstrapConstraints.Zones,
			)
		}
		zones = filteredZones
	}

	var result *environs.StartInstanceResult
	zoneErrors := []error{} // is a collection of errors we encounter for each zone.
	for i, zone := range zones {
		startInstanceArgs.AvailabilityZone = zone
		result, err = env.StartInstance(bootstrapContext, startInstanceArgs)
		if err == nil {
			break
		}
		zoneErrors = append(zoneErrors, fmt.Errorf("starting bootstrap instance in zone %q: %w", zone, err))

		select {
		case <-bootstrapContext.Done():
			return nil, nil, nil, errors.Annotate(err, "starting controller (cancelled)")
		default:
		}

		if zone == "" || errors.Is(err, environs.ErrAvailabilityZoneIndependent) {
			return nil, nil, nil, errors.Annotate(err, "cannot start bootstrap instance")
		}

		if i < len(zones)-1 {
			// Try the next zone.
			logger.Debugf(bootstrapContext, "failed to start instance in availability zone %q: %s", zone, err)
			continue
		}
		// This is the last zone in the list, error.
		if len(zones) > 1 {
			return nil, nil, nil, fmt.Errorf(
				"cannot start bootstrap instance in any availability zone (%s):\n%w",
				strings.Join(zones, ", "), stderrors.Join(zoneErrors...),
			)
		}
		return nil, nil, nil, errors.Annotatef(err, "cannot start bootstrap instance in availability zone %q", zone)
	}
	modelFw, ok := env.(models.ModelFirewaller)
	if ok {
		if err := openControllerModelPorts(bootstrapContext, modelFw, args.ControllerConfig, env.Config()); err != nil {
			return nil, nil, nil, errors.Annotate(err, "cannot open SSH")
		}
	}

	err = statusCleanup()
	if err != nil {
		return nil, nil, nil, errors.Annotate(err, "cleaning up status line")
	}
	msg := fmt.Sprintf(" - %s (%s)", result.Instance.Id(), formatHardware(result.Hardware))
	// We need some padding below to overwrite any previous messages.
	if len(msg) < 40 {
		padding := make([]string, 40-len(msg))
		msg += strings.Join(padding, " ")
	}
	bootstrapContext.Infof(msg)

	finalizer := func(ctx environs.BootstrapContext, icfg *instancecfg.InstanceConfig, opts environs.BootstrapDialOpts) error {
		icfg.Bootstrap.BootstrapMachineInstanceId = result.Instance.Id()
		icfg.Bootstrap.BootstrapMachineDisplayName = result.DisplayName
		icfg.Bootstrap.BootstrapMachineHardwareCharacteristics = result.Hardware
		icfg.Bootstrap.InitialSSHHostKeys = initialSSHHostKeys
		envConfig := env.Config()
		if result.Config != nil {
			updated, err := envConfig.Apply(result.Config.UnknownAttrs())
			if err != nil {
				return errors.Trace(err)
			}
			envConfig = updated
		}
		if err := instancecfg.FinishInstanceConfig(icfg, envConfig); err != nil {
			return err
		}
		return FinishBootstrap(bootstrapContext, client, env, result.Instance, icfg, opts)
	}
	return result, &requestedBootstrapBase, finalizer, nil
}

func startInstanceZones(env environs.Environ, ctx context.Context, args environs.StartInstanceParams) ([]string, error) {
	zonedEnviron, ok := env.(ZonedEnviron)
	if !ok {
		return nil, errors.NotImplementedf("ZonedEnviron")
	}

	// Attempt creating the instance in each of the availability
	// zones, unless the args imply a specific zone.
	zones, err := zonedEnviron.DeriveAvailabilityZones(ctx, args)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(zones) > 0 {
		return zones, nil
	}
	allZones, err := zonedEnviron.AvailabilityZones(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	for _, zone := range allZones {
		if !zone.Available() {
			continue
		}
		zones = append(zones, zone.Name())
	}
	if len(zones) == 0 {
		return nil, errors.New("no usable availability zones")
	}
	return zones, nil
}

// openControllerModelPorts opens port 22 and apiports on the controller to the configured allow list.
// This is all that is required for the bootstrap to continue. Further configured
// rules will be opened by the firewaller, Once it has started
func openControllerModelPorts(bootstrapContext context.Context,
	modelFw models.ModelFirewaller, controllerConfig controller.Config, cfg *config.Config) error {
	rules := firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("22"), cfg.SSHAllow()...),
		firewall.NewIngressRule(network.PortRange{
			Protocol: "tcp",
			FromPort: controllerConfig.APIPort(),
			ToPort:   controllerConfig.APIPort(),
		}),
	}

	if controllerConfig.AutocertDNSName() != "" {
		// Open port 80 as well as it handles Let's Encrypt HTTP challenge.
		rules = append(rules,
			firewall.NewIngressRule(network.PortRange{
				Protocol: "tcp",
				FromPort: 80,
				ToPort:   80,
			}),
		)
	}

	return modelFw.OpenModelPorts(bootstrapContext, rules)
}

func formatHardware(hw *instance.HardwareCharacteristics) string {
	if hw == nil {
		return ""
	}
	out := make([]string, 0, 3)
	if hw.Arch != nil && *hw.Arch != "" {
		out = append(out, fmt.Sprintf("arch=%s", *hw.Arch))
	}
	if hw.Mem != nil && *hw.Mem > 0 {
		out = append(out, fmt.Sprintf("mem=%s", formatMemory(*hw.Mem)))
	}
	if hw.CpuCores != nil && *hw.CpuCores > 0 {
		out = append(out, fmt.Sprintf("cores=%d", *hw.CpuCores))
	}
	// If the virt-type is the default, don't print it out, as it's just noise.
	if hw.VirtType != nil && *hw.VirtType != "" && *hw.VirtType != string(instance.DefaultInstanceType) {
		out = append(out, fmt.Sprintf("virt-type=%s", *hw.VirtType))
	}
	return strings.Join(out, " ")
}

func formatMemory(m uint64) string {
	if m < 1024 {
		return fmt.Sprintf("%dM", m)
	}
	s := fmt.Sprintf("%.1f", float32(m)/1024.0)
	return strings.TrimSuffix(s, ".0") + "G"
}

// FinishBootstrap completes the bootstrap process by connecting
// to the instance via SSH and carrying out the cloud-config.
//
// Note: FinishBootstrap is exposed so it can be replaced for testing.
var FinishBootstrap = func(
	ctx environs.BootstrapContext,
	client ssh.Client,
	env environs.Environ,
	inst instances.Instance,
	instanceConfig *instancecfg.InstanceConfig,
	opts environs.BootstrapDialOpts,
) error {
	interrupted := make(chan os.Signal, 1)
	ctx.InterruptNotify(interrupted)
	defer ctx.StopInterruptNotify(interrupted)

	hostSSHOptions := bootstrapSSHOptionsFunc(instanceConfig)
	addr, err := WaitSSH(
		ctx,
		ctx.GetStderr(),
		client,
		GetCheckNonceCommand(instanceConfig),
		&RefreshableInstance{inst, env},
		opts,
		hostSSHOptions,
	)
	if err != nil {
		return err
	}
	ctx.Infof("Connected to %v", addr)

	sshOptions, cleanup, err := hostSSHOptions(addr)
	if err != nil {
		return err
	}
	defer cleanup()

	return ConfigureMachine(ctx, client, addr, instanceConfig, sshOptions)
}

func GetCheckNonceCommand(instanceConfig *instancecfg.InstanceConfig) string {
	// Each attempt to connect to an address must verify the machine is the
	// bootstrap machine by checking its nonce file exists and contains the
	// nonce in the InstanceConfig. This also blocks sshinit from proceeding
	// until cloud-init has completed, which is necessary to ensure apt
	// invocations don't trample each other.
	nonceFile := utils.ShQuote(path.Join(instanceConfig.DataDir, cloudconfig.NonceFile))
	checkNonceCommand := fmt.Sprintf(`
	noncefile=%s
	if [ ! -e "$noncefile" ]; then
		echo "$noncefile does not exist" >&2
		exit 1
	fi
	content=$(cat $noncefile)
	if [ "$content" != %s ]; then
		echo "$noncefile contents do not match machine nonce" >&2
		exit 1
	fi
	`, nonceFile, utils.ShQuote(instanceConfig.MachineNonce))
	return checkNonceCommand
}

func ConfigureMachine(
	ctx environs.BootstrapContext,
	client ssh.Client,
	host string,
	instanceConfig *instancecfg.InstanceConfig,
	sshOptions *ssh.Options,
) error {
	// Bootstrap is synchronous, and will spawn a subprocess
	// to complete the procedure. If the user hits Ctrl-C,
	// SIGINT is sent to the foreground process attached to
	// the terminal, which will be the ssh subprocess at this
	// point. For that reason, we do not call StopInterruptNotify
	// until this function completes.
	cloudcfg, err := cloudinit.New(instanceConfig.Base.OS)
	if err != nil {
		return errors.Trace(err)
	}

	// Set packaging update here
	cloudcfg.SetSystemUpdate(instanceConfig.EnableOSRefreshUpdate)
	cloudcfg.SetSystemUpgrade(instanceConfig.EnableOSUpgrade)

	sshinitConfig := sshinit.ConfigureParams{
		Host:           "ubuntu@" + host,
		Client:         client,
		SSHOptions:     sshOptions,
		Config:         cloudcfg,
		ProgressWriter: ctx.GetStderr(),
	}

	ft := sshinit.NewFileTransporter(sshinitConfig)
	cloudcfg.SetFileTransporter(ft)

	udata, err := cloudconfig.NewUserdataConfig(instanceConfig, cloudcfg)
	if err != nil {
		return err
	}
	if err := udata.ConfigureJuju(); err != nil {
		return err
	}
	if err := udata.ConfigureCustomOverrides(); err != nil {
		return err
	}
	configScript, err := cloudcfg.RenderScript()
	if err != nil {
		return err
	}

	// Wait for the files to be sent to the machine.
	if err := ft.Dispatch(ctx); err != nil {
		return errors.Annotate(err, "transporting files to machine")
	}

	script := shell.DumpFileOnErrorScript(instanceConfig.CloudInitOutputLog) + configScript
	ctx.Infof("Running machine configuration script...")
	// TODO(benhoyt) - plumb context through juju/utils/ssh?
	return sshinit.RunConfigureScript(script, sshinitConfig)
}

// HostSSHOptionsFunc is a function that, given a hostname, returns
// an ssh.Options and a cleanup function, or an error.
type HostSSHOptionsFunc func(host string) (*ssh.Options, func(), error)

// DefaultHostSSHOptions returns a nil *ssh.Options, which means
// to use the defaults; and a no-op cleanup function.
func DefaultHostSSHOptions(string) (*ssh.Options, func(), error) {
	return nil, func() {}, nil
}

// bootstrapSSHOptionsFunc that takes a bootstrap machine's InstanceConfig
// and returns a HostSSHOptionsFunc.
func bootstrapSSHOptionsFunc(instanceConfig *instancecfg.InstanceConfig) HostSSHOptionsFunc {
	return func(host string) (*ssh.Options, func(), error) {
		return hostBootstrapSSHOptions(host, instanceConfig)
	}
}

func hostBootstrapSSHOptions(
	host string,
	instanceConfig *instancecfg.InstanceConfig,
) (_ *ssh.Options, cleanup func(), err error) {
	cleanup = func() {}
	defer func() {
		if err != nil {
			cleanup()
		}
	}()

	options := &ssh.Options{}
	options.SetStrictHostKeyChecking(ssh.StrictHostChecksYes)

	// If any host keys are being injected, we'll set up a
	// known_hosts file with their contents, and accept only
	// them.
	hostKeys := instanceConfig.Bootstrap.InitialSSHHostKeys
	var algos []string
	var pubKeys []string
	for _, hostKey := range hostKeys {
		algos = append(algos, hostKey.PublicKeyAlgorithm)
		pubKeys = append(pubKeys, hostKey.Public)
	}
	if len(pubKeys) == 0 {
		return options, cleanup, nil
	}

	// Create a temporary known_hosts file.
	f, err := os.CreateTemp("", "juju-known-hosts")
	if err != nil {
		return nil, cleanup, errors.Trace(err)
	}
	cleanup = func() {
		_ = f.Close()
		_ = os.RemoveAll(f.Name())
	}
	w := bufio.NewWriter(f)
	for _, pubKey := range pubKeys {
		fmt.Fprintln(w, host, strings.TrimSpace(pubKey))
	}
	if err := w.Flush(); err != nil {
		return nil, cleanup, errors.Annotate(err, "writing known_hosts")
	}

	options.SetHostKeyAlgorithms(algos...)
	options.SetKnownHostsFile(f.Name())
	return options, cleanup, nil
}

// InstanceRefresher is the subet of the Instance interface required
// for waiting for SSH access to become available.
type InstanceRefresher interface {
	// Refresh refreshes the addresses for the instance.
	Refresh(ctx context.Context) error

	// Addresses returns the addresses for the instance.
	// To ensure that the results are up to date, call
	// Refresh first.
	Addresses(ctx context.Context) (network.ProviderAddresses, error)

	// Status returns the provider-specific status for the
	// instance.
	Status(ctx context.Context) instance.Status
}

type RefreshableInstance struct {
	instances.Instance
	Env environs.Environ
}

// Refresh refreshes the addresses for the instance.
func (i *RefreshableInstance) Refresh(ctx context.Context) error {
	instances, err := i.Env.Instances(ctx, []instance.Id{i.Id()})
	if err != nil {
		return errors.Trace(err)
	}
	i.Instance = instances[0]
	return nil
}

type hostChecker struct {
	addr           network.ProviderAddress
	client         ssh.Client
	hostSSHOptions HostSSHOptionsFunc
	wg             *sync.WaitGroup

	// checkDelay is the amount of time to wait between retries.
	checkDelay time.Duration

	// checkHostScript is executed on the host via SSH.
	// hostChecker.loop will return once the script
	// runs without error.
	checkHostScript string

	// closed is closed to indicate that the host checker should
	// return, without waiting for the result of any ongoing
	// attempts.
	closed <-chan struct{}
}

// Close implements io.Closer, as required by parallel.Try.
func (*hostChecker) Close() error {
	return nil
}

func (hc *hostChecker) loop(dying <-chan struct{}) (io.Closer, error) {
	defer hc.wg.Done()

	address := hc.addr.Value
	sshOptions, cleanup, err := hc.hostSSHOptions(address)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	// The value of connectSSH is taken outside the goroutine that may outlive
	// hostChecker.loop, or we evoke the wrath of the race detector.
	connectSSH := connectSSH
	var lastErr error
	done := make(chan error, 1)
	for {
		go func() {
			done <- connectSSH(hc.client, address, hc.checkHostScript, sshOptions)
		}()
		select {
		case <-dying:
			return hc, lastErr
		case lastErr = <-done:
			if lastErr == nil {
				return hc, nil
			}
			logger.Debugf(context.TODO(), "connection attempt for %s failed: %v", address, lastErr)
		}
		select {
		case <-hc.closed:
			return hc, lastErr
		case <-dying:
		case <-time.After(hc.checkDelay):
		}
	}
}

type parallelHostChecker struct {
	*parallel.Try
	client         ssh.Client
	hostSSHOptions HostSSHOptionsFunc
	stderr         io.Writer
	wg             sync.WaitGroup

	// active is a map of addresses to channels for addresses actively
	// being tested. The goroutine testing the address will continue
	// to attempt connecting to the address until it succeeds, the Try
	// is killed, or the corresponding channel in this map is closed.
	active map[network.ProviderAddress]chan struct{}

	// checkDelay is how long each hostChecker waits between attempts.
	checkDelay time.Duration

	// checkHostScript is the script to run on each host to check that
	// it is the host we expect.
	checkHostScript string
}

func (p *parallelHostChecker) UpdateAddresses(addrs []network.ProviderAddress) {
	for _, addr := range addrs {
		if _, ok := p.active[addr]; ok {
			continue
		}
		fmt.Fprintf(p.stderr, "Attempting to connect to %s\n", net.JoinHostPort(addr.Value, "22"))
		closed := make(chan struct{})
		hc := &hostChecker{
			addr:            addr,
			client:          p.client,
			hostSSHOptions:  p.hostSSHOptions,
			checkDelay:      p.checkDelay,
			checkHostScript: p.checkHostScript,
			closed:          closed,
			wg:              &p.wg,
		}
		p.wg.Add(1)
		p.active[addr] = closed
		_ = p.Start(hc.loop)
	}
}

// Close prevents additional functions from being added to
// the Try, and tells each active hostChecker to exit.
func (p *parallelHostChecker) Close() error {
	// We signal each checker to stop and wait for them
	// each to complete; this allows us to get the error,
	// as opposed to when using try.Kill which does not
	// wait for the functions to complete.
	p.Try.Close()
	for _, ch := range p.active {
		close(ch)
	}
	return nil
}

// connectSSH is called to connect to the specified host and
// execute the "checkHostScript" bash script on it.
var connectSSH = func(client ssh.Client, host, checkHostScript string, options *ssh.Options) error {
	cmd := client.Command("ubuntu@"+host, []string{"/bin/bash"}, options)
	cmd.Stdin = strings.NewReader(checkHostScript)
	output, err := cmd.CombinedOutput()
	if err != nil && len(output) > 0 {
		err = fmt.Errorf("%s", strings.TrimSpace(string(output)))
	}
	return err
}

// WaitSSH waits for the instance to be assigned a routable
// address, then waits until we can connect to it via SSH.
//
// waitSSH attempts on all addresses returned by the instance
// in parallel; the first succeeding one wins. We ensure that
// private addresses are for the correct machine by checking
// the presence of a file on the machine that contains the
// machine's nonce. The "checkHostScript" is a bash script
// that performs this file check.
func WaitSSH(
	ctx context.Context,
	stdErr io.Writer,
	client ssh.Client,
	checkHostScript string,
	inst InstanceRefresher,
	opts environs.BootstrapDialOpts,
	hostSSHOptions HostSSHOptionsFunc,
) (addr string, err error) {
	globalTimeout := time.After(opts.Timeout)
	pollAddresses := time.NewTimer(0)

	// checker checks each address in a loop, in parallel,
	// until one succeeds, the global timeout is reached,
	// or the tomb is killed.
	checker := parallelHostChecker{
		Try:             parallel.NewTry(0, nil),
		client:          client,
		stderr:          stdErr,
		active:          make(map[network.ProviderAddress]chan struct{}),
		checkDelay:      opts.RetryDelay,
		checkHostScript: checkHostScript,
		hostSSHOptions:  hostSSHOptions,
	}
	defer checker.wg.Wait()
	defer checker.Kill()

	fmt.Fprintln(stdErr, "Waiting for address")
	for {
		select {
		case <-pollAddresses.C:
			pollAddresses.Reset(opts.AddressesDelay)
			if err := inst.Refresh(ctx); err != nil {
				return "", fmt.Errorf("refreshing addresses: %v", err)
			}
			instanceStatus := inst.Status(ctx)
			if instanceStatus.Status == status.ProvisioningError {
				if instanceStatus.Message != "" {
					return "", errors.Errorf("instance provisioning failed (%v)", instanceStatus.Message)
				}
				return "", errors.Errorf("instance provisioning failed")
			}
			addresses, err := inst.Addresses(ctx)
			if err != nil {
				return "", fmt.Errorf("getting addresses: %v", err)
			}
			checker.UpdateAddresses(addresses)
		case <-globalTimeout:
			checker.Close()
			lastErr := checker.Wait()
			format := "waited for %v "
			args := []interface{}{opts.Timeout}
			if len(checker.active) == 0 {
				format += "without getting any addresses"
			} else {
				format += "without being able to connect"
			}
			if lastErr != nil && lastErr != parallel.ErrStopped {
				format += ": %v"
				args = append(args, lastErr)
			}
			return "", fmt.Errorf(format, args...)
		case <-ctx.Done():
			return "", bootstrap.Cancelled()
		case <-checker.Dead():
			result, err := checker.Result()
			if err != nil {
				return "", err
			}
			return result.(*hostChecker).addr.Value, nil
		}
	}
}

func generateSSHHostKeys() (instancecfg.SSHHostKeys, error) {
	// Generate a single ssh-rsa key. We'll configure the SSH client
	// such that that is the only host key type we'll accept.
	var keys instancecfg.SSHHostKeys

	hostKeys, err := pkissh.GenerateHostKeys()
	if err != nil {
		return nil, errors.Annotate(err, "generating SSH keys")
	}

	for i, key := range hostKeys {
		private, public, keyType, err := pkissh.FormatKey(key, fmt.Sprintf("juju-bootstrap-%d", i))
		if err != nil {
			return nil, errors.Annotate(err, "generating SSH key")
		}

		keys = append(keys, instancecfg.SSHKeyPair{
			Private:            private,
			Public:             public,
			PublicKeyAlgorithm: keyType,
		})
	}
	return keys, nil
}
