// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package presence

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/pubsub"
	"github.com/juju/worker/v2"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/presence"
	"github.com/juju/juju/pubsub/apiserver"
	"github.com/juju/juju/pubsub/forwarder"
)

// WorkerConfig defines the configuration values that the pubsub worker needs
// to operate.
type WorkerConfig struct {
	Origin   string
	Hub      *pubsub.StructuredHub
	Recorder presence.Recorder
	Logger   Logger
}

// Validate ensures that the required values are set in the structure.
func (c *WorkerConfig) Validate() error {
	if c.Origin == "" {
		return errors.NotValidf("missing origin")
	}
	if c.Hub == nil {
		return errors.NotValidf("missing hub")
	}
	if c.Recorder == nil {
		return errors.NotValidf("missing recorder")
	}
	if c.Logger == nil {
		return errors.NotValidf("missing logger")
	}
	return nil
}

// NewWorker creates a new presence worker that responds to pubsub connection
// messages.
func NewWorker(config WorkerConfig) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	// Don't return from NewWorker until the loop has started and
	// has subscribed to everything.
	started := make(chan struct{})
	w := &wrapper{
		origin:   config.Origin,
		hub:      config.Hub,
		recorder: config.Recorder,
		logger:   config.Logger,
	}
	w.tomb.Go(func() error {
		return w.loop(started)
	})
	select {
	case <-started:
	case <-time.After(10 * time.Second):
		return nil, errors.New("worker failed to start properly")
	}
	return w, nil
}

type wrapper struct {
	tomb     tomb.Tomb
	origin   string
	hub      *pubsub.StructuredHub
	recorder presence.Recorder
	logger   Logger
}

// Report implements worker.Report.
func (w *wrapper) Report() map[string]interface{} {
	all := w.recorder.Connections()
	result := make(map[string]interface{})
	servers := all.Servers()
	for _, name := range servers {
		conns := all.ForServer(name)
		result[name] = conns.Count()
	}
	return result
}

func (w *wrapper) loop(started chan struct{}) error {
	multiplexer, err := w.hub.NewMultiplexer()
	if err != nil {
		return errors.Trace(err)
	}
	defer multiplexer.Unsubscribe()

	if err := multiplexer.Add(forwarder.ConnectedTopic, w.forwarderConnect); err != nil {
		return errors.Trace(err)
	}
	if err := multiplexer.Add(forwarder.DisconnectedTopic, w.forwarderDisconnect); err != nil {
		return errors.Trace(err)
	}
	if err := multiplexer.Add(apiserver.ConnectTopic, w.agentLogin); err != nil {
		return errors.Trace(err)
	}
	if err := multiplexer.Add(apiserver.DisconnectTopic, w.agentDisconnect); err != nil {
		return errors.Trace(err)
	}
	if err := multiplexer.Add(apiserver.PresenceRequestTopic, w.presenceRequest); err != nil {
		return errors.Trace(err)
	}
	if err := multiplexer.Add(apiserver.PresenceResponseTopic, w.presenceResponse); err != nil {
		return errors.Trace(err)
	}
	// Let the caller know we are done.
	close(started)
	// Don't exit until we are told to. Exiting unsubscribes.
	<-w.tomb.Dying()
	w.logger.Tracef("presence loop finished")
	return nil
}

func (w *wrapper) forwarderConnect(topic string, data forwarder.OriginTarget, err error) {
	if err != nil {
		w.logger.Errorf("forwarderConnect error %v", err)
		return
	}

	// If we have just set up forwarding to another server, or another server
	// has just set up forwarding to us, ask for their presence info.
	w.logger.Tracef("forwarding connection up for %s -> %s", data.Origin, data.Target)
	var request string
	switch {
	case data.Origin == w.origin:
		request = data.Target
	case data.Target == w.origin:
		request = data.Origin
	default:
		return
	}
	w.logger.Tracef("request presence info from %s", request)
	msg := apiserver.OriginTarget{Target: request}
	w.hub.Publish(apiserver.PresenceRequestTopic, msg)
	w.logger.Tracef("request sent")
}

