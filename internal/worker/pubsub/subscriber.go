// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package pubsub

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/pubsub/v2"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"

	"github.com/juju/juju/api"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/pubsub/apiserver"
	"github.com/juju/juju/rpc/params"
)

// WorkerConfig defines the configuration values that the pubsub worker needs
// to operate.
type WorkerConfig struct {
	Origin string
	Clock  clock.Clock
	Hub    *pubsub.StructuredHub
	Logger logger.Logger

	APIInfo   *api.Info
	NewWriter func(context.Context, *api.Info) (MessageWriter, error)
	NewRemote func(RemoteServerConfig) (RemoteServer, error)
}

// Validate checks that all the values have been set.
func (c *WorkerConfig) Validate() error {
	if c.Origin == "" {
		return errors.NotValidf("missing origin")
	}
	if c.Clock == nil {
		return errors.NotValidf("missing clock")
	}
	if c.Hub == nil {
		return errors.NotValidf("missing hub")
	}
	if c.Logger == nil {
		return errors.NotValidf("missing logger")
	}
	if c.APIInfo == nil {
		return errors.NotValidf("missing api info")
	}
	if c.NewWriter == nil {
		return errors.NotValidf("missing new writer")
	}
	if c.NewRemote == nil {
		return errors.NotValidf("missing new remote")
	}
	return nil
}

// NewWorker exposes the subscriber as a Worker.
func NewWorker(config WorkerConfig) (worker.Worker, error) {
	return newSubscriber(config)
}

type subscriber struct {
	config   WorkerConfig
	catacomb catacomb.Catacomb

	unsubAll           func()
	unsubServerDetails func()

	// servers represent connections to each of the other api servers.
	servers map[string]RemoteServer
	mutex   sync.Mutex
}

