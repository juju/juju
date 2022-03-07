// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agentconfigupdater

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/pubsub/v2"
	"github.com/juju/worker/v3"
	"gopkg.in/tomb.v2"

	coreagent "github.com/juju/juju/agent"
	"github.com/juju/juju/mongo"
	controllermsg "github.com/juju/juju/pubsub/controller"
	jworker "github.com/juju/juju/worker"
)

// WorkerConfig contains the information necessary to run
// the agent config updater worker.
type WorkerConfig struct {
	Agent                    coreagent.Agent
	Hub                      *pubsub.StructuredHub
	MongoProfile             mongo.MemoryProfile
	JujuDBSnapChannel        string
	NonSyncedWritesToRaftLog bool
	BatchRaftFSM             bool
	Logger                   Logger
}

// Validate ensures that the required values are set in the structure.
func (c *WorkerConfig) Validate() error {
	if c.Agent == nil {
		return errors.NotValidf("missing agent")
	}
	if c.Hub == nil {
		return errors.NotValidf("missing hub")
	}
	if c.Logger == nil {
		return errors.NotValidf("missing logger")
	}
	return nil
}

type agentConfigUpdater struct {
	config WorkerConfig

	tomb                     tomb.Tomb
	mongoProfile             mongo.MemoryProfile
	jujuDBSnapChannel        string
	nonSyncedWritesToRaftLog bool
	batchRaftFSM             bool
}

// NewWorker creates a new agent config updater worker.
func NewWorker(config WorkerConfig) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	started := make(chan struct{})
	w := &agentConfigUpdater{
		config:                   config,
		mongoProfile:             config.MongoProfile,
		jujuDBSnapChannel:        config.JujuDBSnapChannel,
		nonSyncedWritesToRaftLog: config.NonSyncedWritesToRaftLog,
		batchRaftFSM:             config.BatchRaftFSM,
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

func (w *agentConfigUpdater) loop(started chan struct{}) error {
	unsubscribe, err := w.config.Hub.Subscribe(controllermsg.ConfigChanged, w.onConfigChanged)
	if err != nil {
		w.config.Logger.Criticalf("programming error in subscribe function: %v", err)
		return errors.Trace(err)
	}
	defer unsubscribe()
	// Let the caller know we are done.
	close(started)
	// Don't exit until we are told to. Exiting unsubscribes.
	<-w.tomb.Dying()
	w.config.Logger.Tracef("agentConfigUpdater loop finished")
	return nil
}

func (w *agentConfigUpdater) onConfigChanged(topic string, data controllermsg.ConfigChangedMessage, err error) {
	if err != nil {
		w.config.Logger.Criticalf("programming error in %s message data: %v", topic, err)
		return
	}

	mongoProfile := mongo.MemoryProfile(data.Config.MongoMemoryProfile())
	mongoProfileChanged := mongoProfile != w.mongoProfile

	jujuDBSnapChannel := data.Config.JujuDBSnapChannel()
	jujuDBSnapChannelChanged := jujuDBSnapChannel != w.jujuDBSnapChannel

	nonSyncedWritesToRaftLog := data.Config.NonSyncedWritesToRaftLog()
	nonSyncedWritesToRaftLogChanged := nonSyncedWritesToRaftLog != w.nonSyncedWritesToRaftLog

	batchRaftFSM := data.Config.BatchRaftFSM()
	batchRaftFSMChanged := batchRaftFSM != w.batchRaftFSM

	changeDetected := mongoProfileChanged || jujuDBSnapChannelChanged || nonSyncedWritesToRaftLogChanged || batchRaftFSMChanged
	if !changeDetected {
		// Nothing to do, all good.
		return
	}

	err = w.config.Agent.ChangeConfig(func(setter coreagent.ConfigSetter) error {
		if mongoProfileChanged {
			w.config.Logger.Debugf("setting agent config mongo memory profile: %q => %q", w.mongoProfile, mongoProfile)
			setter.SetMongoMemoryProfile(mongoProfile)
		}
		if jujuDBSnapChannelChanged {
			w.config.Logger.Debugf("setting agent config mongo snap channel: %q => %q", w.jujuDBSnapChannel, jujuDBSnapChannel)
			setter.SetJujuDBSnapChannel(jujuDBSnapChannel)
		}
		if nonSyncedWritesToRaftLogChanged {
			w.config.Logger.Debugf("setting no sync writes to raft log: %t => %t", w.nonSyncedWritesToRaftLog, nonSyncedWritesToRaftLog)
			setter.SetNonSyncedWritesToRaftLog(nonSyncedWritesToRaftLog)
		}
		if batchRaftFSMChanged {
			w.config.Logger.Debugf("setting batch raft fsm: %t => %t", w.batchRaftFSM, batchRaftFSM)
			setter.SetBatchRaftFSM(batchRaftFSM)
		}
		return nil
	})
	if err != nil {
		w.tomb.Kill(errors.Annotate(err, "failed to update agent config"))
		return
	}

	w.tomb.Kill(jworker.ErrRestartAgent)
}

// Kill implements Worker.Kill().
func (w *agentConfigUpdater) Kill() {
	w.tomb.Kill(nil)
}

// Wait implements Worker.Wait().
func (w *agentConfigUpdater) Wait() error {
	return w.tomb.Wait()
}
