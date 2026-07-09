// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controllerlokiupdater

import (
	"context"
	stdtesting "testing"

	"github.com/juju/tc"
	"github.com/juju/utils/v4/voyeur"

	corewatcher "github.com/juju/juju/core/watcher"
	"github.com/juju/juju/domain/logging"
	internallogger "github.com/juju/juju/internal/logger"
)

type workerSuite struct{}

func TestWorkerSuite(t *stdtesting.T) {
	tc.Run(t, &workerSuite{})
}

func (s *workerSuite) TestSyncConfigSkipsDuplicateConfig(c *tc.C) {
	service := &stubLokiConfigService{}
	var writes []logging.LokiConfig
	runtimeConfigChanged := voyeur.NewValue(false)

	service.getLokiConfig = func(context.Context) (logging.LokiConfig, error) {
		insecure := true
		return logging.LokiConfig{
			Endpoint:           "https://loki.example.com/loki/api/v1/push",
			CACertificate:      "ca-cert",
			InsecureSkipVerify: &insecure,
			OrgID:              "org-one",
		}, nil
	}

	w := &lokiConfigUpdater{config: Config{
		LokiConfigService: service,
		WriteLokiConfig: func(cfg logging.LokiConfig) error {
			writes = append(writes, cfg)
			return nil
		},
		RuntimeConfigChanged: runtimeConfigChanged,
		Logger:               internallogger.GetLogger("juju.worker.controllerlokiupdater.test"),
	}}

	// First sync: config should be written and voyeur set.
	watcher := runtimeConfigChanged.Watch()
	defer watcher.Close()

	err := w.syncConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(writes, tc.HasLen, 1)
	c.Check(watcher.Next(), tc.IsTrue)

	// Second sync with same config: no write, no voyeur update.
	err = w.syncConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(writes, tc.HasLen, 1)

	// Third sync with different OrgID: new write and voyeur update.
	service.getLokiConfig = func(context.Context) (logging.LokiConfig, error) {
		insecure := true
		return logging.LokiConfig{
			Endpoint:           "https://loki.example.com/loki/api/v1/push",
			CACertificate:      "ca-cert",
			InsecureSkipVerify: &insecure,
			OrgID:              "org-two",
		}, nil
	}

	err = w.syncConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(writes, tc.HasLen, 2)
	c.Check(watcher.Next(), tc.IsTrue)
	c.Check(writes[1].OrgID, tc.Equals, "org-two")
}

type stubLokiConfigService struct {
	getLokiConfig func(context.Context) (logging.LokiConfig, error)
}

func (s *stubLokiConfigService) GetLokiConfig(ctx context.Context) (logging.LokiConfig, error) {
	return s.getLokiConfig(ctx)
}

func (*stubLokiConfigService) WatchLokiConfig(context.Context) (corewatcher.NotifyWatcher, error) {
	return nil, nil
}
