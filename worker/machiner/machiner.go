// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package machiner

import (
	"net"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/network"
	"github.com/juju/juju/watcher"
	"github.com/juju/juju/worker"
)

var logger = loggo.GetLogger("juju.worker.machiner")

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

	// NotifyMachineDead will, if non-nil, be called after the machine
	// is transitioned to the Dead lifecycle state.
	NotifyMachineDead func() error
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

func (mr *Machiner) SetUp() (watcher.NotifyWatcher, error) {
	// Find which machine we're responsible for.
	m, err := mr.config.MachineAccessor.Machine(mr.config.Tag)
	if params.IsCodeNotFoundOrCodeUnauthorized(err) {
		return nil, worker.ErrTerminateAgent
	} else if err != nil {
		return nil, errors.Trace(err)
	}
	mr.machine = m

	if mr.config.ClearMachineAddressesOnStart {
		logger.Debugf("machine addresses ignored on start - resetting machine addresses")
		if err := m.SetMachineAddresses(nil); err != nil {
			return nil, errors.Annotate(err, "reseting machine addresses")
		}
	} else {
		// Set the addresses in state to the host's addresses.
		if err := setMachineAddresses(mr.config.Tag, m); err != nil {
			return nil, errors.Annotate(err, "setting machine addresses")
		}
	}

	// Mark the machine as started and log it.
	if err := m.SetStatus(params.StatusStarted, "", nil); err != nil {
		return nil, errors.Annotatef(err, "%s failed to set status started", mr.config.Tag)
	}
	logger.Infof("%q started", mr.config.Tag)

	return m.Watch()
}

var interfaceAddrs = net.InterfaceAddrs

// setMachineAddresses sets the addresses for this machine to all of the
// host's non-loopback interface IP addresses.
func setMachineAddresses(tag names.MachineTag, m Machine) error {
	addrs, err := interfaceAddrs()
	if err != nil {
		return err
	}
	var hostAddresses []network.Address
	for _, addr := range addrs {
		var ip net.IP
		switch addr := addr.(type) {
		case *net.IPAddr:
			ip = addr.IP
		case *net.IPNet:
			ip = addr.IP
		default:
			continue
		}
		address := network.NewAddress(ip.String())
		// Filter out link-local addresses as we cannot reliably use them.
		if address.Scope == network.ScopeLinkLocal {
			continue
		}
		hostAddresses = append(hostAddresses, address)
	}
	if len(hostAddresses) == 0 {
		return nil
	}
	// Filter out any LXC bridge addresses.
	hostAddresses = network.FilterLXCAddresses(hostAddresses)
	logger.Infof("setting addresses for %v to %q", tag, hostAddresses)
	return m.SetMachineAddresses(hostAddresses)
}

func (mr *Machiner) Handle(_ <-chan struct{}) error {
	if err := mr.machine.Refresh(); params.IsCodeNotFoundOrCodeUnauthorized(err) {
		// NOTE(axw) we can distinguish between NotFound and CodeUnauthorized,
		// so we could call NotifyMachineDead here in case the agent failed to
		// call NotifyMachineDead directly after setting the machine Dead in
		// the first place. We're not doing that to be cautious: the machine
		// could be missing from state due to invalid global state.
		return worker.ErrTerminateAgent
	} else if err != nil {
		return err
	}
	life := mr.machine.Life()
	if life == params.Alive {
		return nil
	}
	logger.Debugf("%q is now %s", mr.config.Tag, life)
	if err := mr.machine.SetStatus(params.StatusStopped, "", nil); err != nil {
		return errors.Annotatef(err, "%s failed to set status stopped", mr.config.Tag)
	}

	// Attempt to mark the machine Dead. If the machine still has units
	// assigned, or storage attached, this will fail with
	// CodeHasAssignedUnits or CodeMachineHasAttachedStorage respectively.
	// Once units or storage are removed, the watcher will trigger again
	// and we'll reattempt.
	if err := mr.machine.EnsureDead(); err != nil {
		if params.IsCodeHasAssignedUnits(err) {
			return nil
		}
		if params.IsCodeMachineHasAttachedStorage(err) {
			logger.Tracef("machine still has storage attached")
			return nil
		}
		return errors.Annotatef(err, "%s failed to set machine to dead", mr.config.Tag)
	}
	// Report on the machine's death. It is important that we do this after
	// the machine is Dead, because this is the mechanism we use to clean up
	// the machine (uninstall). If we were to report before marking the machine
	// as Dead, then we would risk uninstalling prematurely.
	if mr.config.NotifyMachineDead != nil {
		if err := mr.config.NotifyMachineDead(); err != nil {
			return errors.Annotate(err, "reporting machine death")
		}
	}
	return worker.ErrTerminateAgent
}

func (mr *Machiner) TearDown() error {
	// Nothing to do here.
	return nil
}
