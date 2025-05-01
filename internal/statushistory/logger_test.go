// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package statushistory

import (
	"context"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gomock "go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/status"
)

type loggerSuite struct {
	testing.IsolationSuite

	logger *MockLogger
}

var _ = gc.Suite(&loggerSuite{})

func (s *loggerSuite) TestRecord(c *gc.C) {
	defer s.setupMocks(c).Finish()

	labels := logger.Labels{
		categoryKey:    statusHistoryCategory,
		kindKey:        status.KindApplication.String(),
		namespaceIDKey: "123",
		statusKey:      "active",
		messageKey:     "foo",
		sinceKey:       "2019-01-01T00:00:00Z",
		dataKey:        `{"bar":"baz"}`,
	}

	s.logger.EXPECT().Child("status-history", logger.STATUS_HISTORY).Return(s.logger)
	s.logger.EXPECT().Logf(gomock.Any(), logger.INFO, labels, "status-history (status: %q, status-message: %q)", "active", "foo")

	logger := NewLogRecorder(s.logger)
	err := logger.Record(context.Background(), Record{
		Kind:    status.KindApplication,
		ID:      "123",
		Status:  "active",
		Message: "foo",
		Time:    "2019-01-01T00:00:00Z",
		Data: map[string]interface{}{
			"bar": "baz",
		},
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *loggerSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.logger = NewMockLogger(ctrl)

	return ctrl
}
