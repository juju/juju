// Stub provider for OpenStack, using goose will be implemented here

package openstack

import (
	"errors"
	"fmt"
	"io/ioutil"
	"launchpad.net/goose/client"
	gooseerrors "launchpad.net/goose/errors"
	"launchpad.net/goose/identity"
	"launchpad.net/goose/nova"
	"launchpad.net/goose/swift"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/cloudinit"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/trivial"
	"launchpad.net/juju-core/version"
	"net/http"
	"strings"
	"sync"
	"time"
)

const mgoPort = 37017
const apiPort = 17070

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

	ecfgMutex             sync.Mutex
	ecfgUnlocked          *environConfig
	novaUnlocked          *nova.Client
	storageUnlocked       environs.Storage
	publicStorageUnlocked environs.Storage // optional.
}

var _ environs.Environ = (*environ)(nil)

type instance struct {
	e *environ
	*nova.Entity
}

func (inst *instance) String() string {
	return inst.Entity.Id
}

var _ environs.Instance = (*instance)(nil)

func (inst *instance) Id() state.InstanceId {
	return state.InstanceId(inst.Entity.Id)
}

func (inst *instance) DNSName() (string, error) {
	panic("DNSName not implemented")
}

func (inst *instance) WaitDNSName() (string, error) {
	panic("WaitDNSName not implemented")
}

func (inst *instance) OpenPorts(machineId string, ports []state.Port) error {
	panic("OpenPorts not implemented")
}

func (inst *instance) ClosePorts(machineId string, ports []state.Port) error {
	panic("ClosePorts not implemented")
}

