// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machiner

import (
	"context"
	"net"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/worker/v4"

	corelife "github.com/juju/juju/core/life"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/internal/network"
	jworker "github.com/juju/juju/internal/worker"
	"github.com/juju/juju/rpc/params"
)

var logger = internallogger.GetLogger("juju.worker.machiner")

// Config defines the configuration for a machiner worker.
type Config struct {
	// MachineAccessor provides a means of observing and updating the
	// machine's state.
	MachineAccessor MachineAccessor

	// Tag is the machine's tag.
	Tag names.MachineTag

	// ClearMachineAddressesOnStart indicates whether or not to clear
	// the machine's machine addresses when the worker starts.
	ClearMachineAddressesOnStart bool
}

// Validate reports whether or not the configuration is valid.
func (cfg *Config) Validate() error {
	if cfg.MachineAccessor == nil {
		return errors.NotValidf("unspecified MachineAccessor")
	}
	if cfg.Tag == (names.MachineTag{}) {
		return errors.NotValidf("unspecified Tag")
	}
	return nil
}

// Machiner is responsible for a machine agent's lifecycle.
type Machiner struct {
	config  Config
	machine Machine
}

// NewMachiner returns a Worker that will wait for the identified machine
// to become Dying and make it Dead; or until the machine becomes Dead by
// other means.
//
// The machineDead function will be called immediately after the machine's
// lifecycle is updated to Dead.
var NewMachiner = func(cfg Config) (worker.Worker, error) {
	if err := cfg.Validate(); err != nil {
		return nil, errors.Annotate(err, "validating config")
	}
	handler := &Machiner{config: cfg}
	w, err := watcher.NewNotifyWorker(watcher.NotifyConfig{
		Handler: handler,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

// GetObservedNetworkConfig is patched for testing.
var GetObservedNetworkConfig = corenetwork.GetObservedNetworkConfig

func (mr *Machiner) SetUp(ctx context.Context) (watcher.NotifyWatcher, error) {
	// Find which machine we're responsible for.
	m, err := mr.config.MachineAccessor.Machine(ctx, mr.config.Tag)
	if params.IsCodeNotFoundOrCodeUnauthorized(err) {
		return nil, jworker.ErrTerminateAgent
	} else if err != nil {
		return nil, errors.Trace(err)
	}
	mr.machine = m

	switch {
	case corelife.IsNotAlive(m.Life()):
		// Can happen when the machiner is restarting after a failure in EnsureDead.
		// Since we're dying or dead, no need handle the machine addresses.
		logger.Infof(ctx, "%q not alive", mr.config.Tag)
		return m.Watch(ctx)
	case mr.config.ClearMachineAddressesOnStart:
		logger.Debugf(ctx, "machiner configured to reset machine %q addresses to empty", mr.config.Tag)
		if err := m.SetMachineAddresses(ctx, nil); err != nil {
			return nil, errors.Annotate(err, "resetting machine addresses")
		}
	default:
		// Set the addresses in state to the host's addresses if the
		// machine is alive.  No need to set addresses if the machine
		// is dying or dead on a worker restart.
		if err := setMachineAddresses(ctx, mr.config.Tag, m); err != nil {
			return nil, errors.Annotate(err, "setting machine addresses")
		}
	}

	// Mark the machine as started and log it.
	if err := m.SetStatus(ctx, status.Started, "", nil); err != nil {
		return nil, errors.Annotatef(err, "%s failed to set status started", mr.config.Tag)
	}
	logger.Infof(ctx, "%q started", mr.config.Tag)

	return m.Watch(ctx)
}

var interfaceAddrs = net.InterfaceAddrs

// setMachineAddresses sets the addresses for this machine to all of the
// host's non-loopback interface IP addresses.
func setMachineAddresses(ctx context.Context, tag names.MachineTag, m Machine) error {
	addrs, err := interfaceAddrs()
	if err != nil {
		return err
	}
	var hostAddresses corenetwork.ProviderAddresses
	for _, addr := range addrs {
		var (
			ip      net.IP
			netmask net.IPMask
		)

		switch addr := addr.(type) {
		case *net.IPAddr:
			ip = addr.IP
		case *net.IPNet:
			ip = addr.IP
			netmask = addr.Mask
		default:
			continue
		}

		// If we discovered a netmask for the address, include a CIDR
		// when constructing the provider address.
		var addrOpts []func(corenetwork.AddressMutator)
		if cidr := corenetwork.NetworkCIDRFromIPAndMask(ip, netmask); cidr != "" {
			addrOpts = append(addrOpts, corenetwork.WithCIDR(cidr))
		}
		address := corenetwork.NewMachineAddress(ip.String(), addrOpts...).AsProviderAddress()

		// Filter out link-local addresses as we cannot reliably use them.
		if address.Scope == corenetwork.ScopeLinkLocal {
			continue
		}
		hostAddresses = append(hostAddresses, address)
	}
	if len(hostAddresses) == 0 {
		return nil
	}
	// Filter out any LXC or LXD bridge addresses.
	hostAddresses = network.FilterBridgeAddresses(ctx, hostAddresses)
	logger.Infof(ctx, "setting addresses for %q to %v", tag, hostAddresses)

	// TODO (manadart 2019-08-27): This needs refactoring.
	// FilterBridgeAddresses takes a slice of ProviderAddress,
	// so we create an initial slice of that type and extract the machine
	// addresses after filtering.
	// We should work in an appropriate indirection to achieve this logic.
	machineAddresses := make([]corenetwork.MachineAddress, len(hostAddresses))
	for i, addr := range hostAddresses {
		machineAddresses[i] = addr.MachineAddress
	}

	return m.SetMachineAddresses(ctx, machineAddresses)
}

func (mr *Machiner) Handle(ctx context.Context) error {
	if err := mr.machine.Refresh(ctx); params.IsCodeNotFoundOrCodeUnauthorized(err) {
		// NOTE(axw) we can distinguish between NotFound and CodeUnauthorized,
		// so we could call NotifyMachineDead here in case the agent failed to
		// call NotifyMachineDead directly after setting the machine Dead in
		// the first place. We're not doing that to be cautious: the machine
		// could be missing from state due to invalid global state.
		return jworker.ErrTerminateAgent
	} else if err != nil {
		return err
	}

	life := mr.machine.Life()
	if life == corelife.Alive {
		interfaceInfos, err := GetObservedNetworkConfig(corenetwork.DefaultConfigSource())
		if err != nil {
			return errors.Annotate(err, "cannot discover observed network config")
		} else if len(interfaceInfos) == 0 {
			logger.Warningf(ctx, "not updating network config: no observed config found to update")
		}

		observedConfig := params.NetworkConfigFromInterfaceInfo(interfaceInfos)
		if len(observedConfig) > 0 {
			if err := mr.machine.SetObservedNetworkConfig(ctx, observedConfig); err != nil {
				return errors.Annotate(err, "cannot update observed network config")
			}
		}
		logger.Debugf(ctx, "observed network config updated for %q to %+v", mr.config.Tag, observedConfig)

		return nil
	}

	logger.Debugf(ctx, "%q is now %s", mr.config.Tag, life)
	if err := mr.machine.SetStatus(ctx, status.Stopped, "", nil); err != nil {
		return errors.Annotatef(err, "%s failed to set status stopped", mr.config.Tag)
	}

	// Attempt to mark the machine Dead. If the machine still has units
	// assigned, or storage attached, this will fail with
	// CodeHasAssignedUnits or CodeMachineHasAttachedStorage respectively.
	// Once units or storage are removed, the watcher will trigger again
	// and we'll reattempt.  If the machine has containers, EnsureDead will
	// fail with CodeMachineHasContainers.  However the watcher will not
	// trigger again, so fail and let the machiner restart and try again.
	if err := mr.machine.EnsureDead(ctx); err != nil {
		if params.IsCodeHasAssignedUnits(err) {
			logger.Tracef(ctx, "machine still has units")
			return nil
		}
		if params.IsCodeMachineHasAttachedStorage(err) {
			logger.Tracef(ctx, "machine still has storage attached")
			return nil
		}
		if params.IsCodeTryAgain(err) {
			logger.Tracef(ctx, "waiting for machine to be removed as a controller")
			return nil
		}
		if params.IsCodeMachineHasContainers(err) {
			logger.Tracef(ctx, "machine still has containers")
			return errors.Annotatef(err, "%q", mr.config.Tag)
		}
		err = errors.Annotatef(err, "%s failed to set machine to dead", mr.config.Tag)
		if e := mr.machine.SetStatus(ctx, status.Error, errors.Annotate(err, "destroying machine").Error(), nil); e != nil {
			logger.Errorf(ctx, "failed to set status for error %v ", err)
		}
		return errors.Trace(err)
	}
	return jworker.ErrTerminateAgent
}

func (mr *Machiner) TearDown() error {
	// Nothing to do here.
	return nil
}
