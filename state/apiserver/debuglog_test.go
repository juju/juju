// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/testing/testbase"
)

type streamSuite struct {
	testbase.LoggingSuite
}

var _ = gc.Suite(&stateSuite{})

func (s *stateSuite) TestFilterInclude(c *gc.C) {

}

var logLines = `
machine-0: 2014-03-24 22:34:25 INFO juju.cmd supercommand.go:297 running juju-1.17.7.1-trusty-amd64 [gc]
machine-0: 2014-03-24 22:34:25 INFO juju.cmd.jujud machine.go:127 machine agent machine-0 start (1.17.7.1-trusty-amd64 [gc])
machine-0: 2014-03-24 22:34:25 DEBUG juju.agent agent.go:384 read agent config, format "1.18"
machine-0: 2014-03-24 22:34:25 INFO juju.cmd.jujud machine.go:155 Starting StateWorker for machine-0
machine-0: 2014-03-24 22:34:25 INFO juju runner.go:262 worker: start "state"
machine-0: 2014-03-24 22:34:25 INFO juju.state open.go:80 opening state; mongo addresses: ["localhost:37017"]; entity "machine-0"
machine-0: 2014-03-24 22:34:25 INFO juju runner.go:262 worker: start "api"
machine-0: 2014-03-24 22:34:25 INFO juju apiclient.go:114 state/api: dialing "wss://localhost:17070/"
machine-0: 2014-03-24 22:34:25 INFO juju runner.go:262 worker: start "termination"
machine-0: 2014-03-24 22:34:25 ERROR juju apiclient.go:119 state/api: websocket.Dial wss://localhost:17070/: dial tcp 127.0.0.1:17070: connection refused
machine-0: 2014-03-24 22:34:25 ERROR juju runner.go:220 worker: exited "api": websocket.Dial wss://localhost:17070/: dial tcp 127.0.0.1:17070: connection refused
machine-0: 2014-03-24 22:34:25 INFO juju runner.go:254 worker: restarting "api" in 3s
machine-0: 2014-03-24 22:34:25 INFO juju.state open.go:118 connection established
machine-0: 2014-03-24 22:34:25 DEBUG juju.utils gomaxprocs.go:24 setting GOMAXPROCS to 8
machine-0: 2014-03-24 22:34:25 INFO juju runner.go:262 worker: start "local-storage"
machine-0: 2014-03-24 22:34:25 INFO juju runner.go:262 worker: start "instancepoller"
machine-0: 2014-03-24 22:34:25 INFO juju runner.go:262 worker: start "apiserver"
machine-0: 2014-03-24 22:34:25 INFO juju runner.go:262 worker: start "resumer"
machine-0: 2014-03-24 22:34:25 INFO juju runner.go:262 worker: start "cleaner"
machine-0: 2014-03-24 22:34:25 INFO juju.state.apiserver apiserver.go:43 listening on "[::]:17070"
machine-0: 2014-03-24 22:34:25 INFO juju runner.go:262 worker: start "minunitsworker"
machine-0: 2014-03-24 22:34:28 INFO juju runner.go:262 worker: start "api"
machine-0: 2014-03-24 22:34:28 INFO juju apiclient.go:114 state/api: dialing "wss://localhost:17070/"
machine-0: 2014-03-24 22:34:28 INFO juju.state.apiserver apiserver.go:131 [1] API connection from 127.0.0.1:36491
machine-0: 2014-03-24 22:34:28 INFO juju apiclient.go:124 state/api: connection established
machine-0: 2014-03-24 22:34:28 DEBUG juju.state.apiserver apiserver.go:120 <- [1] <unknown> {"RequestId":1,"Type":"Admin","Request":"Login","Params":{"AuthTag":"machine-0","Password":"ARbW7iCV4LuMugFEG+Y4e0yr","Nonce":"user-admin:bootstrap"}}
machine-0: 2014-03-24 22:34:28 DEBUG juju.state.apiserver apiserver.go:127 -> [1] machine-0 10.305679ms {"RequestId":1,"Response":{}} Admin[""].Login
machine-1: 2014-03-24 22:36:28 INFO juju.cmd supercommand.go:297 running juju-1.17.7.1-precise-amd64 [gc]
machine-1: 2014-03-24 22:36:28 INFO juju.cmd.jujud machine.go:127 machine agent machine-1 start (1.17.7.1-precise-amd64 [gc])
machine-1: 2014-03-24 22:36:28 DEBUG juju.agent agent.go:384 read agent config, format "1.18"
machine-1: 2014-03-24 22:36:28 INFO juju runner.go:262 worker: start "api"
machine-1: 2014-03-24 22:36:28 INFO juju apiclient.go:114 state/api: dialing "wss://10.0.3.1:17070/"
machine-1: 2014-03-24 22:36:28 INFO juju runner.go:262 worker: start "termination"
machine-1: 2014-03-24 22:36:28 INFO juju apiclient.go:124 state/api: connection established
machine-1: 2014-03-24 22:36:28 DEBUG juju.agent agent.go:523 writing configuration file
machine-1: 2014-03-24 22:36:28 INFO juju runner.go:262 worker: start "upgrader"
machine-1: 2014-03-24 22:36:28 INFO juju runner.go:262 worker: start "upgrade-steps"
machine-1: 2014-03-24 22:36:28 INFO juju runner.go:262 worker: start "machiner"
machine-1: 2014-03-24 22:36:28 INFO juju.cmd.jujud machine.go:458 upgrade to 1.17.7.1-precise-amd64 already completed.
machine-1: 2014-03-24 22:36:28 INFO juju.cmd.jujud machine.go:445 upgrade to 1.17.7.1-precise-amd64 completed.
unit-ubuntu-0: 2014-03-24 22:36:28 INFO juju.cmd supercommand.go:297 running juju-1.17.7.1-precise-amd64 [gc]
unit-ubuntu-0: 2014-03-24 22:36:28 DEBUG juju.agent agent.go:384 read agent config, format "1.18"
unit-ubuntu-0: 2014-03-24 22:36:28 INFO juju.jujud unit.go:76 unit agent unit-ubuntu-0 start (1.17.7.1-precise-amd64 [gc])
unit-ubuntu-0: 2014-03-24 22:36:28 INFO juju runner.go:262 worker: start "api"
unit-ubuntu-0: 2014-03-24 22:36:28 INFO juju apiclient.go:114 state/api: dialing "wss://10.0.3.1:17070/"
unit-ubuntu-0: 2014-03-24 22:36:28 INFO juju apiclient.go:124 state/api: connection established
unit-ubuntu-0: 2014-03-24 22:36:28 DEBUG juju.agent agent.go:523 writing configuration file
unit-ubuntu-0: 2014-03-24 22:36:28 INFO juju runner.go:262 worker: start "upgrader"
unit-ubuntu-0: 2014-03-24 22:36:28 INFO juju runner.go:262 worker: start "logger"
unit-ubuntu-0: 2014-03-24 22:36:28 DEBUG juju.worker.logger logger.go:35 initial log config: "<root>=DEBUG"
unit-ubuntu-0: 2014-03-24 22:36:28 INFO juju runner.go:262 worker: start "uniter"
unit-ubuntu-0: 2014-03-24 22:36:28 DEBUG juju.worker.logger logger.go:60 logger setup
unit-ubuntu-0: 2014-03-24 22:36:28 INFO juju runner.go:262 worker: start "rsyslog"
unit-ubuntu-0: 2014-03-24 22:36:28 DEBUG juju.worker.rsyslog worker.go:76 starting rsyslog worker mode 1 for "unit-ubuntu-0" "tim-local"
`[1:]
