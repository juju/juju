// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The presence package works on the premise that an agent it alive
// if it has a current connection to one of the API servers.
//
// This package handles all of the logic around collecting an organising
// the information around all the connections made to the API servers.
package presence

import (
	"sync"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
)

// Recorder records the presence status of every apiserver connection
// for the agents.
type Recorder interface {
	// Disable clears the entries and marks the recorder as disabled. Note
	// that all agents will have a recorder, but they are only enabled for
	// API server agents.
	Disable()

	// Enable marks the recorder as enabled.
	Enable()

	// IsEnabled returns whether or not the recorder is enabled.
	IsEnabled() bool

	// Connect adds an entry for the specified agent.
	// The server and agent strings are the stringified machine and unit tags.
	// The model is the UUID for the model.
	Connect(server, model, agent string, id uint64, controllerAgent bool, userData string)

	// Disconnect removes the entry for the specified connection id.
	Disconnect(server string, id uint64)

	// Activity updates the last seen timestamp for the connection specified.
	Activity(server string, id uint64)

	// ServerDown marks all connections on the specified server as unknown.
	ServerDown(server string)

	// UpdateServer replaces all known connections for the specified server
	// with the connections specified. If any of the connections are not for
	// the specified server, an error is returned.
	UpdateServer(server string, connections []Value) error

	// Connections returns all connections info that the recorder has.
	Connections() Connections
}

// Connections provides a way to slice the full presence understanding
// across various axis like server, model and agent.
type Connections interface {
	// ForModel will return the connections just for the specified model UUID.
	ForModel(model string) Connections

	// ForServer will return just the connections for agents connected to the specified
	// server. The server is a stringified machine tag for the API server.
	ForServer(server string) Connections

	// ForAgent returns the connections for the specified agent in the model.
	// The agent is the stringified machine or unit tag.
	ForAgent(agent string) Connections

	// Count returns the number of connections.
	Count() int

	// Models returns all the model UUIDs that have connections.
	Models() []string

	// Servers returns all the API servers that have connections in this
	// collection.
	Servers() []string

	// Agents returns all the stringified agent tags that have connections
	// in this collection.
	Agents() []string

	// For a given non controller agent, return the Status for that agent.
	AgentStatus(agent string) (Status, error)

	// Values returns the connection information for this collection.
	Values() []Value
}

// Status represents the state of a given agent's presence.
type Status int

const (
	// Unknown means that the agent specified is not in the collection.
	Unknown Status = iota

	// Missing means that the agent was connected, but the server that it was
	// connected to has gone away and not yet come back.
	Missing

	// Alive means that the connection is active.
	Alive
)

// String implements Stringer.
func (s Status) String() string {
	switch s {
	case Unknown:
		return "unknown"
	case Missing:
		return "missing"
	case Alive:
		return "alive"
	default:
		return ""
	}
}

// Value holds the information about a single agent connection to an apiserver
// machine.
type Value struct {
	// Model is the model UUID.
	Model string

	// Server is the stringified machine tag of the API server.
	Server string

	// Agent is the stringified machine, unit, or application tag of the agent.
	Agent string

	// ControllerAgent is true if the agent is in the controller model.
	ControllerAgent bool

	// ConnectionID is the unique identifier given by the API server.
	ConnectionID uint64

	// Status is either Missing or Alive.
	Status Status

	// UserData is the user data provided with the Login API call.
	UserData string

	// LastSeen is the timestamp when the connection was added using
	// Connect, or the last time Activity was called.
	LastSeen time.Time
}

type connections struct {
	model  string
	values []Value
}

// Clock provides an interface for dealing with clocks.
type Clock interface {
	// Now returns the current clock time.
	Now() time.Time
}

// New returns a new empty Recorder.
func New(clock Clock) Recorder {
	return &recorder{clock: clock}
}

type recorder struct {
	mu      sync.Mutex
	enabled bool
	clock   Clock
	entries []Value
}

// Disable implements Recorder.
func (r *recorder) Disable() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.enabled = false
	r.entries = nil
}

// Enable implements Recorder.
func (r *recorder) Enable() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.enabled = true

}

// IsEnabled implements Recorder.
func (r *recorder) IsEnabled() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.enabled
}

func (r *recorder) findIndex(server string, id uint64) int {
	for i, e := range r.entries {
		if e.Server == server && e.ConnectionID == id {
			return i
		}
	}
	return -1
}