func newSubscriber(config WorkerConfig) (*subscriber, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	sub := &subscriber{
		config:  config,
		servers: make(map[string]RemoteServer),
	}
	unsub, err := config.Hub.SubscribeMatch(pubsub.MatchAll, sub.forwardMessage)
	if err != nil {
		return nil, errors.Trace(err)
	}
	sub.unsubAll = unsub
	config.Logger.Debugf(context.Background(), "subscribing to details topic")
	unsub, err = config.Hub.Subscribe(apiserver.DetailsTopic, sub.apiServerChanges)
	if err != nil {
		return nil, errors.Trace(err)
	}
	sub.unsubServerDetails = unsub

	// Ask for the current server details now that we're subscribed.
	detailsRequest := apiserver.DetailsRequest{
		Requester: "pubsub-forwarder",
		LocalOnly: true,
	}
	if _, err := config.Hub.Publish(apiserver.DetailsRequestTopic, detailsRequest); err != nil {
		return nil, errors.Trace(err)
	}

	err = catacomb.Invoke(catacomb.Plan{
		Name: "pubsub-subscriber",
		Site: &sub.catacomb,
		Work: sub.loop,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}

	return sub, nil
}

// Report returns the same information as the introspection report
// but in the map for the dependency engine report.
func (s *subscriber) Report() map[string]interface{} {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	result := map[string]interface{}{
		"source": s.config.Origin,
	}
	targets := make(map[string]interface{})
	for target, remote := range s.servers {
		targets[target] = remote.Report()
	}
	if len(targets) > 0 {
		result["targets"] = targets
	}
	return result

}

// IntrospectionReport is the method called by the introspection
// worker to get what to show to the user.
func (s *subscriber) IntrospectionReport() string {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	var result []string
	for target, remote := range s.servers {
		info := fmt.Sprintf("Target: %s\n%s",
			target, remote.IntrospectionReport())
		result = append(result, info)
	}
	prefix := fmt.Sprintf("Source: %s\n\n", s.config.Origin)
	// Sorting the result gives us consistent ordering.
	sort.Strings(result)
	return prefix + strings.Join(result, "\n")
}

func (s *subscriber) loop() error {
	ctx, cancel := s.scopedContext()
	defer cancel()

	s.config.Logger.Tracef(ctx, "wait for catacomb dying before unsubscribe")
	defer s.unsubAll()
	defer s.unsubServerDetails()

	<-s.catacomb.Dying()
	s.config.Logger.Tracef(ctx, "dying now")
	return s.catacomb.ErrDying()
}

func (s *subscriber) apiServerChanges(topic string, details apiserver.Details, err error) {
	ctx, cancel := s.scopedContext()
	defer cancel()

	s.config.Logger.Tracef(ctx, "apiServerChanges: %#v", details)
	// Make sure we have workers for the defined details.
	if err != nil {
		// This should never happen.
		s.config.Logger.Errorf(ctx, "subscriber callback error: %v", err)
		return
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()
	apiServers := set.NewStrings()
	for id, apiServer := range details.Servers {
		originTag, err := names.ParseTag(s.config.Origin)
		if err != nil {
			// This should never happen.
			s.config.Logger.Errorf(ctx, "subscriber origin tag error: %v", err)
			continue
		}
		// The target is constructed from an id, and the tag type
		// needs to match that of the origin tag.
		var target string
		switch originTag.Kind() {
		case names.MachineTagKind:
			target = names.NewMachineTag(id).String()
		case names.ControllerAgentTagKind:
			target = names.NewControllerAgentTag(id).String()
		default:
			// This should never happen.
			s.config.Logger.Errorf(ctx, "unknown subscriber origin tag: %v", originTag)
			continue
		}

		apiServers.Add(target)
		if target == s.config.Origin {
			// We don't need to forward messages to ourselves.
			continue
		}

		// TODO: always use the internal address?
		addresses := apiServer.Addresses
		if apiServer.InternalAddress != "" {
			addresses = []string{apiServer.InternalAddress}
		}

		server, found := s.servers[target]
		if found {
			s.config.Logger.Tracef(ctx, "update addresses for %s to %v", target, addresses)
			server.UpdateAddresses(addresses)
		} else {
			s.config.Logger.Debugf(ctx, "new forwarder for %s", target)
			newInfo := *s.config.APIInfo
			newInfo.Addrs = addresses
			server, err := s.config.NewRemote(RemoteServerConfig{
				Hub:       s.config.Hub,
				Origin:    s.config.Origin,
				Target:    target,
				Clock:     s.config.Clock,
				Logger:    s.config.Logger,
				APIInfo:   &newInfo,
				NewWriter: s.config.NewWriter,
			})
			if err != nil {
				s.config.Logger.Errorf(ctx, "unable to add new remote server for %q, %v", target, err)
				continue
			}
			s.servers[target] = server
			_ = s.catacomb.Add(server)
		}
	}
	for name, server := range s.servers {
		if !apiServers.Contains(name) {
			s.config.Logger.Debugf(ctx, "%s no longer listed as an apiserver", name)
			server.Kill()
			err := server.Wait()
			if err != nil {
				s.config.Logger.Errorf(ctx, "%v", err)
			}
			delete(s.servers, name)
		}
	}
	s.config.Logger.Tracef(ctx, "update complete")
}

func (s *subscriber) forwardMessage(topic string, data map[string]interface{}) {
	ctx, cancel := s.scopedContext()
	defer cancel()

	if data["origin"] != s.config.Origin {
		// Message does not originate from the place we care about.
		// Nothing to do.
		s.config.Logger.Tracef(ctx, "skipping message %q as origin not ours", topic)
		return
	}
	// If local-only isn't specified, then the default interface{} value is
	// returned, which is nil, and nil isn't true.
	if data["local-only"] == true {
		// Local message, don't forward.
		s.config.Logger.Tracef(ctx, "skipping message %q as local-only", topic)
		return
	}

	s.config.Logger.Tracef(ctx, "forward message %q", topic)
	message := &params.PubSubMessage{Topic: topic, Data: data}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	for _, remote := range s.servers {
		remote.Publish(message)
	}
}

// Kill is part of the worker.Worker interface.
func (s *subscriber) Kill() {
	s.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (s *subscriber) Wait() error {
	return s.catacomb.Wait()
}

func (s *subscriber) scopedContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(s.catacomb.Context(context.Background()))
}
