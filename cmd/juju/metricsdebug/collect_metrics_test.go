// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricsdebug_test

import (
	"fmt"
	//"sort"
	"strings"
	"time"

	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	//"github.com/juju/utils/exec"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/metricsdebug"
	"github.com/juju/juju/testing"
)

type collectMetricsSuite struct {
	testing.FakeJujuHomeSuite
}

var _ = gc.Suite(&collectMetricsSuite{})

func (*collectMetricsSuite) TestTargetArgParsing(c *gc.C) {
	for i, test := range []struct {
		message  string
		args     []string
		units    []string
		services []string
		errMatch string
	}{{
		message:  "no target",
		args:     []string{""},
		errMatch: "You must specify a target, either through --service or --unit",
	}, {
		message:  "too many args",
		args:     []string{"--unit=foo/0", "oops"},
		errMatch: `unrecognized args: \["oops"\]`,
	}, {
		message:  "command to services wordpress and mysql",
		args:     []string{"--service=wordpress,mysql"},
		services: []string{"wordpress", "mysql"},
	}, {
		message: "bad service names",
		args:    []string{"--service", "foo,2,foo/0"},
		errMatch: "" +
			"The following run targets are not valid:\n" +
			"  \"2\" is not a valid service name\n" +
			"  \"foo/0\" is not a valid service name",
	}, {
		message: "command to valid units",
		args:    []string{"--unit=wordpress/0,wordpress/1,mysql/0"},
		units:   []string{"wordpress/0", "wordpress/1", "mysql/0"},
	}, {
		message: "bad unit names",
		args:    []string{"--unit", "foo,2,foo/0", "sudo reboot"},
		errMatch: "" +
			"The following run targets are not valid:\n" +
			"  \"foo\" is not a valid unit name\n" +
			"  \"2\" is not a valid unit name",
	}, {
		message:  "command to mixed valid targets",
		args:     []string{"--unit=wordpress/0,wordpress/1", "--service=mysql"},
		services: []string{"mysql"},
		units:    []string{"wordpress/0", "wordpress/1"},
	}} {
		c.Log(fmt.Sprintf("running test %v: %s", i, test.message))
		cmd := metricsdebug.NewUnwrappedCollectMetricsCommand()
		collectCmd := envcmd.Wrap(cmd)
		testing.TestInit(c, collectCmd, test.args, test.errMatch)
		if test.errMatch == "" {
			c.Check(cmd.Services(), gc.DeepEquals, test.services)
			c.Check(cmd.Units(), gc.DeepEquals, test.units)
		}
	}
}

func (*collectMetricsSuite) TestTimeoutArgParsing(c *gc.C) {
	for i, test := range []struct {
		message  string
		args     []string
		errMatch string
		timeout  time.Duration
	}{{
		message: "default time",
		args:    []string{"--unit=foo/0"},
		timeout: 5 * time.Minute,
	}, {
		message:  "invalid time",
		args:     []string{"--timeout=foo", "--unit=foo/0"},
		errMatch: `invalid value "foo" for flag --timeout: time: invalid duration foo`,
	}, {
		message: "two hours",
		args:    []string{"--timeout=2h", "--unit=foo/0"},
		timeout: 2 * time.Hour,
	}, {
		message: "3 minutes 30 seconds",
		args:    []string{"--timeout=3m30s", "--unit=foo/0"},
		timeout: (3 * time.Minute) + (30 * time.Second),
	}} {
		c.Log(fmt.Sprintf("running test %v: %s", i, test.message))
		cmd := metricsdebug.NewUnwrappedCollectMetricsCommand()
		runCmd := envcmd.Wrap(cmd)
		testing.TestInit(c, runCmd, test.args, test.errMatch)
		if test.errMatch == "" {
			c.Check(cmd.Timeout(), gc.Equals, test.timeout)
		}
	}
}

func (s *collectMetricsSuite) TestRunForMachineAndUnit(c *gc.C) {
	mock := s.setupMockAPI()
	unitResponse := params.CollectMetricsResult{
		UnitId: "unit/0",
	}
	mock.setResponse("unit/0", unitResponse)

	jsonFormatted, err := cmd.FormatJson([]params.CollectMetricsResult{unitResponse})
	c.Assert(err, jc.ErrorIsNil)

	context, err := testing.RunCommand(c, metricsdebug.NewCollectMetricsCommand(),
		"--format=json", "--unit=unit/0",
	)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(testing.Stdout(context), gc.Equals, string(jsonFormatted)+"\n")
}

func (s *collectMetricsSuite) TestBlockRunForMachineAndUnit(c *gc.C) {
	mock := s.setupMockAPI()
	// Block operation
	mock.block = true
	_, err := testing.RunCommand(c, metricsdebug.NewCollectMetricsCommand(),
		"--format=json", "--unit=unit/0",
	)
	c.Assert(err, gc.ErrorMatches, cmd.ErrSilent.Error())
	// msg is logged
	stripped := strings.Replace(c.GetTestLog(), "\n", "", -1)
	c.Check(stripped, gc.Matches, ".*To unblock changes.*")
}

func (s *collectMetricsSuite) setupMockAPI() *mockCollectMetricsAPI {
	mock := &mockCollectMetricsAPI{}
	s.PatchValue(metricsdebug.GetCollectMetricsAPIClient, metricsdebug.GetCollectMetricsAPIClientFunction(mock))
	return mock
}

type mockCollectMetricsAPI struct {
	responses map[string]params.CollectMetricsResult
	block     bool
}

var _ metricsdebug.CollectMetricsClient = (*mockCollectMetricsAPI)(nil)

func (m *mockCollectMetricsAPI) setResponse(id string, resp params.CollectMetricsResult) {
	if m.responses == nil {
		m.responses = make(map[string]params.CollectMetricsResult)
	}
	m.responses[id] = resp
}

func (*mockCollectMetricsAPI) Close() error {
	return nil
}

func (m *mockCollectMetricsAPI) CollectMetrics(collect params.CollectMetricsParams) ([]params.CollectMetricsResult, error) {
	var result []params.CollectMetricsResult
	if m.block {
		return result, common.OperationBlockedError("the operation has been blocked")
	}
	// mock ignores services
	for _, id := range collect.Units {
		response, found := m.responses[id]
		if found {
			result = append(result, response)
		}
	}
	return result, nil
}
