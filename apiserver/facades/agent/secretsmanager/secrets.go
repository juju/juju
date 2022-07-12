// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsmanager

import (
	"time"

	"gopkg.in/tomb.v2"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/rpc/params"
)

// SecretsManagerAPI is the implementation for the SecretsManager facade.
// This is only a stub implementation to keep older unit agents happy.
type SecretsManagerAPI struct {
	controllerUUID string
	modelUUID      string
	resources      facade.Resources
}

type SecretRotationChange struct {
	ID             int           `json:"secret-id"`
	URL            string        `json:"url"`
	RotateInterval time.Duration `json:"rotate-interval"`
	LastRotateTime time.Time     `json:"last-rotate-time"`
}

type SecretRotationWatchResult struct {
	SecretRotationWatcherId string                 `json:"watcher-id"`
	Changes                 []SecretRotationChange `json:"changes"`
	Error                   *params.Error          `json:"error,omitempty"`
}

type SecretRotationWatchResults struct {
	Results []SecretRotationWatchResult `json:"results"`
}

type SecretRotationChannel <-chan []SecretRotationChange

type noopSecretsWatcher struct {
	tomb tomb.Tomb
}

func (w *noopSecretsWatcher) Changes() SecretRotationChannel {
	return nil
}

func (w *noopSecretsWatcher) Stop() error {
	w.Kill()
	return w.Wait()
}

func (w *noopSecretsWatcher) Kill() {
	w.tomb.Kill(nil)
}

func (w *noopSecretsWatcher) Err() error {
	return w.tomb.Err()
}

func (w *noopSecretsWatcher) Wait() error {
	return w.tomb.Wait()
}

func (s *SecretsManagerAPI) WatchSecretsRotationChanges(args params.Entities) (SecretRotationWatchResults, error) {
	results := SecretRotationWatchResults{
		Results: make([]SecretRotationWatchResult, len(args.Entities)),
	}
	for i := range args.Entities {
		results.Results[i].SecretRotationWatcherId = s.resources.Register(&noopSecretsWatcher{})
	}
	return results, nil
}
