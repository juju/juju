// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricsender

import (
	"bytes"
	"encoding/json"
	"net/http"

	"github.com/juju/errors"
	wireformat "github.com/juju/romulus/wireformat/metrics"
)

var (
	metricsHost string = "https://api.jujucharms.com/omnibus/v2/metrics"
)

// HttpSender is the default used for sending
// metrics to the collector service.
type HttpSender struct {
}

// Send sends the given metrics to the collector service.
func (s *HttpSender) Send(metrics []*wireformat.MetricBatch) (*wireformat.Response, error) {
	b, err := json.Marshal(metrics)
	if err != nil {
		return nil, errors.Trace(err)
	}
	r := bytes.NewBuffer(b)
	client := &http.Client{}
	resp, err := client.Post(metricsHost, "application/json", r)
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