func (inst *instance) Ports(machineId string) ([]state.Port, error) {
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

func (e *environ) Name() string {
	return e.name
}

func (e *environ) Storage() environs.Storage {
	e.ecfgMutex.Lock()
	storage := e.storageUnlocked
	e.ecfgMutex.Unlock()
	return storage
}

func (e *environ) PublicStorage() environs.StorageReader {
	e.ecfgMutex.Lock()
	defer e.ecfgMutex.Unlock()
	if e.publicStorageUnlocked == nil {
		return environs.EmptyStorage
	}
	return e.publicStorageUnlocked
}

func (e *environ) Bootstrap(uploadTools bool, cert, key []byte) error {
	password := e.Config().AdminSecret()
	if password == "" {
		return fmt.Errorf("admin-secret is required for bootstrap")
	}
	log.Printf("environs/openstack: bootstrapping environment %q", e.name)
	// If the state file exists, it might actually have just been
	// removed by Destroy, and eventual consistency has not caught
	// up yet, so we retry to verify if that is happening.
	var err error
	for a := shortAttempt.Start(); a.Next(); {
		_, err = e.loadState()
		if err != nil {
			break
		}
	}
	if err == nil {
		return fmt.Errorf("environment is already bootstrapped")
	}
	if _, notFound := err.(*environs.NotFoundError); !notFound {
		return fmt.Errorf("cannot query old bootstrap state: %v", err)
	}
	var tools *state.Tools
	if uploadTools {
		tools, err = environs.PutTools(e.Storage(), nil)
		if err != nil {
			return fmt.Errorf("cannot upload tools: %v", err)
		}
	} else {
		flags := environs.HighestVersion | environs.CompatVersion
		v := version.Current
		v.Series = e.Config().DefaultSeries()
		tools, err = environs.FindTools(e, v, flags)
		if err != nil {
			return fmt.Errorf("cannot find tools: %v", err)
		}
	}
	config, err := environs.BootstrapConfig(providerInstance, e.Config(), tools)
	if err != nil {
		return fmt.Errorf("unable to determine inital configuration: %v", err)
	}
	caCert, hasCert := e.Config().CACert()
	if !hasCert {
		return fmt.Errorf("no CA certificate in environment configuration")
	}
	inst, err := e.startInstance(&startInstanceParams{
		machineId: "0",
		info: &state.Info{
			Password: trivial.PasswordHash(password),
			CACert:   caCert,
		},
		apiInfo: &api.Info{
			Password: trivial.PasswordHash(password),
			CACert:   caCert,
		},
		tools:           tools,
		stateServer:     true,
		config:          config,
		stateServerCert: cert,
		stateServerKey:  key,
	})
	if err != nil {
		return fmt.Errorf("cannot start bootstrap instance: %v", err)
	}
	err = e.saveState(&bootstrapState{
		StateInstances: []state.InstanceId{inst.Id()},
	})
	if err != nil {
		// ignore error on StopInstance because the previous error is
		// more important.
		e.StopInstances([]environs.Instance{inst})
		return fmt.Errorf("cannot save state: %v", err)
	}
	// TODO make safe in the case of racing Bootstraps
	// If two Bootstraps are called concurrently, there's
	// no way to use Swift to make sure that only one succeeds.
	// Perhaps consider using SimpleDB for state storage
	// which would enable that possibility.

	return nil
}

func (e *environ) StateInfo() (*state.Info, *api.Info, error) {
	panic("StateInfo not implemented")
}

func (e *environ) Config() *config.Config {
	return e.ecfg().Config
}

func (e *environ) client(ecfg *environConfig, authMethodCfg AuthMethod) client.AuthenticatingClient {
	cred := &identity.Credentials{
		User:       ecfg.username(),
		Secrets:    ecfg.password(),
		Region:     ecfg.region(),
		TenantName: ecfg.tenantName(),
		URL:        ecfg.authURL(),
	}
	// authMethodCfg has already been validated so we know it's one of the values below.
	var authMethod identity.AuthMethod
	switch authMethodCfg {
	case AuthLegacy:
		authMethod = identity.AuthLegacy
	case AuthUserPass:
		authMethod = identity.AuthUserPass
	}
	return client.NewClient(cred, authMethod, nil)
}

func (e *environ) publicClient(ecfg *environConfig) client.Client {
	return client.NewPublicClient(ecfg.publicBucketURL(), nil)
}

func (e *environ) SetConfig(cfg *config.Config) error {
	ecfg, err := providerInstance.newConfig(cfg)
	if err != nil {
		return err
	}
	// At this point, the authentication method config value has been validated so we extract it's value here
	// to avoid having to validate again each time when creating the OpenStack client.
	var authMethodCfg AuthMethod
	e.ecfgMutex.Lock()
	defer e.ecfgMutex.Unlock()
	e.name = ecfg.Name()
	authMethodCfg = AuthMethod(ecfg.authMethod())
	e.ecfgUnlocked = ecfg

	novaClient := e.client(ecfg, authMethodCfg)
	e.novaUnlocked = nova.New(novaClient)

	// create new storage instances, existing instances continue
	// to reference their existing configuration.
	e.storageUnlocked = &storage{
		containerName: ecfg.controlBucket(),
		containerACL:  swift.Private,
		swift:         swift.New(e.client(ecfg, authMethodCfg))}
	if ecfg.publicBucket() != "" && ecfg.publicBucketURL() != "" {
		e.publicStorageUnlocked = &storage{
			containerName: ecfg.publicBucket(),
			containerACL:  swift.PublicRead,
			swift:         swift.New(e.publicClient(ecfg))}
	} else {
		e.publicStorageUnlocked = nil
	}

	return nil
}

func (e *environ) StartInstance(machineId string, info *state.Info, apiInfo *api.Info, tools *state.Tools) (environs.Instance, error) {
	return e.startInstance(&startInstanceParams{
		machineId: machineId,
		info:      info,
		apiInfo:   apiInfo,
		tools:     tools,
	})
}

type startInstanceParams struct {
	machineId       string
	info            *state.Info
	apiInfo         *api.Info
	tools           *state.Tools
	stateServer     bool
	config          *config.Config
	stateServerCert []byte
	stateServerKey  []byte
}

func (e *environ) userData(scfg *startInstanceParams) ([]byte, error) {
	cfg := &cloudinit.MachineConfig{
		StateServer:        scfg.stateServer,
		MongoPort:          mgoPort,
		APIPort:            apiPort,
		StateInfo:          scfg.info,
		APIInfo:            scfg.apiInfo,
		StateServerCert:    scfg.stateServerCert,
		StateServerKey:     scfg.stateServerKey,
		InstanceIdAccessor: "$(curl http://169.254.169.254/1.0/meta-data/instance-id)",
		ProviderType:       "openstack",
		DataDir:            "/var/lib/juju",
		Tools:              scfg.tools,
		MachineId:          scfg.machineId,
		AuthorizedKeys:     e.ecfg().AuthorizedKeys(),
		Config:             scfg.config,
	}
	cloudcfg, err := cloudinit.New(cfg)
	if err != nil {
		return nil, err
	}
	bytes, err := cloudcfg.Render()
	if err != nil {
		return nil, err
	}
	return bytes, nil
}

const (
	// Until image lookup is implemented, we'll use some pre-established, known values for starting instances.
	defaultFlavorId = "1" //m1.tiny
	// This is an existing image on Canonistack - smoser-cloud-images/ubuntu-quantal-12.10-i386-server-20121017
	defaultImageId = "0f602ea9-c09e-440c-9e29-cfae5635afa3"
)

// startInstance is the internal version of StartInstance, used by Bootstrap
// as well as via StartInstance itself.
func (e *environ) startInstance(scfg *startInstanceParams) (environs.Instance, error) {
	if scfg.tools == nil {
		var err error
		flags := environs.HighestVersion | environs.CompatVersion
		scfg.tools, err = environs.FindTools(e, version.Current, flags)
		if err != nil {
			return nil, err
		}
	}

	log.Printf("environs/openstack: starting machine %s in %q running tools version %q from %q",
		scfg.machineId, e.name, scfg.tools.Binary, scfg.tools.URL)
	// TODO(wallyworld) - implement spec lookup
	if strings.Contains(scfg.tools.Series, "unknown") || strings.Contains(scfg.tools.Series, "unknown") {
		return nil, fmt.Errorf("cannot find image for unknown series or architecture")
	}
	userData, err := e.userData(scfg)
	if err != nil {
		return nil, fmt.Errorf("cannot make user data: %v", err)
	}
	log.Debugf("environs/openstack: openstack user data: %q", userData)
	groups, err := e.setUpGroups(scfg.machineId)
	if err != nil {
		return nil, fmt.Errorf("cannot set up groups: %v", err)
	}
	var groupNames = make([]nova.SecurityGroupName, len(groups))
	for i, g := range groups {
		groupNames[i] = nova.SecurityGroupName{g.Name}
	}

	var server *nova.Entity
	for a := shortAttempt.Start(); a.Next(); {
		server, err = e.nova().RunServer(nova.RunServerOpts{
			Name: state.MachineEntityName(scfg.machineId),
			// TODO(wallyworld) - do not use hard coded image
			FlavorId:           defaultFlavorId,
			ImageId:            defaultImageId,
			UserData:           userData,
			SecurityGroupNames: groupNames,
		})
		if err == nil || !gooseerrors.IsNotFound(err) {
			break
		}
	}
	if err != nil {
		return nil, fmt.Errorf("cannot run instance: %v", err)
	}
	inst := &instance{e, server}
	log.Printf("environs/openstack: started instance %q", inst.Id())
	return inst, nil
}

func (e *environ) StopInstances(insts []environs.Instance) error {
	ids := make([]state.InstanceId, len(insts))
	for i, inst := range insts {
		instanceValue, ok := inst.(*instance)
		if !ok {
			return errors.New("Incompatible environs.Instance supplied")
		}
		ids[i] = instanceValue.Id()
	}
	return e.terminateInstances(ids)
}

func (e *environ) Instances(ids []state.InstanceId) ([]environs.Instance, error) {
	// TODO FIXME Instances must somehow be tagged to be part of the environment.
	// This is returning *all* instances, which means it's impossible to have two different
	// environments on the same account.
	if len(ids) == 0 {
		return nil, nil
	}
	insts := make([]environs.Instance, len(ids))
	servers, err := e.nova().ListServers(nil)
	if err != nil {
		return nil, err
	}
	for i, id := range ids {
		for j, _ := range servers {
			if servers[j].Id == string(id) {
				insts[i] = &instance{e, &servers[j]}
			}
		}
	}
	return insts, nil
}

func (e *environ) AllInstances() (insts []environs.Instance, err error) {
	// TODO FIXME Instances must somehow be tagged to be part of the environment.
	// This is returning *all* instances, which means it's impossible to have two different
	// environments on the same account.
	// TODO(wallyworld): add filtering to exclude deleted images etc
	servers, err := e.nova().ListServers(nil)
	if err != nil {
		return nil, err
	}
	for _, server := range servers {
		var s = server
		insts = append(insts, &instance{e, &s})
	}
	return insts, err
}

func (e *environ) Destroy(ensureInsts []environs.Instance) error {
	log.Printf("environs/openstack: destroying environment %q", e.name)
	insts, err := e.AllInstances()
	if err != nil {
		return fmt.Errorf("cannot get instances: %v", err)
	}
	found := make(map[state.InstanceId]bool)
	var ids []state.InstanceId
	for _, inst := range insts {
		ids = append(ids, inst.Id())
		found[inst.Id()] = true
	}

	// Add any instances we've been told about but haven't yet shown
	// up in the instance list.
	for _, inst := range ensureInsts {
		id := state.InstanceId(inst.(*instance).Id())
		if !found[id] {
			ids = append(ids, id)
			found[id] = true
		}
	}
	err = e.terminateInstances(ids)
	if err != nil {
		return err
	}

	// To properly observe e.storageUnlocked we need to get its value while
	// holding e.ecfgMutex. e.Storage() does this for us, then we convert
	// back to the (*storage) to access the private deleteAll() method.
	st := e.Storage().(*storage)
	return st.deleteAll()
}

func (e *environ) AssignmentPolicy() state.AssignmentPolicy {
	panic("AssignmentPolicy not implemented")
}

func (e *environ) globalGroupName() string {
	return fmt.Sprintf("%s-global", e.jujuGroupName())
}

func (e *environ) machineGroupName(machineId string) string {
	return fmt.Sprintf("%s-%s", e.jujuGroupName(), machineId)
}

func (e *environ) jujuGroupName() string {
	return fmt.Sprintf("juju-%s", e.name)
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

// setUpGroups creates the security groups for the new machine, and
// returns them.
//
// Instances are tagged with a group so they can be distinguished from
// other instances that might be running on the same OpenStack account.
// In addition, a specific machine security group is created for each
// machine, so that its firewall rules can be configured per machine.
func (e *environ) setUpGroups(machineId string) ([]nova.SecurityGroup, error) {
	jujuGroup, err := e.ensureGroup(e.jujuGroupName(),
		[]nova.RuleInfo{
			{
				IPProtocol: "tcp",
				FromPort:   22,
				ToPort:     22,
				Cidr:       "0.0.0.0/0",
			},
			{
				IPProtocol: "tcp",
				FromPort:   mgoPort,
				ToPort:     mgoPort,
				Cidr:       "0.0.0.0/0",
			},
			{
				IPProtocol: "tcp",
				FromPort:   1,
				ToPort:     65535,
			},
			{
				IPProtocol: "udp",
				FromPort:   1,
				ToPort:     65535,
			},
			{
				IPProtocol: "icmp",
				FromPort:   -1,
				ToPort:     -1,
			},
		})
	if err != nil {
		return nil, err
	}
	var machineGroup nova.SecurityGroup
	switch e.Config().FirewallMode() {
	case config.FwInstance:
		machineGroup, err = e.ensureGroup(e.machineGroupName(machineId), nil)
	case config.FwGlobal:
		machineGroup, err = e.ensureGroup(e.globalGroupName(), nil)
	}
	if err != nil {
		return nil, err
	}
	return []nova.SecurityGroup{jujuGroup, machineGroup}, nil
}

// zeroGroup holds the zero security group.
var zeroGroup nova.SecurityGroup

// ensureGroup returns the security group with name and perms.
// If a group with name does not exist, one will be created.
// If it exists, its permissions are set to perms.
func (e *environ) ensureGroup(name string, rules []nova.RuleInfo) (nova.SecurityGroup, error) {
	nova := e.nova()
	group, err := nova.CreateSecurityGroup(name, "juju group")
	if err != nil {
		if !gooseerrors.IsDuplicateValue(err) {
			return zeroGroup, err
		} else {
			// We just tried to create a duplicate group, so load the existing group.
			group, err = nova.SecurityGroupByName(name)
			if err != nil {
				return zeroGroup, err
			}
		}
	}
	// The group is created so now add the rules.
	for _, rule := range rules {
		rule.ParentGroupId = group.Id
		_, err := nova.CreateSecurityGroupRule(rule)
		if err != nil && !gooseerrors.IsDuplicateValue(err) {
			return zeroGroup, err
		}
	}
	return *group, nil
}

func (e *environ) terminateInstances(ids []state.InstanceId) error {
	if len(ids) == 0 {
		return nil
	}
	var firstErr error
	nova := e.nova()
	for _, id := range ids {
		err := nova.DeleteServer(string(id))
		if gooseerrors.IsNotFound(err) {
			err = nil
		}
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
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
