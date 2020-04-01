// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricobserver_test

import (
	"testing"

	"github.com/golang/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/observer/metricobserver/mocks"
)

func Test(t *testing.T) {
	gc.TestingT(t)
}

func createMockMetrics(c *gc.C, labels interface{}) (*mocks.MockMetricsCollector, func()) {
	ctrl := gomock.NewController(c)

	summary := mocks.NewMockSummary(ctrl)
	summary.EXPECT().Observe(gomock.Any()).AnyTimes()

	summaryVec := mocks.NewMockSummaryVec(ctrl)
	summaryVec.EXPECT().With(labels).Return(summary).AnyTimes()

	metricsCollector := mocks.NewMockMetricsCollector(ctrl)
	metricsCollector.EXPECT().APIRequestDuration().Return(summaryVec).AnyTimes()

	return metricsCollector, ctrl.Finish
}