func (w *wrapper) forwarderDisconnect(topic string, data forwarder.OriginTarget, err error) {
	if err != nil {
		w.logger.Errorf("forwarderChange error %v", err)
		return
	}
	// If we have lost connectivity to the target, we mark the server down.
	// Ideally this would be when the target is no longer forwarding us messages,
	// but we aren't guaranteed to get those messages, so we use the lack of our
	// connectivity to the other machine to indicate that comms are down.
	if data.Origin == w.origin {
		w.logger.Tracef("forwarding connection down for %s", data.Target)
		w.recorder.ServerDown(data.Target)
	}
}

func (w *wrapper) agentLogin(topic string, data apiserver.APIConnection, err error) {
	if err != nil {
		w.logger.Errorf("agentLogin error %v", err)
		return
	}
	if w.logger.IsTraceEnabled() {
		agentName := data.AgentTag
		if data.ControllerAgent {
			agentName += " (controller)"
		}
		w.logger.Tracef("api connect %s:%s -> %s (%v)", data.ModelUUID, agentName, data.Origin, data.ConnectionID)
	}
	w.recorder.Connect(data.Origin, data.ModelUUID, data.AgentTag, data.ConnectionID, data.ControllerAgent, data.UserData)
}

func (w *wrapper) agentDisconnect(topic string, data apiserver.APIConnection, err error) {
	if err != nil {
		w.logger.Errorf("agentDisconnect error %v", err)
		return
	}
	w.logger.Tracef("api disconnect %s (%v)", data.Origin, data.ConnectionID)
	w.recorder.Disconnect(data.Origin, data.ConnectionID)
}

func (w *wrapper) presenceRequest(topic string, data apiserver.OriginTarget, err error) {
	if err != nil {
		w.logger.Errorf("connectionChange error %v", err)
		return
	}
	// If the message is not meant for us, ignore.
	if data.Target != w.origin {
		w.logger.Tracef("presence request for %s ignored, as we are %s", data.Target, w.origin)
		return
	}

	w.logger.Tracef("presence request from %s", data.Origin)

	connections := w.recorder.Connections().ForServer(w.origin)
	values := connections.Values()
	response := apiserver.PresenceResponse{
		Connections: make([]apiserver.APIConnection, len(values)),
	}
	for i, value := range values {
		if value.Status != presence.Alive {
			w.logger.Infof("presence response has weird status: %#v", value)
		}
		response.Connections[i] = apiserver.APIConnection{
			AgentTag:        value.Agent,
			ControllerAgent: value.ControllerAgent,
			ModelUUID:       value.Model,
			ConnectionID:    value.ConnectionID,
			Origin:          value.Server,
			UserData:        value.UserData,
		}
	}
	_, err = w.hub.Publish(apiserver.PresenceResponseTopic, response)
	if err != nil {
		w.logger.Errorf("cannot send presence response: %v", err)
	}
}

func (w *wrapper) presenceResponse(topic string, data apiserver.PresenceResponse, err error) {
	if err != nil {
		w.logger.Errorf("connectionChange error %v", err)
		return
	}
	// If this message is from us, ignore it.
	if data.Origin == w.origin {
		w.logger.Tracef("ignoring our own presence response message")
		return
	}

	// Build up a slice of presence values so we can transactionally
	// update the recorder.
	values := make([]presence.Value, 0, len(data.Connections))
	for _, conn := range data.Connections {
		if w.logger.IsTraceEnabled() {
			agentName := conn.AgentTag
			if conn.ControllerAgent {
				agentName += " (controller)"
			}
			w.logger.Tracef("setting presence %s:%s -> %s (%v)", conn.ModelUUID, agentName, conn.Origin, conn.ConnectionID)
		}
		values = append(values, presence.Value{
			Model:           conn.ModelUUID,
			Server:          conn.Origin,
			Agent:           conn.AgentTag,
			ConnectionID:    conn.ConnectionID,
			ControllerAgent: conn.ControllerAgent,
			UserData:        conn.UserData,
		})
	}

	err = w.recorder.UpdateServer(data.Origin, values)
	// An error here is only if the values don't come from the server.
	// This would be a programming error, and as such, we just log it.
	if err != nil {
		w.logger.Errorf("UpdateServer error %v", err)
	}
}

// Kill implements Worker.Kill().
func (w *wrapper) Kill() {
	w.tomb.Kill(nil)
}

// Wait implements Worker.Wait().
func (w *wrapper) Wait() error {
	return w.tomb.Wait()
}
