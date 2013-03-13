package maas

import (
	"errors"
	"fmt"
	"launchpad.net/gomaasapi"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/trivial"
	"launchpad.net/juju-core/version"
	"net/url"
	"sync"
	"time"
)

const (
	mgoPort     = 37017
	apiPort     = 17070
	jujuDataDir = "/var/lib/juju"
)

type maasEnviron struct {
	name string

	// ecfgMutext protects the *Unlocked fields below.
	ecfgMutex sync.Mutex

	ecfgUnlocked       *maasEnvironConfig
	maasClientUnlocked *gomaasapi.MAASObject
	storageUnlocked    environs.Storage
}

var _ environs.Environ = (*maasEnviron)(nil)

var couldNotAllocate = errors.New("Could not allocate MAAS environment object.")

func NewEnviron(cfg *config.Config) (*maasEnviron, error) {
	env := new(maasEnviron)
	if env == nil {
		return nil, couldNotAllocate
	}
	err := env.SetConfig(cfg)
	if err != nil {
		return nil, err
	}
	env.storageUnlocked = NewStorage(env)
	return env, nil
}

func (env *maasEnviron) Name() string {
	return env.name
}

// quiesceStateFile waits (up to a few seconds) for any existing state file to
// disappear.
//
// This is used when bootstrapping, to deal with any previous state file that
// may have been removed by a Destroy that hasn't reached its eventual
// consistent state yet.
func (env *maasEnviron) quiesceStateFile() error {
	// This was all cargo-culted off the EC2 provider.
	var err error
	retry := trivial.AttemptStrategy{
		Total: 5 * time.Second,
		Delay: 200 * time.Millisecond,
	}
	for a := retry.Start(); err == nil && a.Next(); {
		_, err = env.loadState()
	}
	if err == nil {
		// The state file outlived the timeout.  Looks like it wasn't
		// being destroyed after all.
		return fmt.Errorf("environment is already bootstrapped")
	}
	if _, notFound := err.(*environs.NotFoundError); !notFound {
		return fmt.Errorf("cannot query old bootstrap state: %v", err)
	}
	// Got to this point?  Then the error was "not found," which is the
	// state we're looking for.
	return nil
}

// uploadTools builds the current version of the juju tools and uploads them
// to the environment's Storage.
func (env *maasEnviron) uploadTools() (*state.Tools, error) {
	tools, err := environs.PutTools(env.Storage(), nil)
	if err != nil {
		return nil, fmt.Errorf("cannot upload tools: %v", err)
	}
	return tools, nil
}

// findTools looks for a current version of the juju tools that is already
// uploaded in the environment.
func (env *maasEnviron) findTools() (*state.Tools, error) {
	flags := environs.HighestVersion | environs.CompatVersion
	v := version.Current
	v.Series = env.Config().DefaultSeries()
	tools, err := environs.FindTools(env, v, flags)
	if err != nil {
		return nil, fmt.Errorf("cannot find tools: %v", err)
	}
	return tools, nil
}

// getMongoURL returns the URL to the appropriate MongoDB instance.
func (env *maasEnviron) getMongoURL(tools *state.Tools) string {
	v := version.Current
	v.Series = tools.Series
	v.Arch = tools.Arch
	return environs.MongoURL(env, v)
}

// Suppress compiler errors for unused variables.
// TODO: Eliminate all usage of this function.  It's just for development.
func unused(...interface{}) {}

// startBootstrapNode starts the juju bootstrap node for this environment.
func (env *maasEnviron) startBootstrapNode(tools *state.Tools, cert, key []byte, password string) (environs.Instance, error) {
	config, err := environs.BootstrapConfig(env.Provider(), env.Config(), tools)
	if err != nil {
		return nil, fmt.Errorf("unable to determine initial configuration: %v", err)
	}
	caCert, hasCert := env.Config().CACert()
	if !hasCert {
		return nil, fmt.Errorf("no CA certificate in environment configuration")
	}
	mongoURL := env.getMongoURL(tools)
	stateInfo := state.Info{
		Password: trivial.PasswordHash(password),
		CACert:   caCert,
	}
	apiInfo := api.Info{
		Password: trivial.PasswordHash(password),
		CACert:   caCert,
	}
	// TODO: Obtain userdata based on mongoURL cert/key, and config
	unused(mongoURL, cert, key, config)
	var userdata []byte
	inst, err := env.obtainNode("0", &stateInfo, &apiInfo, tools, userdata)
	if err != nil {
		return nil, fmt.Errorf("cannot start bootstrap instance: %v", err)
	}
	return inst, nil
}

