// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package metricsender contains types and functions for sending
// metrics from a state server to a remote metric collector.
package metricsender

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"net/http"

	"github.com/juju/juju/state"
)

var (
	metricsCertsPool *x509.CertPool
	metricsHost      string
)

// DefaultSender is the default used for sending
// metrics to the collector service.
type DefaultSender struct {
}

// Send sends the given metrics to the collector service.
func (s *DefaultSender) Send(metrics []*state.MetricBatch) error {
	b, err := json.Marshal(metrics)
	if err != nil {
		return err
	}
	r := bytes.NewBuffer(b)
	t := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: metricsCertsPool}}
	client := &http.Client{Transport: t}
	resp, err := client.Post(metricsHost, "application/json", r)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}
