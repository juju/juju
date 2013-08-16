// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

import (
	"launchpad.net/juju-core/agent/tools"
)

// Entity identifies a single entity.
type Entity struct {
	Tag string
}

// Entities identifies multiple entities.
type Entities struct {
	Entities []Entity
}

// CharmURL identifies a single charm URL.
type CharmURL struct {
	URL string
}

// CharmURLs identifies multiple charm URLs.
type CharmURLs struct {
	URLs []CharmURL
}

// StringsResult holds the result of an API call that returns a slice
// of strings or an error.
type StringsResult struct {
	Error  *Error
	Result []string
}

// StringsResults holds the bulk operation result of an API call
// that returns a slice of strings or an error.
type StringsResults struct {
	Results []StringsResult
}

// StringResult holds a string or an error.
type StringResult struct {
	Error  *Error
	Result string
}

// StringResults holds the bulk operation result of an API call
// that returns a string or an error.
type StringResults struct {
	Results []StringResult
}

// StringBoolResult holds the result of an API call that returns a
// string and a boolean.
type StringBoolResult struct {
	Error  *Error
	Result string
	Ok     bool
}

// StringBoolResults holds multiple results with a string and a bool
// each.
type StringBoolResults struct {
	Results []StringBoolResult
}

// Settings holds charm config options names and values.
type Settings map[string]interface{}

// SettingsResult holds a charm settings map or an error.
type SettingsResult struct {
	Error    *Error
	Settings Settings
}

// SettingsResults holds the result of an API calls that returns
// settings for multiple entities.
type SettingsResults struct {
	Results []SettingsResult
}

// RelationUnit holds a relation and a unit tag.
type RelationUnit struct {
	Relation string
	Unit     string
}

// RelationUnits holds the parameters for API calls expecting a pair
// of relation and unit tags.
type RelationUnits struct {
	RelationUnits []RelationUnit
}

// RelationUnitPair holds a relation tag, a local and remote unit tags.
type RelationUnitPair struct {
	Relation   string
	LocalUnit  string
	RemoteUnit string
}

// RelationUnitPairs holds the parameters for API calls expecting
// multiple sets of a relation tag, a local and remote unit tags.
type RelationUnitPairs struct {
	RelationUnitPairs []RelationUnitPair
}

// RelationUnitSettings holds a relation tag, a unit tag and (optional
// for EnterScope, mandatory for WriteSettings) local unit settings.
type RelationUnitSettings struct {
	Relation string
	Unit     string
	Settings Settings
}

// RelationUnitsSettings holds the arguments for making a EnterScope
// or WriteSettings API calls.
type RelationUnitsSettings struct {
	RelationUnits []RelationUnitSettings
}

// RelationResult holds the relation id, key and the local endpoint
// for a single relation or an error.
type RelationResult struct {
	Error    *Error
	Id       int
	Key      string
	Endpoint Endpoint
}

// RelationResults holds the result of an API call that returns
// information about multiple relations.
type RelationResults struct {
	Results []RelationResult
}

// EntityPort holds an entity's tag, a protocol and a port.
type EntityPort struct {
	Tag      string
	Protocol string
	Port     int
}

// EntitiesPorts holds the parameters for making an OpenPort or
// ClosePort on some entities.
type EntitiesPorts struct {
	Entities []EntityPort
}

// EntityCharmURL holds an entity's tag and a charm URL.
type EntityCharmURL struct {
	Tag      string
	CharmURL string
}

// EntitiesCharmURL holds the parameters for making a SetCharmURL API
// call.
type EntitiesCharmURL struct {
	Entities []EntityCharmURL
}

// BytesResult holds the result of an API call that returns a slice
// of bytes.
type BytesResult struct {
	Result []byte
}

// LifeResult holds the life status of a single entity, or an error
// indicating why it is not available.
type LifeResult struct {
	Life  Life
	Error *Error
}

// LifeResults holds the life or error status of multiple entities.
type LifeResults struct {
	Results []LifeResult
}

// SetEntityAddress holds an entity tag and an address.
type SetEntityAddress struct {
	Tag     string
	Address string
}

// SetEntityAddresses holds the parameters for making a Set*Address
// call, where the address can be a public or a private one.
type SetEntityAddresses struct {
	Entities []SetEntityAddress
}

// MachineSetStatus holds a machine tag, status and extra info.
// DEPRECATE(v1.14)
type MachineSetStatus struct {
	Tag    string
	Status Status
	Info   string
}

// MachinesSetStatus holds the parameters for making a Machiner.SetStatus call.
// DEPRECATE(v1.14)
type MachinesSetStatus struct {
	Machines []MachineSetStatus
}

// SetEntityStatus holds an entity tag, status and extra info.
type SetEntityStatus struct {
	Tag    string
	Status Status
	Info   string
}

// SetStatus holds the parameters for making a SetStatus call.
type SetStatus struct {
	Entities []SetEntityStatus
	// Machines is only here to ensure compatibility with v1.12.
	// DEPRECATE(v1.14)
	Machines []SetEntityStatus
}

// MachineAgentGetMachinesResults holds the results of a
// machineagent.API.GetMachines call.
// DEPRECATE(v1.14)
type MachineAgentGetMachinesResults struct {
	Machines []MachineAgentGetMachinesResult
}

// MachineAgentGetMachinesResult holds the results of a
// machineagent.API.GetMachines call for a single machine.
// DEPRECATE(v1.14)
type MachineAgentGetMachinesResult struct {
	Life  Life
	Jobs  []MachineJob
	Error *Error
}

// AgentGetEntitiesResults holds the results of a
// agent.API.GetEntities call.
type AgentGetEntitiesResults struct {
	Entities []AgentGetEntitiesResult
}

// AgentGetEntitiesResult holds the results of a
// machineagent.API.GetEntities call for a single entity.
type AgentGetEntitiesResult struct {
	Life  Life
	Jobs  []MachineJob
	Error *Error
}

// AgentToolsResult holds the tools and possibly error for a given AgentTools request
type AgentToolsResult struct {
	Tools *tools.Tools
	Error *Error
}

// AgentToolsResults is a list of tools for various requested agents.
type AgentToolsResults struct {
	Results []AgentToolsResult
}

// SetAgent specifies tools to be set for an agent with the
// given tag.
type SetAgentTools struct {
	Tag   string
	Tools *tools.Tools
}

// SetAgentsTools specifies what tools are being run for
// multiple agents.
type SetAgentsTools struct {
	AgentTools []SetAgentTools
}

// PasswordChanges holds the parameters for making a SetPasswords call.
type PasswordChanges struct {
	Changes []PasswordChange
}

// PasswordChange specifies a password change for the entity
// with the given tag.
type PasswordChange struct {
	Tag      string
	Password string
}

// NotifyWatchResult holds a NotifyWatcher id and an error (if any).
type NotifyWatchResult struct {
	NotifyWatcherId string
	Error           *Error
}

// NotifyWatchResults holds the results for any API call which ends up
// returning a list of NotifyWatchers
type NotifyWatchResults struct {
	Results []NotifyWatchResult
}

// StringsWatchResult holds a StringsWatcher id, changes and an error
// (if any).
type StringsWatchResult struct {
	StringsWatcherId string
	Changes          []string
	Error            *Error
}

// StringsWatchResults holds the results for any API call which ends up
// returning a list of StringsWatchers.
type StringsWatchResults struct {
	Results []StringsWatchResult
}
