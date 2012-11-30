// Stub provider for OpenStack, using goose will be implemented here

package openstack

import (
	"fmt"
	"io/ioutil"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/trivial"
	"net/http"
	"strings"
	"sync"
	"time"
)

type environProvider struct{}

var _ environs.EnvironProvider = (*environProvider)(nil)

var providerInstance environProvider

// A request may fail to due "eventual consistency" semantics, which
// should resolve fairly quickly.  A request may also fail due to a slow
// state transition (for instance an instance taking a while to release
// a security group after termination).  The former failure mode is
// dealt with by shortAttempt, the latter by longAttempt.
var shortAttempt = trivial.AttemptStrategy{
	Total: 5 * time.Second,
	Delay: 200 * time.Millisecond,
}

var longAttempt = trivial.AttemptStrategy{
	Total: 3 * time.Minute,
	Delay: 1 * time.Second,
}

func init() {
	environs.RegisterProvider("openstack", environProvider{})
}

func (p environProvider) Open(cfg *config.Config) (environs.Environ, error) {
	log.Printf("environs/openstack: opening environment %q", cfg.Name())
	e := new(environ)
	err := e.SetConfig(cfg)
	if err != nil {
		return nil, err
	}
	return e, nil
}

func (p environProvider) SecretAttrs(cfg *config.Config) (map[string]interface{}, error) {
	m := make(map[string]interface{})
	ecfg, err := providerInstance.newConfig(cfg)
	if err != nil {
		return nil, err
	}
	m["username"] = ecfg.username()
	m["password"] = ecfg.password()
	m["tenant-name"] = ecfg.tenantName()
	return m, nil
}

func (p environProvider) PublicAddress() (string, error) {
	return fetchMetadata("public-hostname")
}

func (p environProvider) PrivateAddress() (string, error) {
	return fetchMetadata("local-hostname")
}

type environ struct {
	name string

	ecfgMutex    sync.Mutex
	ecfgUnlocked *environConfig
}

var _ environs.Environ = (*environ)(nil)

func (e *environ) ecfg() *environConfig {
	e.ecfgMutex.Lock()
	ecfg := e.ecfgUnlocked
	e.ecfgMutex.Unlock()
	return ecfg
}

func (e *environ) Name() string {
	return e.name
}

func (e *environ) Bootstrap(uploadTools bool, cert, key []byte) error {
	panic("not implemented")
}

func (e *environ) StateInfo() (*state.Info, error) {
	panic("not implemented")
}

func (e *environ) Config() *config.Config {
	panic("not implemented")
}

func (e *environ) SetConfig(cfg *config.Config) error {
	ecfg, err := providerInstance.newConfig(cfg)
	if err != nil {
		return err
	}
	e.ecfgMutex.Lock()
	defer e.ecfgMutex.Unlock()
	e.name = ecfg.Name()
	e.ecfgUnlocked = ecfg

	// TODO(dimitern): setup the goose client auth/compute, etc. here
	return nil
}

func (e *environ) StartInstance(machineId string, info *state.Info, tools *state.Tools) (environs.Instance, error) {
	panic("not implemented")
}

func (e *environ) StopInstances([]environs.Instance) error {
	panic("not implemented")
}

func (e *environ) Instances(ids []state.InstanceId) ([]environs.Instance, error) {
	panic("not implemented")
}

func (e *environ) AllInstances() ([]environs.Instance, error) {
	panic("not implemented")
}

func (e *environ) Storage() environs.Storage {
	panic("not implemented")
}

func (e *environ) PublicStorage() environs.StorageReader {
	panic("not implemented")
}

func (e *environ) Destroy(insts []environs.Instance) error {
	panic("not implemented")
}

func (e *environ) AssignmentPolicy() state.AssignmentPolicy {
	panic("not implemented")
}

func (e *environ) OpenPorts(ports []state.Port) error {
	panic("not implemented")
}

func (e *environ) ClosePorts(ports []state.Port) error {
	panic("not implemented")
}

func (e *environ) Ports() ([]state.Port, error) {
	panic("not implemented")
}

func (e *environ) Provider() environs.EnvironProvider {
	return &providerInstance
}

// metadataHost holds the address of the instance metadata service.
// It is a variable so that tests can change it to refer to a local
// server when needed.
var metadataHost = "http://169.254.169.254"

// fetchMetadata fetches a single atom of data from the openstack instance metadata service.
// http://docs.amazonwebservices.com/AWSEC2/latest/UserGuide/AESDG-chapter-instancedata.html
// (the same specs is implemented in OpenStack, hence the reference)
func fetchMetadata(name string) (value string, err error) {
	uri := fmt.Sprintf("%s/2011-01-01/meta-data/%s", metadataHost, name)
	for a := shortAttempt.Start(); a.Next(); {
		var resp *http.Response
		resp, err = http.Get(uri)
		if err != nil {
			continue
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			err = fmt.Errorf("bad http response %v", resp.Status)
			continue
		}
		var data []byte
		data, err = ioutil.ReadAll(resp.Body)
		if err != nil {
			continue
		}
		return strings.TrimSpace(string(data)), nil
	}
	if err != nil {
		return "", fmt.Errorf("cannot get %q: %v", uri, err)
	}
	return
}
