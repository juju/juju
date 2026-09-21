// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backupsweeper

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v5/workertest"

	corebackups "github.com/juju/juju/core/backups"
	environsconfig "github.com/juju/juju/environs/config"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/uuid"
)

type sweeperWorkerSuite struct {
	baseSuite
}

func TestSweeperWorkerSuite(t *testing.T) {
	testhelpers.PrintGoroutineLeaks(t, func(t *testing.T) {
		tc.Run(t, &sweeperWorkerSuite{})
	})
}

func (s *sweeperWorkerSuite) TestValidateConfig(c *tc.C) {
	cfg := s.getConfig(c)
	c.Check(cfg.Validate(), tc.ErrorIsNil)

	cfg = s.getConfig(c)
	cfg.ModelConfigService = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.Clock = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.Logger = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)
}

func (s *sweeperWorkerSuite) TestWorkerSweeps(c *tc.C) {
	defer s.setupMocks(c).Finish()

	backupDir := c.MkDir()
	expiredID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)
	freshID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	defaultTTL, err := time.ParseDuration(environsconfig.DefaultBackupDownloadTTL)
	c.Assert(err, tc.ErrorIsNil)
	now := time.Now()
	expiredPath := s.stageOneShot(c, backupDir, expiredID.String(), now.Add(-2*defaultTTL))
	freshPath := s.stageOneShot(c, backupDir, freshID.String(), now)

	s.expectClock()
	done := make(chan struct{})
	s.expectTimerRepeated(1, done)
	s.expectModelConfig(c, backupDir)

	w := s.newWorker(c)
	defer workertest.CleanKill(c, w)

	select {
	case <-done:
	case <-c.Context().Done():
		c.Fatal("timed out waiting for the sweep to run")
	}

	_, err = os.Stat(expiredPath)
	c.Assert(err, tc.Satisfies, os.IsNotExist)
	_, err = os.Stat(freshPath)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *sweeperWorkerSuite) TestWorkerHonoursConfiguredTTL(c *tc.C) {
	defer s.setupMocks(c).Finish()

	backupDir := c.MkDir()
	id, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	// The archive is older than the 15-minute default but younger than
	// the configured 2-hour TTL, so it must survive the sweep.
	now := time.Now()
	path := s.stageOneShot(c, backupDir, id.String(), now.Add(-30*time.Minute))

	s.expectClock()
	done := make(chan struct{})
	s.expectTimerRepeated(1, done)
	s.expectModelConfigWithTTL(c, backupDir, "2h")

	w := s.newWorker(c)
	defer workertest.CleanKill(c, w)

	select {
	case <-done:
	case <-c.Context().Done():
		c.Fatal("timed out waiting for the sweep to run")
	}

	_, err = os.Stat(path)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *sweeperWorkerSuite) TestWorkerSurvivesSweepFailure(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()
	done := make(chan struct{})
	s.expectTimerRepeated(1, done)

	// A config resolution failure must not kill the worker; it is logged
	// and retried on the next tick.
	s.modelConfig.EXPECT().ModelConfig(gomock.Any()).Return(nil, errors.New("boom"))

	w := s.newWorker(c)
	defer workertest.CleanKill(c, w)

	select {
	case <-done:
	case <-c.Context().Done():
		c.Fatal("timed out waiting for the sweep to run")
	}
	workertest.CheckAlive(c, w)
}

// stageOneShot writes a one-shot archive for the given id with the given
// modification time and returns its path.
func (s *sweeperWorkerSuite) stageOneShot(c *tc.C, backupDir, id string, modTime time.Time) string {
	path, err := corebackups.OneShotArchivePath(backupDir, id)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(os.MkdirAll(filepath.Dir(path), 0755), tc.ErrorIsNil)
	c.Assert(os.WriteFile(path, []byte("archive data"), 0600), tc.ErrorIsNil)
	c.Assert(os.Chtimes(path, modTime, modTime), tc.ErrorIsNil)
	return path
}

func (s *sweeperWorkerSuite) expectModelConfig(c *tc.C, backupDir string) {
	s.expectModelConfigWithTTL(c, backupDir, "")
}

func (s *sweeperWorkerSuite) expectModelConfigWithTTL(c *tc.C, backupDir, ttl string) {
	attrs := map[string]any{
		"backup-dir": backupDir,
		"uuid":       "deadbeef-0bad-400d-8000-4b1d0d06f00d",
		"type":       "manual",
		"name":       "controller",
	}
	if ttl != "" {
		attrs["backup-download-ttl"] = ttl
	}
	cfg, err := environsconfig.New(environsconfig.UseDefaults, attrs)
	c.Assert(err, tc.ErrorIsNil)
	s.modelConfig.EXPECT().ModelConfig(gomock.Any()).Return(cfg, nil).AnyTimes()
}

func (s *sweeperWorkerSuite) getConfig(c *tc.C) WorkerConfig {
	return WorkerConfig{
		ModelConfigService: s.modelConfig,
		Clock:              s.clock,
		Logger:             loggertesting.WrapCheckLog(c),
	}
}

func (s *sweeperWorkerSuite) newWorker(c *tc.C) *Sweeper {
	w, err := NewWorker(s.getConfig(c))
	c.Assert(err, tc.ErrorIsNil)
	return w.(*Sweeper)
}
