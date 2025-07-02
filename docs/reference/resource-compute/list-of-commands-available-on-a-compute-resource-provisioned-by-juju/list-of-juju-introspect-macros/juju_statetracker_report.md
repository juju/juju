(juju_statetracker_report)=
# `juju_statetracker_report`

The statetracker report is built using the Go runtime profiling infrastructure.
This primarily a diagnostic tool for developers. Whenever a new mongo state
instance is created, the execution stack is added to the statetracker profile.
The report displays the content of that profile.

## Usage
Must be run on a juju controller machine.
```code
juju_statetracker_report
```

## Example output
```text
$ juju_statetracker_report 
juju/state/tracker profile: total 2
1 @ 0x21bb86c 0x21bb2e5 0x21c68b0 0x3fd23b9 0x3fd0a05 0x3f13cb7 0x233e712 0x233df72 0x233dfb3 0x482681
#       0x21bb86b       github.com/juju/juju/state.newState+0x44b                                               /home/ian/juju/go/src/juju/juju/state/open.go:189
#       0x21bb2e4       github.com/juju/juju/state.open+0x84                                                    /home/ian/juju/go/src/juju/juju/state/open.go:105
#       0x21c68af       github.com/juju/juju/state.OpenStatePool+0x20f                                          /home/ian/juju/go/src/juju/juju/state/pool.go:157
#       0x3fd23b8       github.com/juju/juju/cmd/jujud/agent.openStatePool+0x278                                /home/ian/juju/go/src/juju/juju/cmd/jujud/agent/machine.go:1174
#       0x3fd0a04       github.com/juju/juju/cmd/jujud/agent.(*MachineAgent).initState+0x184                    /home/ian/juju/go/src/juju/juju/cmd/jujud/agent/machine.go:956
#       0x3f13cb6       github.com/juju/juju/cmd/jujud/agent/machine.commonManifolds.Manifold.func11+0x156      /home/ian/juju/go/src/juju/juju/internal/worker/state/manifold.go:86
#       0x233e711       github.com/juju/worker/v3/dependency.(*Engine).runWorker.func1+0x351                    /home/ian/go/pkg/mod/github.com/juju/worker/v3@v3.5.0/dependency/engine.go:520
#       0x233df71       github.com/juju/worker/v3/dependency.(*Engine).runWorker.func2+0xb1                     /home/ian/go/pkg/mod/github.com/juju/worker/v3@v3.5.0/dependency/engine.go:524
#       0x233dfb2       github.com/juju/worker/v3/dependency.(*Engine).runWorker+0xf2                           /home/ian/go/pkg/mod/github.com/juju/worker/v3@v3.5.0/dependency/engine.go:555

1 @ 0x21bb86c 0x21c7ae7 0x21c74f3 0x23794ab 0x16a67ab 0x482681
#       0x21bb86b       github.com/juju/juju/state.newState+0x44b                                       /home/ian/juju/go/src/juju/juju/state/open.go:189
#       0x21c7ae6       github.com/juju/juju/state.(*StatePool).openState+0x86                          /home/ian/juju/go/src/juju/juju/state/pool.go:299
#       0x21c74f2       github.com/juju/juju/state.(*StatePool).Get+0x272                               /home/ian/juju/go/src/juju/juju/state/pool.go:277
#       0x23794aa       github.com/juju/juju/internal/worker/state.(*modelStateWorker).loop+0x4a        /home/ian/juju/go/src/juju/juju/internal/worker/state/worker.go:158
#       0x16a67aa       gopkg.in/tomb%2ev2.(*Tomb).run+0x2a                                             /home/ian/go/pkg/mod/gopkg.in/tomb.v2@v2.0.0-20161208151619-d5d1b5820637/tomb.go:163
```