// Connect implements Recorder.
func (r *recorder) Connect(server, model, agent string, id uint64, controllerAgent bool, userData string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.enabled {
		return
	}

	pos := r.findIndex(server, id)
	if pos == -1 {
		r.entries = append(r.entries, Value{
			Model:           model,
			Server:          server,
			Agent:           agent,
			ControllerAgent: controllerAgent,
			ConnectionID:    id,
			Status:          Alive,
			UserData:        userData,
			LastSeen:        r.clock.Now(),
		})
	} else {
		// Need to access the value in the array, not a copy of it.
		r.entries[pos].Status = Alive
		r.entries[pos].LastSeen = r.clock.Now()
	}
}

// Disconnect implements Recorder.
func (r *recorder) Disconnect(server string, id uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.enabled {
		return
	}

	if pos := r.findIndex(server, id); pos >= 0 {
		if pos == 0 {
			r.entries = r.entries[1:]
		} else {
			r.entries = append(r.entries[0:pos], r.entries[pos+1:]...)
		}
	}
}

// Activity implements Recorder.
func (r *recorder) Activity(server string, id uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.enabled {
		return
	}

	if pos := r.findIndex(server, id); pos >= 0 {
		r.entries[pos].LastSeen = r.clock.Now()
	}
}

// ServerDown implements Recorder.
func (r *recorder) ServerDown(server string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.enabled {
		return
	}

	for i, value := range r.entries {
		if value.Server == server {
			r.entries[i].Status = Missing
		}
	}
}

// UpdateServer implements Recorder.
func (r *recorder) UpdateServer(server string, connections []Value) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.enabled {
		return errors.New("recorder not enabled")
	}

	// Alive is a map of connection IDs for connections that are alive
	// to an index into the entries slice.
	alive := make(map[uint64]int)
	entries := make([]Value, 0, len(r.entries))
	for _, value := range r.entries {
		if value.Server != server {
			entries = append(entries, value)
		} else if value.Status == Alive {
			pos := len(entries)
			entries = append(entries, value)
			alive[value.ConnectionID] = pos
		}
	}

	for _, value := range connections {
		if value.Server != server {
			return errors.Errorf("connection server mismatch, got %q expected %q", value.Server, server)
		}
		// If the connection has already been recorded as alive,
		// just update the timestamp, otherwise add it in.
		if i, found := alive[value.ConnectionID]; found {
			entries[i].LastSeen = r.clock.Now()
		} else {
			value.Status = Alive
			value.LastSeen = r.clock.Now()
			entries = append(entries, value)
		}
	}

	r.entries = entries
	return nil
}

// Connections implements Recorder.
func (r *recorder) Connections() Connections {
	r.mu.Lock()
	defer r.mu.Unlock()

	entries := make([]Value, len(r.entries))
	copy(entries, r.entries)
	return &connections{values: entries}
}

// ForModel implements Connections.
func (c *connections) ForModel(model string) Connections {
	var values []Value
	for _, value := range c.values {
		if value.Model == model {
			values = append(values, value)
		}
	}
	return &connections{model: model, values: values}
}

// ForServer implements Connections.
func (c *connections) ForServer(server string) Connections {
	var values []Value
	for _, value := range c.values {
		if value.Server == server {
			values = append(values, value)
		}
	}
	return &connections{model: c.model, values: values}
}

// ForAgent implements Connections.
func (c *connections) ForAgent(agent string) Connections {
	var values []Value
	for _, value := range c.values {
		if value.Agent == agent {
			values = append(values, value)
		}
	}
	return &connections{model: c.model, values: values}
}

// Count implements Connections.
func (c *connections) Count() int {
	return len(c.values)
}

// Models implements Connections.
func (c *connections) Models() []string {
	models := set.NewStrings()
	for _, value := range c.values {
		models.Add(value.Model)
	}
	return models.Values()
}

// Servers implements Connections.
func (c *connections) Servers() []string {
	servers := set.NewStrings()
	for _, value := range c.values {
		servers.Add(value.Server)
	}
	return servers.Values()
}

// Agents implements Connections.
func (c *connections) Agents() []string {
	agents := set.NewStrings()
	for _, value := range c.values {
		agents.Add(value.Agent)
	}
	return agents.Values()
}

// AgentStatus implements Connections.
func (c *connections) AgentStatus(agent string) (Status, error) {
	if c.model == "" {
		return Unknown, errors.New("connections not limited to a model, agent ambiguous")
	}
	result := Unknown
	for _, value := range c.values {
		if value.Agent == agent && !value.ControllerAgent {
			if value.Status > result {
				result = value.Status
			}
		}
	}
	return result, nil
}

// Values implements Connections.
func (c *connections) Values() []Value {
	return c.values
}
