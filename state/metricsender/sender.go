// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build sender

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
type defaultSender struct {
}

func init() {
	state.MetricSend = &defaultSender{}
}

// Send sends the given metrics to the collector service.
func (s *defaultSender) Send(metrics []*state.MetricBatch) error {
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
