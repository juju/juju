package converter_test

import (
	"net"
	stdtesting "testing"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/converter"
)

func TestPackage(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type UnitConverterSuite struct {
	jujutesting.JujuConnSuite
}

var _ = gc.Suite(&UnitConverterSuite{})

func (s *UnitConverterSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	err := s.State.SetAPIHostPorts(nil)
	c.Assert(err, jc.ErrorIsNil)
	// By default mock these to better isolate the test from the real machine.
	s.PatchValue(&network.InterfaceByNameAddrs, func(string) ([]net.Addr, error) {
		return nil, nil
	})
	s.PatchValue(&network.LXCNetDefaultConfig, "")
}

func (s *UnitConverterSuite) TestStartStop(c *gc.C) {
	st, _ := s.OpenAPIAsNewMachine(c, state.JobHostUnits)
	worker := converter.NewUnitConverter(st.Machiner(), &apiAddressSetter{})
	worker.Kill()
	c.Assert(worker.Wait(), gc.IsNil)
}

/*
func (s *converterSuite) TestWorkerCatchesConverterEvent(c *gc.C) {
	wrk, err := converter.NewConverter(s.converterState, s.AgentConfigForTag(c, s.machine.Tag()), s.lock)
	c.Assert(err, jc.ErrorIsNil)
	err = s.converterState.RequestConverter()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(wrk.Wait(), gc.Equals, worker.ErrConverterMachine)
}

func (s *converterSuite) TestContainerCatchesParentFlag(c *gc.C) {
	wrk, err := converter.NewConverter(s.ctConverterState, s.AgentConfigForTag(c, s.ct.Tag()), s.lock)
	c.Assert(err, jc.ErrorIsNil)
	err = s.converterState.RequestConverter()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(wrk.Wait(), gc.Equals, worker.ErrShutdownMachine)
}

func (s *converterSuite) TestCleanupIsDoneOnBoot(c *gc.C) {
	s.lock.Lock(converter.ConverterMessage)

	wrk, err := converter.NewConverter(s.converterState, s.AgentConfigForTag(c, s.machine.Tag()), s.lock)
	c.Assert(err, jc.ErrorIsNil)
	wrk.Kill()
	c.Assert(wrk.Wait(), gc.IsNil)

	c.Assert(s.lock.IsLocked(), jc.IsFalse)
}
*/
