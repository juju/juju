// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricsender

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"net/http"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/metricsender/wireformat"
)

var (
	metricsCertsPool *x509.CertPool
	metricsHost      string
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
	t := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: metricsCertsPool}}
	client := &http.Client{Transport: t}
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