// Bootstrap is specified in the Environ interface.
func (env *maasEnviron) Bootstrap(uploadTools bool, stateServerCert, stateServerKey []byte) error {
	// This was all cargo-culted from the EC2 provider.
	password := env.Config().AdminSecret()
	if password == "" {
		return fmt.Errorf("admin-secret is required for bootstrap")
	}
	log.Printf("environs/maas: bootstrapping environment %q.", env.Name())
	err := env.quiesceStateFile()
	if err != nil {
		return err
	}
	var tools *state.Tools
	if uploadTools {
		tools, err = env.uploadTools()
	} else {
		tools, err = env.findTools()
	}
	if err != nil {
		return err
	}
	inst, err := env.startBootstrapNode(tools, stateServerCert, stateServerKey, password)
	if err != nil {
		return err
	}
	err = env.saveState(&bootstrapState{StateInstances: []state.InstanceId{inst.Id()}})
	if err != nil {
		env.stopInstance(inst)
		return fmt.Errorf("cannot save state: %v", err)
	}

	// TODO make safe in the case of racing Bootstraps
	// If two Bootstraps are called concurrently, there's
	// no way to make sure that only one succeeds.

	return nil
}

// StateInfo is specified in the Environ interface.
func (*maasEnviron) StateInfo() (*state.Info, *api.Info, error) {
	panic("Not implemented.")
}

// ecfg returns the environment's maasEnvironConfig, and protects it with a
// mutex.
func (env *maasEnviron) ecfg() *maasEnvironConfig {
	env.ecfgMutex.Lock()
	defer env.ecfgMutex.Unlock()
	return env.ecfgUnlocked
}

// Config is specified in the Environ interface.
func (env *maasEnviron) Config() *config.Config {
	return env.ecfg().Config
}

// SetConfig is specified in the Environ interface.
func (env *maasEnviron) SetConfig(cfg *config.Config) error {
	ecfg, err := env.Provider().(*maasEnvironProvider).newConfig(cfg)
	if err != nil {
		return err
	}

	env.ecfgMutex.Lock()
	defer env.ecfgMutex.Unlock()

	env.name = cfg.Name()
	env.ecfgUnlocked = ecfg

	authClient, err := gomaasapi.NewAuthenticatedClient(ecfg.MAASServer(), ecfg.MAASOAuth())
	if err != nil {
		return err
	}
	env.maasClientUnlocked = gomaasapi.NewMAAS(*authClient)

	return nil
}

// acquireNode allocates a node from the MAAS.
func (environ *maasEnviron) acquireNode() (gomaasapi.MAASObject, error) {
	retry := trivial.AttemptStrategy{
		Total: 5 * time.Second,
		Delay: 200 * time.Millisecond,
	}
	var result gomaasapi.JSONObject
	// Initialize err to a non-nil value as a sentinel for the following
	// loop.
	var err error = fmt.Errorf("(no error)")
	for a := retry.Start(); a.Next() && err != nil; {
		client := environ.maasClientUnlocked.GetSubObject("nodes/")
		result, err = client.CallPost("acquire", nil)
	}
	if err != nil {
		return gomaasapi.MAASObject{}, err
	}
	node, err := result.GetMAASObject()
	if err != nil {
		msg := fmt.Errorf("unexpected result from 'acquire' on MAAS API: %v", err)
		return gomaasapi.MAASObject{}, msg
	}
	return node, nil
}

// startNode installs and boots a node.
func (environ *maasEnviron) startNode(node gomaasapi.MAASObject, tools *state.Tools, userdata []byte) error {
	retry := trivial.AttemptStrategy{
		Total: 5 * time.Second,
		Delay: 200 * time.Millisecond,
	}
	params := url.Values{
		"distro_series": {tools.Series},
		// TODO: How do we pass user_data!?
		//"user_data": {userdata},
	}
	// Initialize err to a non-nil value as a sentinel for the following
	// loop.
	var err error = fmt.Errorf("(no error)")
	for a := retry.Start(); a.Next() && err != nil; {
		_, err = node.CallPost("start", params)
	}
	return err
}

// obtainNode allocates and starts a MAAS node.  It is used both for the
// implementation of StartInstance, and to initialize the bootstrap node.
func (environ *maasEnviron) obtainNode(machineId string, stateInfo *state.Info, apiInfo *api.Info, tools *state.Tools, userdata []byte) (*maasInstance, error) {

	log.Printf("environs/maas: starting machine %s in $q running tools version %q from %q", machineId, environ.name, tools.Binary, tools.URL)

	node, err := environ.acquireNode()
	if err != nil {
		return nil, fmt.Errorf("cannot run instances: %v", err)
	}
	instance := maasInstance{&node, environ}

	err = environ.startNode(node, tools, userdata)
	if err != nil {
		environ.StopInstances([]environs.Instance{&instance})
		return nil, fmt.Errorf("cannot start instance: %v", err)
	}
	log.Printf("environs/maas: started instance %q", instance.Id())
	return &instance, nil
}

// StartInstance is specified in the Environ interface.
func (environ *maasEnviron) StartInstance(machineId string, info *state.Info, apiInfo *api.Info, tools *state.Tools) (environs.Instance, error) {
	if tools == nil {
		flags := environs.HighestVersion | environs.CompatVersion
		var err error
		tools, err = environs.FindTools(environ, version.Current, flags)
		if err != nil {
			return nil, err
		}
	}
	// TODO: Obtain userdata.
	var userdata []byte
	return environ.obtainNode(machineId, info, apiInfo, tools, userdata)
}

