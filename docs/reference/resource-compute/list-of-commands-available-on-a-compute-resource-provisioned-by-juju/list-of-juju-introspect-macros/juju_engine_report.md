(juju_engine_report)=
# `juju_engine_report`

The engine report is a window into the internals of the agent. This is primarily useful to developers to help debug problems that may be occurring in deployed systems.

In order to manage complexity in the Juju agents, there are *workers* that have very distinct and limited purpose. Workers can have dependencies on other workers. The [dependency engine](https://godoc.org/gopkg.in/juju/worker.v1/dependency) is the entity that runs the workers and deals with those dependencies. The `juju_engine_report` is the current view into the dependency engine running the agent's workers.

## Usage
Can be run on any Juju machine, expected state is different for controller machines, ha, and machines running workloads.
```code
juju_engine_report
```

## Example output
```text
manifolds:
  agent:
    inputs: []
    report:
      agent: machine-0
      model-uuid: 1b13f1f5-c0cf-47c5-86ae-55c393e19405
    resource-log: []
    start-count: 1
    started: 2018-08-09 22:01:39
    state: started
  api-address-updater:
    inputs:
    - agent
    - api-caller
    - migration-fortress
    - migration-inactive-flag
    report:
      servers:
      - - 10.173.141.131:17070
        - 127.0.0.1:17070
        - '[::1]:17070'
    resource-log:
    - name: migration-inactive-flag
      type: '*engine.Flag'
    - name: migration-fortress
      type: '*fortress.Guest'
    - name: agent
      type: '*agent.Agent'
    - name: api-caller
      type: '*base.APICaller'
    start-count: 1
    started: 2018-08-09 22:01:41
    state: started
  api-caller:
    inputs:
    - agent
    - api-config-watcher
    resource-log:
    - name: agent
      type: '*agent.Agent'
    start-count: 1
    started: 2018-08-09 22:01:40
    state: started
# and many more
```

## Interesting Output

* Dependencies with a larger `start_count` than others. This can indicate that the worker is bouncing.

* Dependencies that are stopped when they should be started. Perhaps the `inputs` are not starting.

* Dependencies that are started which should be stopped. Can prevent a unit from upgrading or migrating if the workers do not quiesce.

* A controllers engine report will contain the model cache contents as of 2.9

* The report from an individual unit contains the local-state and relation, formerly in a file on the unit:
```
                 report:
                    local-state:
                      hook-kind: continue
                      hook-step: pending
                      installed: true
                      leader: true
                      removed: false
                      started: true
                      stopped: false
                    relations:
                      "0":
                        application-members:
                          ntp: 0
                        dying: false
                        endpoint: ntp-peers
                        is-peer: false
                        members: {}
                        relation: ntp:ntp-peers
                      "1":
                        application-members:
                          ubuntu: 0
                        dying: false
                        endpoint: juju-info
                        is-peer: false
                        members:
                          ubuntu/0: 0
                        relation: ntp:juju-info ubuntu:juju-info
```
