// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import "github.com/juju/juju/v2/pubsub/common"

// DetailsTopic is the topic name for the published message when the details
// of the api servers change. This message is normally published by the
// peergrouper when the set of API servers changes.
const DetailsTopic = "apiserver.details"

// APIServer contains the machine id and addresses of a single API server machine.
type APIServer struct {
	// ID contains the Juju machine ID for the apiserver.
	ID string `yaml:"id"`

	// Addresses contains all of the usable addresses of the
	// apiserver machine.
	Addresses []string `yaml:"addresses"`

	// InternalAddress, if non-empty, is the address that
	// other API servers should use to connect to this API
	// server, in the form addr:port.
	//
	// This may be empty if the API server is not fully
	// initialised.
	InternalAddress string `yaml:"internal-address,omitempty"`
}

// Details contains the ids and addresses of all the current API server
// machines.
type Details struct {
	// Servers is a map of machine ID to the details for that server.
	Servers   map[string]APIServer `yaml:"servers"`
	LocalOnly bool                 `yaml:"local-only"`
}

// DetailsRequestTopic is the topic that details requests are
// published on. The peergrouper responds those requests, publishing
// the current details on the DetailsTopic.
const DetailsRequestTopic = "apiserver.details-request"

// DetailsRequest indicates the worker who is asking for the details
// to be sent. It should always be LocalOnly - we only want to ask our
// local PeerGrouper for details.
type DetailsRequest struct {
	Requester string `yaml:"requester"`
	LocalOnly bool   `yaml:"local-only"`
}

// ConnectTopic is the topic name for the published message
// whenever an agent conntects to the API server.
// data: `APIConnection`
const ConnectTopic = "apiserver.agent-connect"

// DisconnectTopic is the topic name for the published message
// whenever an agent disconntects to the API server.
// data: `APIConnection`
const DisconnectTopic = "apiserver.agent-disconnect"

// APIConnection holds all the salient pieces of information that are
// available when an agent connects to the API server.
type APIConnection struct {
	AgentTag        string `yaml:"agent-tag"`
	ControllerAgent bool   `yaml:"controller-agent,omitempty"`
	ModelUUID       string `yaml:"model-uuid"`
	ConnectionID    uint64 `yaml:"connection-id"`
	Origin          string `yaml:"origin"`
	UserData        string `yaml:"user-data,omitempty"`
}

// PresenceRequestTopic is used by the presence worker to ask another HA server
// to report its connections.
// data: `OriginTarget`
const PresenceRequestTopic = "presence.request"

// PresenceResponseTopic is used by the presence worker to respond to the
// request topic above.
// data: `PresenceResponse`
const PresenceResponseTopic = "presence.response"

// PresenceResponse contains all of the current connections for the server
// identified by Origin.
type PresenceResponse struct {
	Origin      string          `yaml:"origin"`
	Connections []APIConnection `yaml:"connections"`
}

// OriginTarget represents the data for the connect and disconnect
// topics.
type OriginTarget common.OriginTarget

// RestartTopic is used by the API server to listen for events that should
// cause the API server to be bounced.
const RestartTopic = "apiserver.restart"

// Restart message only contains the local-only indicator as the restart
// is only ever for the same agent.
type Restart common.LocalOnly