// StopInstances is specified in the Environ interface.
func (environ *maasEnviron) StopInstances(instances []environs.Instance) error {
	// Shortcut to exit quickly if 'instances' is an empty slice or nil.
	if len(instances) == 0 {
		return nil
	}
	// Tell MAAS to release each of the instances.  If there are errors,
	// return only the first one (but release all instances regardless).
	// Note that releasing instances also turns them off.
	var firstErr error
	for _, instance := range instances {
		err := environ.releaseInstance(instance)
		if firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// stopInstance stops a single instance.  Avoid looping over this in bulk
// operations: use StopInstances for those.
func (environ *maasEnviron) stopInstance(inst environs.Instance) error {
	maasInst := inst.(*maasInstance)
	maasObj := maasInst.maasObject
	_, err := maasObj.CallPost("stop", nil)
	if err != nil {
		log.Debugf("environs/maas: error stopping instance %v", maasInst)
	}

	return err
}

// releaseInstance releases a single instance.
func (environ *maasEnviron) releaseInstance(inst environs.Instance) error {
	maasInst := inst.(*maasInstance)
	maasObj := maasInst.maasObject
	_, err := maasObj.CallPost("release", nil)
	if err != nil {
		log.Debugf("environs/maas: error releasing instance %v", maasInst)
	}
	return err
}

// Instances returns the environs.Instance objects corresponding to the given
// slice of state.InstanceId.  Similar to what the ec2 provider does,
// Instances returns nil if the given slice is empty or nil.
func (environ *maasEnviron) Instances(ids []state.InstanceId) ([]environs.Instance, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	return environ.instances(ids)
}

// instances is an internal method which returns the instances matching the
// given instance ids or all the instances if 'ids' is empty.
// If the some of the intances could not be found, it returns the instance
// that could be found plus the error environs.ErrPartialInstances in the error
// return.
func (environ *maasEnviron) instances(ids []state.InstanceId) ([]environs.Instance, error) {
	nodeListing := environ.maasClientUnlocked.GetSubObject("nodes")
	filter := getSystemIdValues(ids)
	listNodeObjects, err := nodeListing.CallGet("list", filter)
	if err != nil {
		return nil, err
	}
	listNodes, err := listNodeObjects.GetArray()
	if err != nil {
		return nil, err
	}
	instances := make([]environs.Instance, len(listNodes))
	for index, nodeObj := range listNodes {
		node, err := nodeObj.GetMAASObject()
		if err != nil {
			return nil, err
		}
		instances[index] = &maasInstance{
			maasObject: &node,
			environ:    environ,
		}
	}
	if len(ids) != 0 && len(ids) != len(instances) {
		return instances, environs.ErrPartialInstances
	}
	return instances, nil
}

// AllInstances returns all the environs.Instance in this provider.
func (environ *maasEnviron) AllInstances() ([]environs.Instance, error) {
	return environ.instances(nil)
}

// Storage is defined by the Environ interface.
func (env *maasEnviron) Storage() environs.Storage {
	env.ecfgMutex.Lock()
	defer env.ecfgMutex.Unlock()
	return env.storageUnlocked
}

// PublicStorage is defined by the Environ interface.
func (env *maasEnviron) PublicStorage() environs.StorageReader {
	// MAAS does not have a shared storage.
	return environs.EmptyStorage
}

func (environ *maasEnviron) Destroy(ensureInsts []environs.Instance) error {
	log.Printf("environs/maas: destroying environment %q", environ.name)
	insts, err := environ.AllInstances()
	if err != nil {
		return fmt.Errorf("cannot get instances: %v", err)
	}
	found := make(map[state.InstanceId]bool)
	for _, inst := range insts {
		found[inst.Id()] = true
	}

	// Add any instances we've been told about but haven't yet shown
	// up in the instance list.
	for _, inst := range ensureInsts {
		id := state.InstanceId(inst.(*maasInstance).Id())
		if !found[id] {
			insts = append(insts, inst)
			found[id] = true
		}
	}
	err = environ.StopInstances(insts)
	if err != nil {
		return err
	}

	// To properly observe e.storageUnlocked we need to get its value while
	// holding e.ecfgMutex. e.Storage() does this for us, then we convert
	// back to the (*storage) to access the private deleteAll() method.
	st := environ.Storage().(*maasStorage)
	return st.deleteAll()
}

func (*maasEnviron) AssignmentPolicy() state.AssignmentPolicy {
	panic("Not implemented.")
}

func (*maasEnviron) OpenPorts([]state.Port) error {
	panic("Not implemented.")
}

func (*maasEnviron) ClosePorts([]state.Port) error {
	panic("Not implemented.")
}

func (*maasEnviron) Ports() ([]state.Port, error) {
	panic("Not implemented.")
}

func (*maasEnviron) Provider() environs.EnvironProvider {
	return &providerInstance
}
