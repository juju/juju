// Stub provider for OpenStack, using goose will be implemented here

package openstack

import (
	"fmt"
	"io/ioutil"
	"launchpad.net/goose/client"
	"launchpad.net/goose/identity"
	"launchpad.net/goose/nova"
	"launchpad.net/goose/swift"
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

	ecfgMutex     sync.Mutex
	ecfgUnlocked  *environConfig
	novaUnlocked  *nova.Client
	swiftUnlocked *swift.Client
}

var _ environs.Environ = (*environ)(nil)

type instance struct {
	e *environ
	*nova.Entity
}

func (inst *instance) String() string {
	return inst.Id()
}

var _ environs.Instance = (*instance)(nil)

func (inst *instance) Id() string {
	return inst.Entity.Id
}

func (inst *instance) DNSName() (string, error) {
	panic("DNSName not implemented")
}

func (inst *instance) WaitDNSName() (string, error) {
	panic("WaitDNSName not implemented")
}

func (inst *instance) OpenPorts(machineId int, ports []state.Port) error {
	panic("OpenPorts not implemented")
}

func (inst *instance) ClosePorts(machineId int, ports []state.Port) error {
	panic("ClosePorts not implemented")
}

func (inst *instance) Ports(machineId int) ([]state.Port, error) {
	panic("Ports not implemented")
}

func (e *environ) ecfg() *environConfig {
	e.ecfgMutex.Lock()
	ecfg := e.ecfgUnlocked
	e.ecfgMutex.Unlock()
	return ecfg
}

func (e *environ) nova() *nova.Client {
	e.ecfgMutex.Lock()
	nova := e.novaUnlocked
	e.ecfgMutex.Unlock()
	return nova
}

func (e *environ) swift() *swift.Client {
	e.ecfgMutex.Lock()
	swift := e.swiftUnlocked
	e.ecfgMutex.Unlock()
	return swift
}

func (e *environ) Name() string {
	return e.name
}

func (e *environ) Bootstrap(uploadTools bool, cert, key []byte) error {
	panic("Bootstrap not implemented")
}

func (e *environ) StateInfo() (*state.Info, error) {
	panic("StateInfo not implemented")
}

func (e *environ) Config() *config.Config {
	return e.ecfg().Config
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

	cred := &identity.Credentials{
		User:       ecfg.username(),
		Secrets:    ecfg.password(),
		Region:     ecfg.region(),
		TenantName: ecfg.tenantName(),
		URL:        ecfg.authURL(),
	}
	// TODO: do not hard code authentication type
	client := client.NewClient(cred, identity.AuthUserPass)
	e.novaUnlocked = nova.New(client)
	e.swiftUnlocked = swift.New(client)
	return nil
}

func (e *environ) StartInstance(machineId int, info *state.Info, tools *state.Tools) (environs.Instance, error) {
	panic("StartInstance not implemented")
}

func (e *environ) StopInstances([]environs.Instance) error {
	panic("StopInstances not implemented")
}

func (e *environ) Instances(ids []string) ([]environs.Instance, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	insts := make([]environs.Instance, len(ids))
	servers, err := e.nova().ListServers()
	if err != nil {
		return nil, err
	}
	for i, id := range ids {
		for j, _ := range *servers {
			if (*servers)[j].Id == id {
				insts[i] = &instance{e, &(*servers)[j]}
			}
		}
	}
	return insts, nil
}

func (e *environ) AllInstances() (insts []environs.Instance, err error) {
	// TODO: add filtering to exclude deleted images etc
	servers, err := e.nova().ListServers()
	if err != nil {
		return nil, err
	}
	// TODO: WHY DOESN't THIS WORK?
	//	for _, server := range *servers {
	//		insts = append(insts, &instance{e, &server})
	//	}
	for i := range *servers {
		insts = append(insts, &instance{e, &(*servers)[i]})
	}

	return insts, err
}

func (e *environ) Storage() environs.Storage {
	panic("Storage not implemented")
}

func (e *environ) PublicStorage() environs.StorageReader {
	panic("PublicStorage not implemented")
}

func (e *environ) Destroy(insts []environs.Instance) error {
	log.Printf("environs/openstack: destroying environment %q", e.name)
	return nil
}

func (e *environ) AssignmentPolicy() state.AssignmentPolicy {
	panic("AssignmentPolicy not implemented")
}

func (e *environ) OpenPorts(ports []state.Port) error {
	panic("OpenPorts not implemented")
}

func (e *environ) ClosePorts(ports []state.Port) error {
	panic("ClosePorts not implemented")
}

func (e *environ) Ports() ([]state.Port, error) {
	panic("Ports not implemented")
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
