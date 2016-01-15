// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricsdebug

import (
	"time"
)

var (
	GetCollectMetricsAPIClient        = &getCollectMetricsAPIClient
	NewUnwrappedCollectMetricsCommand = func() *collectMetricsCommand {
		return &collectMetricsCommand{}
	}
)

// Timeout returns the command's timeout field for testing purposes.
func (c *collectMetricsCommand) Timeout() time.Duration {
	return c.timeout
}

// Services returns the command's services field for testing purposes.
func (c *collectMetricsCommand) Services() []string {
	return c.services
}

// Units returns the command's units field for testing purposes.
func (c *collectMetricsCommand) Units() []string {
	return c.units
}

func GetCollectMetricsAPIClientFunction(client CollectMetricsClient) func(*collectMetricsCommand) (CollectMetricsClient, error) {
	return func(_ *collectMetricsCommand) (CollectMetricsClient, error) {
		return client, nil
	}
}
