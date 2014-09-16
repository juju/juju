// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build sender

package metricsender_test

import (
	"crypto/x509"
	"io/ioutil"
	"os"
	stdtesting "testing"
	"time"

	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state/metricsender"
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
	unit := s.Factory.MakeUnit(c, &factory.UnitParams{SetCharmURL: true})
	now := time.Now()
	certPool := x509.NewCertPool()
	certPool.AppendCertsFromPEM(cert)
	cleanup := metricsender.PatchHostAndCertPool(host, certPool)
	defer cleanup()
	ma := s.Factory.MakeMetric(c, &factory.MetricParams{Unit: unit, Sent: false, Time: &now})
	mb := s.Factory.MakeMetric(c, &factory.MetricParams{Unit: unit, Sent: false, Time: &now})
	mc := s.Factory.MakeMetric(c, &factory.MetricParams{Unit: unit, Sent: false, Time: &now})
	sender := &metricsender.DefaultSender{}
	err = s.State.SendMetrics(sender)
	c.Assert(err, gc.IsNil)
	err = ma.Refresh()
	c.Assert(err, gc.IsNil)
	err = mb.Refresh()
	c.Assert(err, gc.IsNil)
	err = mc.Refresh()
	c.Assert(err, gc.IsNil)

	c.Assert(ma.Sent(), jc.IsTrue)
	c.Assert(mb.Sent(), jc.IsTrue)
	c.Assert(mc.Sent(), jc.IsTrue)

}
