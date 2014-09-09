// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build sender

package metricsender

import (
	"crypto/x509"
	"io/ioutil"
	"os"
	stdtesting "testing"
	"time"

	gc "launchpad.net/gocheck"

	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

type SenderSuite struct {
	jujutesting.JujuConnSuite
}

var _ = gc.Suite(&SenderSuite{})

// TestPackage integrates the tests into gotest.
func TestPackage(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}

// TestDefaultSender check that if the default sender
// is in use metrics get sent
func (s *SenderSuite) TestDefaultSender(c *gc.C) {
	caCert := os.Getenv("JUJU_METRIC_CACERT")
	host := os.Getenv("JUJU_METRIC_HOST")
	if caCert == "" || host == "" {
		c.Skip("Not enough options provided to test default sender")
	}
	cert, err := ioutil.ReadFile(caCert)
	c.Assert(err, gc.IsNil)
	unit := s.Factory.MakeUnit(c, nil)
	now := time.Now()
	certPool := x509.NewCertPool()
	certPool.AppendCertsFromPEM(cert)
	s.PatchValue(&metricsHost, host)
	s.PatchValue(&metricsCertsPool, certPool)
	s.Factory.MakeMetric(c, &factory.MetricParams{Unit: unit, Sent: false, Time: &now})
	s.Factory.MakeMetric(c, &factory.MetricParams{Unit: unit, Sent: false, Time: &now})
	s.Factory.MakeMetric(c, &factory.MetricParams{Unit: unit, Sent: false, Time: &now})
	err = s.State.SendMetrics()
	c.Assert(err, gc.IsNil)
	sent, err := s.State.CountofSentMetrics()
	c.Assert(err, gc.IsNil)
	c.Assert(sent, gc.Equals, 3)

}
