// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"fmt"
	"launchpad.net/gwacl"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/utils"
	"sync"
	"time"
)

var longAttempt = utils.AttemptStrategy{
	Total: 3 * time.Minute,
	Delay: 1 * time.Second,
}

type azureEnviron struct {
	// Except where indicated otherwise, all fields in this object should
	// only be accessed using a lock or a snapshot.
	sync.Mutex

	// name is immutable; it can be accessed without locking.
	name string

	// ecfg is the environment's Azure-specific configuration.
	ecfg *azureEnvironConfig

	storage environs.Storage
}

// azureEnviron implements Environ.
var _ environs.Environ = (*azureEnviron)(nil)

func NewEnviron(cfg *config.Config) (*azureEnviron, error) {
	env := azureEnviron{}
	err := env.SetConfig(cfg)
	if err != nil {
		return nil, err
	}
	env.storage = &azureStorage{}
	return &env, nil
}

// Name is specified in the Environ interface.
func (env *azureEnviron) Name() string {
	return env.name
}

// getSnapshot produces an atomic shallow copy of the environment object.
// Whenever you need to access the environment object's fields without
// modifying them, get a snapshot and read its fields instead.  You will
// get a consistent view of the fields without any further locking.
// If you do need to modify the environment's fields, do not get a snapshot
// but lock the object throughout the critical section.
func (env *azureEnviron) getSnapshot() *azureEnviron {
	env.Lock()
	defer env.Unlock()

	// Copy the environment.  (Not the pointer, the environment itself.)
	// This is a shallow copy.
	snap := *env
	// Reset the snapshot's mutex, because we just copied it while we
	// were holding it.  The snapshot will have a "clean," unlocked mutex.
	snap.Mutex = sync.Mutex{}
	return &snap
}

// Bootstrap is specified in the Environ interface.
func (env *azureEnviron) Bootstrap(cons constraints.Value) error {
	panic("unimplemented")
}

// StateInfo is specified in the Environ interface.
func (env *azureEnviron) StateInfo() (*state.Info, *api.Info, error) {
	// This code is cargo-culted from the ec2/maas/openstack providers.
	// It's not clear that the longAttempt loop has any business being
	// here, but it's probably a refactoring that needs to happen outside
	// of the provider code.
	st, err := env.loadState()
	if err != nil {
		return nil, nil, err
	}
	config := env.Config()
	cert, hasCert := config.CACert()
	if !hasCert {
		return nil, nil, fmt.Errorf("no CA certificate in environment configuration")
	}
	var stateAddrs []string
	var apiAddrs []string
	// Wait for the DNS names of any of the instances to become available.
	log.Debugf("environs/azure: waiting for DNS name(s) of state server instances %v", st.StateInstances)
	for a := longAttempt.Start(); len(stateAddrs) == 0 && a.Next(); {
		insts, err := env.Instances(st.StateInstances)
		if err != nil && err != environs.ErrPartialInstances {
			log.Debugf("environs/azure: error  getting state instance: %v", err.Error())
			return nil, nil, err
		}
		log.Debugf("environs/azure: started processing instances: %#v", insts)
		for _, inst := range insts {
			if inst == nil {
				continue
			}
			name, err := inst.DNSName()
			if err != nil {
				continue
			}
			if name != "" {
				statePortSuffix := fmt.Sprintf(":%d", config.StatePort())
				apiPortSuffix := fmt.Sprintf(":%d", config.APIPort())
				stateAddrs = append(stateAddrs, name+statePortSuffix)
				apiAddrs = append(apiAddrs, name+apiPortSuffix)
			}
		}
	}
	if len(stateAddrs) == 0 {
		return nil, nil, fmt.Errorf("timed out waiting for mgo address from %v", st.StateInstances)
	}
	return &state.Info{
			Addrs:  stateAddrs,
			CACert: cert,
		}, &api.Info{
			Addrs:  apiAddrs,
			CACert: cert,
		}, nil
}

// Config is specified in the Environ interface.
func (env *azureEnviron) Config() *config.Config {
	snap := env.getSnapshot()
	return snap.ecfg.Config
}

// SetConfig is specified in the Environ interface.
func (env *azureEnviron) SetConfig(cfg *config.Config) error {
	panic("unimplemented")
}

