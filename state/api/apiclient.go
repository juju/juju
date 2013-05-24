// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"fmt"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/tomb"
	"strings"
	"sync"
)

// Machine represents the state of a machine.
type Machine struct {
	st  *State
	id  string
	doc params.Machine
}

// Client represents the client-accessible part of the state.
type Client struct {
	st *State
}

// Client returns an object that can be used
// to access client-specific functionality.
func (st *State) Client() *Client {
	return &Client{st}
}

// MachineInfo holds information about a machine.
type MachineInfo struct {
	InstanceId string // blank if not set.
}

// Status holds information about the status of a juju environment.
type Status struct {
	Machines map[string]MachineInfo
	// TODO the rest
}

// Status returns the status of the juju environment.
func (c *Client) Status() (*Status, error) {
	var s Status
	if err := c.st.call("Client", "", "Status", nil, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// ServiceSet sets configuration options on a service.
func (c *Client) ServiceSet(service string, options map[string]string) error {
	p := params.ServiceSet{
		ServiceName: service,
		Options:     options,
	}
	return c.st.call("Client", "", "ServiceSet", p, nil)
}

// Resolved clears errors on a unit.
func (c *Client) Resolved(unit string, retry bool) error {
	p := params.Resolved{
		UnitName: unit,
		Retry:    retry,
	}
	return c.st.call("Client", "", "Resolved", p, nil)
}

// ServiceSetYAML sets configuration options on a service
// given options in YAML format.
func (c *Client) ServiceSetYAML(service string, yaml string) error {
	p := params.ServiceSetYAML{
		ServiceName: service,
		Config:      yaml,
	}
	return c.st.call("Client", "", "ServiceSetYAML", p, nil)
}

// ServiceGet returns the configuration for the named service.
func (c *Client) ServiceGet(service string) (*params.ServiceGetResults, error) {
	var results params.ServiceGetResults
	params := params.ServiceGet{ServiceName: service}
	err := c.st.call("Client", "", "ServiceGet", params, &results)
	return &results, err
}

// AddRelation adds a relation between the specified endpoints and returns the relation info.
func (c *Client) AddRelation(endpoints ...string) (*params.AddRelationResults, error) {
	var addRelRes params.AddRelationResults
	params := params.AddRelation{Endpoints: endpoints}
	err := c.st.call("Client", "", "AddRelation", params, &addRelRes)
	return &addRelRes, err
}

// DestroyRelation removes the relation between the specified endpoints.
func (c *Client) DestroyRelation(endpoints ...string) error {
	params := params.DestroyRelation{Endpoints: endpoints}
	return c.st.call("Client", "", "DestroyRelation", params, nil)
}

// ServiceExpose changes the juju-managed firewall to expose any ports that
// were also explicitly marked by units as open.
func (c *Client) ServiceExpose(service string) error {
	params := params.ServiceExpose{ServiceName: service}
	return c.st.call("Client", "", "ServiceExpose", params, nil)
}

// ServiceUnexpose changes the juju-managed firewall to unexpose any ports that
// were also explicitly marked by units as open.
func (c *Client) ServiceUnexpose(service string) error {
	params := params.ServiceUnexpose{ServiceName: service}
	return c.st.call("Client", "", "ServiceUnexpose", params, nil)
}

// ServiceDeploy obtains the charm, either locally or from the charm store,
// and deploys it.
func (c *Client) ServiceDeploy(charmUrl string, serviceName string, numUnits int, configYAML string, cons constraints.Value) error {
	params := params.ServiceDeploy{
		ServiceName: serviceName,
		CharmUrl:    charmUrl,
		NumUnits:    numUnits,
		// BUG(lp:1162122): ConfigYAML has no tests.
		ConfigYAML:  configYAML,
		Constraints: cons,
	}
	return c.st.call("Client", "", "ServiceDeploy", params, nil)
}

// AddServiceUnits adds a given number of units to a service.
func (c *Client) AddServiceUnits(service string, numUnits int) ([]string, error) {
	args := params.AddServiceUnits{
		ServiceName: service,
		NumUnits:    numUnits,
	}
	results := new(params.AddServiceUnitsResults)
	err := c.st.call("Client", "", "AddServiceUnits", args, results)
	return results.Units, err
}

// DestroyServiceUnits decreases the number of units dedicated to a service.
func (c *Client) DestroyServiceUnits(unitNames []string) error {
	params := params.DestroyServiceUnits{unitNames}
	return c.st.call("Client", "", "DestroyServiceUnits", params, nil)
}

// ServiceDestroy destroys a given service.
func (c *Client) ServiceDestroy(service string) error {
	params := params.ServiceDestroy{
		ServiceName: service,
	}
	return c.st.call("Client", "", "ServiceDestroy", params, nil)
}

// GetServiceConstraints returns the constraints for the given service.
func (c *Client) GetServiceConstraints(service string) (constraints.Value, error) {
	results := new(params.GetServiceConstraintsResults)
	err := c.st.call("Client", "", "GetServiceConstraints", params.GetServiceConstraints{service}, results)
	return results.Constraints, err
}

// SetServiceConstraints specifies the constraints for the given service.
func (c *Client) SetServiceConstraints(service string, constraints constraints.Value) error {
	params := params.SetServiceConstraints{
		ServiceName: service,
		Constraints: constraints,
	}
	return c.st.call("Client", "", "SetServiceConstraints", params, nil)
}

// CharmInfo holds information about a charm.
type CharmInfo struct {
	Revision int
	URL      string
	Config   *charm.Config
	Meta     *charm.Meta
}

// CharmInfo returns information about the requested charm.
func (c *Client) CharmInfo(charmURL string) (*CharmInfo, error) {
	args := params.CharmInfo{CharmURL: charmURL}
	info := new(CharmInfo)
	if err := c.st.call("Client", "", "CharmInfo", args, info); err != nil {
		return nil, err
	}
	return info, nil
}

// EnvironmentInfo holds information about the Juju environment.
type EnvironmentInfo struct {
	DefaultSeries string
	ProviderType  string
	Name          string
}

// EnvironmentInfo returns details about the Juju environment.
func (c *Client) EnvironmentInfo() (*EnvironmentInfo, error) {
	info := new(EnvironmentInfo)
	err := c.st.call("Client", "", "EnvironmentInfo", nil, info)
	return info, err
}

// AllWatcher holds information allowing us to get Deltas describing changes
// to the entire environment.
type AllWatcher struct {
	client *Client
	id     *string
}

func newAllWatcher(client *Client, id *string) *AllWatcher {
	return &AllWatcher{client, id}
}

func (watcher *AllWatcher) Next() ([]params.Delta, error) {
	info := new(params.AllWatcherNextResults)
	err := watcher.client.st.call("AllWatcher", *watcher.id, "Next", nil, info)
	return info.Deltas, err
}

func (watcher *AllWatcher) Stop() error {
	return watcher.client.st.call("AllWatcher", *watcher.id, "Stop", nil, nil)
}

// WatchAll holds the id of the newly-created AllWatcher.
type WatchAll struct {
	AllWatcherId string
}

// WatchAll returns an AllWatcher, from which you can request the Next
// collection of Deltas.
func (c *Client) WatchAll() (*AllWatcher, error) {
	info := new(WatchAll)
	if err := c.st.call("Client", "", "WatchAll", nil, info); err != nil {
		return nil, err
	}
	return newAllWatcher(c, &info.AllWatcherId), nil
}

// GetAnnotations returns annotations that have been set on the given entity.
func (c *Client) GetAnnotations(tag string) (map[string]string, error) {
	args := params.GetAnnotations{tag}
	ann := new(params.GetAnnotationsResults)
	err := c.st.call("Client", "", "GetAnnotations", args, ann)
	return ann.Annotations, err
}

// SetAnnotations sets the annotation pairs on the given entity.
// Currently annotations are supported on machines, services,
// units and the environment itself.
func (c *Client) SetAnnotations(tag string, pairs map[string]string) error {
	args := params.SetAnnotations{tag, pairs}
	return c.st.call("Client", "", "SetAnnotations", args, nil)
}

// Machine returns a reference to the machine with the given id.
func (st *State) Machine(id string) (*Machine, error) {
	m := &Machine{
		st: st,
		id: id,
	}
	if err := m.Refresh(); err != nil {
		return nil, err
	}
	return m, nil
}

// Unit represents the state of a service unit.
type Unit struct {
	st   *State
	name string
	doc  params.Unit
}

// Unit returns a unit by name.
func (st *State) Unit(name string) (*Unit, error) {
	u := &Unit{
		st:   st,
		name: name,
	}
	if err := u.Refresh(); err != nil {
		return nil, err
	}
	return u, nil
}

// Login authenticates as the entity with the given name and password.
// Subsequent requests on the state will act as that entity.
// This method is usually called automatically by Open.
func (st *State) Login(tag, password string) error {
	return st.call("Admin", "", "Login", &params.Creds{
		AuthTag:  tag,
		Password: password,
	}, nil)
}

// Id returns the machine id.
func (m *Machine) Id() string {
	return m.id
}

// Tag returns a name identifying the machine that is safe to use
// as a file name.  The returned name will be different from other
// Tag values returned by any other entities from the same state.
func (m *Machine) Tag() string {
	return MachineTag(m.Id())
}

// MachineTag returns the tag for the
// machine with the given id.
func MachineTag(id string) string {
	return fmt.Sprintf("machine-%s", id)
}

// Refresh refreshes the contents of the machine from the underlying
// state. TODO(rog) It returns a NotFoundError if the machine has been removed.
func (m *Machine) Refresh() error {
	return m.st.call("Machine", m.id, "Get", nil, &m.doc)
}

// String returns the machine's id.
func (m *Machine) String() string {
	return m.id
}

// InstanceId returns the provider specific instance id for this machine
// and whether it has been set.
func (m *Machine) InstanceId() (string, bool) {
	return m.doc.InstanceId, m.doc.InstanceId != ""
}

// SetAgentAlive signals that the agent for machine m is alive. It
// returns the started pinger.
func (m *Machine) SetAgentAlive() (*Pinger, error) {
	var id params.PingerId
	err := m.st.call("Machine", m.id, "SetAgentAlive", nil, &id)
	if err != nil {
		return nil, err
	}
	return &Pinger{
		st: m.st,
		id: id.PingerId,
	}, nil
}

// EnsureDead sets the machine lifecycle to Dead if it is Alive or Dying.
// It does nothing otherwise. EnsureDead will fail if the machine has
// principal units assigned, or if the machine has JobManageEnviron.
// If the machine has assigned units, EnsureDead will return
// a CodeHasAssignedUnits error.
func (m *Machine) EnsureDead() error {
	return m.st.call("Machine", m.id, "EnsureDead", nil, nil)
}

// SetStatus sets the status of the machine.
func (m *Machine) SetStatus(status params.Status, info string) error {
	return m.st.call("Machine", m.id, "SetStatus", &params.SetStatus{
		Status: status,
		Info:   info,
	}, nil)
}

// Life returns whether the machine is "alive", "dying" or "dead".
func (m *Machine) Life() params.Life {
	return m.doc.Life
}

// Pinger periodically reports that a specific key is alive, so that
// watchers interested on that fact can react appropriately.
type Pinger struct {
	st *State
	id string
}

// Stop stops the p's periodical ping. Watchers will not notice p has
// stopped pinging until the previous ping times out.
func (p *Pinger) Stop() error {
	return p.st.call("Pinger", p.id, "Stop", nil, nil)
}

// SetPassword sets the password for the machine's agent.
func (m *Machine) SetPassword(password string) error {
	return m.st.call("Machine", m.id, "SetPassword", &params.Password{
		Password: password,
	}, nil)
}

func (m *Machine) Watch() *EntityWatcher {
	return newEntityWatcher(m.st, "Machine", m.id)
}

// commonWatcher implements common watcher logic in one place to
// reduce code duplication, but it's not in fact a complete watcher;
// it's intended for embedding.
type commonWatcher struct {
	tomb tomb.Tomb
	wg   sync.WaitGroup
	in   chan interface{}

	// These fields must be set by the embedding watcher, before
	// calling init().

	// newResult must return a pointer to a value of the type returned
	// by the watcher's Next call.
	newResult func() interface{}

	// call should invoke the given API method, placing the call's
	// returned value in result (if any).
	call func(method string, result interface{}) error
}

// init must be called to initialize an embedded commonWatcher's
// fields. Make sure newResult and call fields are set beforehand.
func (w *commonWatcher) init() {
	w.in = make(chan interface{})
	if w.newResult == nil {
		panic("newResult must me set")
	}
	if w.call == nil {
		panic("call must be set")
	}
}

// commonLoop implements the loop structure common to the client
// watchers. It should be started in a separate goroutine by any
// watcher that embeds commonWatcher. It kills the commonWatcher's
// tomb when an error occurs.
func (w *commonWatcher) commonLoop() {
	defer close(w.in)
	w.wg.Add(1)
	go func() {
		// When the watcher has been stopped, we send a Stop request
		// to the server, which will remove the watcher and return a
		// CodeStopped error to any currently outstanding call to
		// Next. If a call to Next happens just after the watcher has
		// been stopped, we'll get a CodeNotFound error; Either way
		// we'll return, wait for the stop request to complete, and
		// the watcher will die with all resources cleaned up.
		defer w.wg.Done()
		<-w.tomb.Dying()
		if err := w.call("Stop", nil); err != nil {
			log.Errorf("state/api: error trying to stop watcher: %v", err)
		}
	}()
	w.wg.Add(1)
	go func() {
		// Because Next blocks until there are changes, we need to
		// call it in a separate goroutine, so the watcher can be
		// stopped normally.
		defer w.wg.Done()
		for {
			result := w.newResult()
			err := w.call("Next", &result)
			if err != nil {
				if code := ErrCode(err); code == CodeStopped || code == CodeNotFound {
					if w.tomb.Err() != tomb.ErrStillAlive {
						// The watcher has been stopped at the client end, so we're
						// expecting one of the above two kinds of error.
						// We might see the same errors if the server itself
						// has been shut down, in which case we leave them
						// untouched.
						err = tomb.ErrDying
					}
				}
				// Something went wrong, just report the error and bail out.
				w.tomb.Kill(err)
				return
			}
			select {
			case <-w.tomb.Dying():
				return
			case w.in <- result:
				// Report back the result we just got.
			}
		}
	}()
	w.wg.Wait()
}

func (w *commonWatcher) Stop() error {
	w.tomb.Kill(nil)
	return w.tomb.Wait()
}

func (w *commonWatcher) Err() error {
	return w.tomb.Err()
}

type EntityWatcher struct {
	commonWatcher
	st    *State
	etype string
	eid   string
	out   chan struct{}
}

func newEntityWatcher(st *State, etype, id string) *EntityWatcher {
	w := &EntityWatcher{
		st:    st,
		etype: etype,
		eid:   id,
		out:   make(chan struct{}),
	}
	go func() {
		defer w.tomb.Done()
		defer close(w.out)
		defer w.wg.Wait() // Wait for watcher to be stopped.
		w.tomb.Kill(w.loop())
	}()
	return w
}

func (w *EntityWatcher) loop() error {
	var id params.EntityWatcherId
	if err := w.st.call(w.etype, w.eid, "Watch", nil, &id); err != nil {
		return err
	}
	// No results for this watcher type.
	w.newResult = func() interface{} { return nil }
	w.call = func(request string, result interface{}) error {
		return w.st.call("EntityWatcher", id.EntityWatcherId, request, nil, result)
	}
	w.commonWatcher.init()
	go w.commonLoop()

	// Watch calls Next internally at the server-side, so we expect
	// changes right away.
	out := w.out
	for {
		select {
		case _, ok := <-w.in:
			if !ok {
				// The tomb is already killed with the correct error
				// at this point, so just return.
				return nil
			}
			// We have received changes, so send them out.
			out = w.out
		case out <- struct{}{}:
			// Wait until we have new changes to send.
			out = nil
		}
	}
	panic("unreachable")
}

// Changes returns a channel that receives a value when a given entity
// changes in some way.
func (w *EntityWatcher) Changes() <-chan struct{} {
	return w.out
}

// Refresh refreshes the contents of the Unit from the underlying
// state. TODO(rog) It returns a NotFoundError if the unit has been removed.
func (u *Unit) Refresh() error {
	return u.st.call("Unit", u.name, "Get", nil, &u.doc)
}

// SetPassword sets the password for the unit's agent.
func (u *Unit) SetPassword(password string) error {
	return u.st.call("Unit", u.name, "SetPassword", &params.Password{
		Password: password,
	}, nil)
}

// UnitTag returns the tag for the
// unit with the given name.
func UnitTag(unitName string) string {
	return "unit-" + strings.Replace(unitName, "/", "-", -1)
}

// Tag returns a name identifying the unit that is safe to use
// as a file name.  The returned name will be different from other
// Tag values returned by any other entities from the same state.
func (u *Unit) Tag() string {
	return UnitTag(u.name)
}

// DeployerTag returns the tag of the agent responsible for deploying
// the unit. If no such entity can be determined, false is returned.
func (u *Unit) DeployerTag() (string, bool) {
	return u.doc.DeployerTag, u.doc.DeployerTag != ""
}
