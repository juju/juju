// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricobserver_test

import (
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/prometheus/client_golang/prometheus"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/observer/metricobserver/mocks"
)

func Test(t *testing.T) {
	gc.TestingT(t)
}

func createMockMetrics(c *gc.C) (*mocks.MockMetricsCollector, func()) {
	metricsCollector, finish := createMockMetricsWith(c, func(ctrl *gomock.Controller, counterVec *mocks.MockCounterVec, summaryVec *mocks.MockSummaryVec) {
		counter := mocks.NewMockCounter(ctrl)
		counter.EXPECT().Inc().AnyTimes()

		counterVec.EXPECT().With(gomock.AssignableToTypeOf(prometheus.Labels{})).Return(counter).AnyTimes()

		summary := mocks.NewMockSummary(ctrl)
		summary.EXPECT().Observe(gomock.Any()).AnyTimes()

		summaryVec.EXPECT().With(gomock.AssignableToTypeOf(prometheus.Labels{})).Return(summary).AnyTimes()
	})

	return metricsCollector, finish
}

func createMockMetricsWith(c *gc.C, fn func(ctrl *gomock.Controller, counter *mocks.MockCounterVec, summary *mocks.MockSummaryVec)) (*mocks.MockMetricsCollector, func()) {
	ctrl := gomock.NewController(c)

	counterVec := mocks.NewMockCounterVec(ctrl)

	summaryVec := mocks.NewMockSummaryVec(ctrl)

	metricsCollector := mocks.NewMockMetricsCollector(ctrl)
	metricsCollector.EXPECT().APIRequestDuration().Return(summaryVec).AnyTimes()

	metricsCollector.EXPECT().DeprecatedAPIRequestsTotal().Return(counterVec).AnyTimes()
	metricsCollector.EXPECT().DeprecatedAPIRequestDuration().Return(summaryVec).AnyTimes()

	fn(ctrl, counterVec, summaryVec)

	return metricsCollector, ctrl.Finish
}