// StartInstance is specified in the Environ interface.
func (env *azureEnviron) StartInstance(machineId, machineNonce string, series string, cons constraints.Value, info *state.Info, apiInfo *api.Info) (instance.Instance, error) {
	panic("unimplemented")
}

// StopInstances is specified in the Environ interface.
func (env *azureEnviron) StopInstances([]instance.Instance) error {
	panic("unimplemented")
}

// Instances is specified in the Environ interface.
func (env *azureEnviron) Instances(ids []instance.Id) ([]instance.Instance, error) {
	panic("unimplemented")
}

// AllInstances is specified in the Environ interface.
func (env *azureEnviron) AllInstances() ([]instance.Instance, error) {
	panic("unimplemented")
}

// Storage is specified in the Environ interface.
func (env *azureEnviron) Storage() environs.Storage {
	return env.getSnapshot().storage
}

// PublicStorage is specified in the Environ interface.
func (env *azureEnviron) PublicStorage() environs.StorageReader {
	panic("unimplemented")
}

// Destroy is specified in the Environ interface.
func (env *azureEnviron) Destroy(insts []instance.Instance) error {
	panic("unimplemented")
}

// OpenPorts is specified in the Environ interface.
func (env *azureEnviron) OpenPorts(ports []instance.Port) error {
	panic("unimplemented")
}

// ClosePorts is specified in the Environ interface.
func (env *azureEnviron) ClosePorts(ports []instance.Port) error {
	panic("unimplemented")
}

// Ports is specified in the Environ interface.
func (env *azureEnviron) Ports() ([]instance.Port, error) {
	panic("unimplemented")
}

// Provider is specified in the Environ interface.
func (env *azureEnviron) Provider() environs.EnvironProvider {
	panic("unimplemented")
}

// azureManagementContext wraps two things: a gwacl.ManagementAPI (effectively
// a session on the Azure management API) and a tempCertFile, which keeps track
// of the temporary certificate file that needs to be deleted once we're done
// with this particular session.
// Since it embeds *gwacl.ManagementAPI, you can use it much as if it were a
// pointer to a ManagementAPI object.  Just don't forget to release it after
// use.
type azureManagementContext struct {
	*gwacl.ManagementAPI
	certFile *tempCertFile
}

// getManagementAPI obtains a context object for interfacing with Azure's
// management API.
// For now, each invocation just returns a separate object.  This is probably
// wasteful (each context gets its own SSL connection) and may need optimizing
// later.
func (env *azureEnviron) getManagementAPI() (*azureManagementContext, error) {
	snap := env.getSnapshot()
	subscription := snap.ecfg.ManagementSubscriptionId()
	certData := snap.ecfg.ManagementCertificate()
	certFile, err := newTempCertFile([]byte(certData))
	if err != nil {
		return nil, err
	}
	// After this point, if we need to leave prematurely, we should clean
	// up that certificate file.
	mgtAPI, err := gwacl.NewManagementAPI(subscription, certFile.Path())
	if err != nil {
		certFile.Delete()
		return nil, err
	}
	context := azureManagementContext{
		ManagementAPI: mgtAPI,
		certFile:      certFile,
	}
	return &context, nil
}

// releaseManagementAPI frees up a context object obtained through
// getManagementAPI.
func (env *azureEnviron) releaseManagementAPI(context *azureManagementContext) {
	// Be tolerant to incomplete context objects, in case we ever get
	// called during cleanup of a failed attempt to create one.
	if context == nil || context.certFile == nil {
		return
	}
	// For now, all that needs doing is to delete the temporary certificate
	// file.  We may do cleverer things later, such as connection pooling
	// where this method returns a context to the pool.
	context.certFile.Delete()
}

// getStorageContext obtains a context object for interfacing with Azure's
// storage API.
// For now, each invocation just returns a separate object.  This is probably
// wasteful (each context gets its own SSL connection) and may need optimizing
// later.
func (env *azureEnviron) getStorageContext() (*gwacl.StorageContext, error) {
	snap := env.getSnapshot()
	context := gwacl.StorageContext{
		Account: snap.ecfg.StorageAccountName(),
		Key:     snap.ecfg.StorageAccountKey(),
	}
	// There is currently no way for this to fail.
	return &context, nil
}
