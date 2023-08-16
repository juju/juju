// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricsender

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"

	"github.com/juju/errors"
	wireformat "github.com/juju/romulus/wireformat/metrics"
)

// HTTPSender is the default used for sending
// metrics to the collector service.
type HTTPSender struct {
	url string
}

// Send sends the given metrics to the collector service.
func (s *HTTPSender) Send(ctx context.Context, metrics []*wireformat.MetricBatch) (*wireformat.Response, error) {
	b, err := json.Marshal(metrics)
	if err != nil {
		return nil, errors.Trace(err)
	}
	r := bytes.NewBuffer(b)
	client := &http.Client{}
	resp, err := client.Post(s.url, "application/json", r)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, errors.Errorf("failed to send metrics http %v", resp.StatusCode)
	}

	defer resp.Body.Close()
	respReader := json.NewDecoder(resp.Body)
	metricsResponse := wireformat.Response{}
	err = respReader.Decode(&metricsResponse)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &metricsResponse, nil
}
