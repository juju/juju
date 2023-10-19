// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets_test

import (
	"context"
	"time"

	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common/secrets"
	"github.com/juju/juju/apiserver/common/secrets/mocks"
	"github.com/juju/juju/environs/config"
	coretesting "github.com/juju/juju/testing"
)

type watcherSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&watcherSuite{})

func (s *watcherSuite) TestSecretBackendModelConfigWatcher(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	modelConfigChangesWatcher := mocks.NewMockNotifyWatcher(ctrl)
	model := mocks.NewMockModel(ctrl)

	ch := make(chan struct{}, 3)
	done := make(chan struct{})
	receiverReady := make(chan struct{})
	defer close(receiverReady)
	go func() {
		for {
			_, ok := <-receiverReady
			if !ok {
				return
			}
			ch <- struct{}{}
		}
	}()
	receiverReady <- struct{}{}

	modelConfigChangesWatcher.EXPECT().Wait().Return(nil)
	modelConfigChangesWatcher.EXPECT().Changes().Return(ch).AnyTimes()

	gomock.InOrder(
		model.EXPECT().ModelConfig(gomock.Any()).DoAndReturn(
			// Initail call to get the current secret backend.
			func(_ context.Context) (*config.Config, error) {
				configAttrs := map[string]interface{}{
					"name":           "some-name",
					"type":           "some-type",
					"uuid":           coretesting.ModelTag.Id(),
					"secret-backend": "backend-id",
				}
				cfg, err := config.New(config.NoDefaults, configAttrs)
				c.Assert(err, jc.ErrorIsNil)
				return cfg, nil
			},
		),
		model.EXPECT().ModelConfig(gomock.Any()).DoAndReturn(
			// Call to get the current secret backend after the first change(no change, but we always send the initial event).
			func(_ context.Context) (*config.Config, error) {
				configAttrs := map[string]interface{}{
					"name":           "some-name",
					"type":           "some-type",
					"uuid":           coretesting.ModelTag.Id(),
					"secret-backend": "backend-id",
				}
				cfg, err := config.New(config.NoDefaults, configAttrs)
				c.Assert(err, jc.ErrorIsNil)
				return cfg, nil
			},
		),
		model.EXPECT().ModelConfig(gomock.Any()).DoAndReturn(
			// Call to get the current secret backend after the first change(no change, we won'ts send the event).
			func(_ context.Context) (*config.Config, error) {
				configAttrs := map[string]interface{}{
					"name":           "some-name",
					"type":           "some-type",
					"uuid":           coretesting.ModelTag.Id(),
					"secret-backend": "backend-id",
				}
				cfg, err := config.New(config.NoDefaults, configAttrs)
				c.Assert(err, jc.ErrorIsNil)
				return cfg, nil
			},
		),
		model.EXPECT().ModelConfig(gomock.Any()).DoAndReturn(
			// Call to get the current secret backend after the second change - backend changed.
			func(_ context.Context) (*config.Config, error) {
				configAttrs := map[string]interface{}{
					"name":           "some-name",
					"type":           "some-type",
					"uuid":           coretesting.ModelTag.Id(),
					"secret-backend": "a-different-backend-id",
				}
				cfg, err := config.New(config.NoDefaults, configAttrs)
				c.Assert(err, jc.ErrorIsNil)
				close(done)
				return cfg, nil
			},
		),
	)

	w, err := secrets.NewSecretBackendModelConfigWatcher(context.Background(), s.model, s.modelConfigChangesWatcher, loggo.GetLogger("juju.apiserver.secretsdrain"))
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(c *gc.C) { workertest.DirtyKill(c, w) })

	received := 0
ensureReceived:
	for a := coretesting.LongAttempt.Start(); a.Next(); {
		select {
		case _, ok := <-w.Changes():
			if !ok {
				break ensureReceived
			}
			received++
		case <-time.After(coretesting.ShortWait):
		}

		if received == 2 {
			return
		}

		select {
		case receiverReady <- struct{}{}:
		case <-done:
			break ensureReceived
		case <-time.After(coretesting.ShortWait):
		}

	}
	c.Fatalf("expected 2 events, got %d", received)
}
