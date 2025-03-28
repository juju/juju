(juju_goroutines)=
# `juju_goroutines`

The `juju_goroutines` function allows the operator to quickly get a list of running goroutines from the agent.

When called without any argument the goroutines for the machine agent are returned.

The output of this is mostly just useful for Juju developers to help identify where things may be stuck.

```bash
$ juju_goroutines 
Querying @jujud-machine-0 introspection socket: /debug/pprof/goroutine?debug=1
goroutine profile: total 234
19 @ 0x42f59a 0x42f64e 0x406c62 0x40691b 0x951ada 0x9c34ed 0x9c0177 0x45b211
#	0x951ad9	gopkg.in/tomb%2ev1.(*Tomb).Wait+0x49				/home/tim/go/src/gopkg.in/tomb.v1/tomb.go:113
#	0x9c34ec	github.com/juju/juju/api/watcher.(*commonWatcher).Wait+0x2c	/home/tim/go/src/github.com/juju/juju/api/watcher/watcher.go:138
#	0x9c0176	github.com/juju/juju/internal/worker/catacomb.(*Catacomb).add.func1+0x86	/home/tim/go/src/github.com/juju/juju/internal/worker/catacomb/catacomb.go:175

19 @ 0x42f59a 0x42f64e 0x406c62 0x40691b 0x9599f9 0xa20249 0x9c6673 0x9c69e9 0x45b211
#	0x9599f8	github.com/juju/juju/rpc.(*Conn).Call+0x128				/home/tim/go/src/github.com/juju/juju/rpc/client.go:148
#	0xa20248	github.com/juju/juju/api.(*state).APICall+0x1c8				/home/tim/go/src/github.com/juju/juju/api/apiclient.go:917
#	0x9c6672	github.com/juju/juju/api/watcher.makeWatcherAPICaller.func1+0x142	/home/tim/go/src/github.com/juju/juju/api/watcher/watcher.go:54
#	0x9c69e8	github.com/juju/juju/api/watcher.(*commonWatcher).commonLoop.func2+0xe8	/home/tim/go/src/github.com/juju/juju/api/watcher/watcher.go:104

19 @ 0x42f59a 0x42f64e 0x406c62 0x40691b 0x9c6732 0x45b211
#	0x9c6731	github.com/juju/juju/api/watcher.(*commonWatcher).commonLoop.func1+0x71	/home/tim/go/src/github.com/juju/juju/api/watcher/watcher.go:88

19 @ 0x42f59a 0x42f64e 0x43ff34 0x43fb59 0x4646a2 0x9c3438 0x45b211
#	0x43fb58	sync.runtime_Semacquire+0x38						/snap/go/2130/src/runtime/sema.go:56
#	0x4646a1	sync.(*WaitGroup).Wait+0x71						/snap/go/2130/src/sync/waitgroup.go:129
#	0x9c3437	github.com/juju/juju/api/watcher.(*commonWatcher).commonLoop+0xf7	/home/tim/go/src/github.com/juju/juju/api/watcher/watcher.go:128

19 @ 0x42f59a 0x43f2b0 0x9c02d8 0x45b211
#	0x9c02d7	github.com/juju/juju/internal/worker/catacomb.(*Catacomb).add.func2+0x107	/home/tim/go/src/github.com/juju/juju/internal/worker/catacomb/catacomb.go:181

15 @ 0x42f59a 0x43f2b0 0x9bffcd 0x45b211
#	0x9bffcc	github.com/juju/juju/internal/worker/catacomb.Invoke.func2+0x14c	/home/tim/go/src/github.com/juju/juju/internal/worker/catacomb/catacomb.go:101

13 @ 0x42f59a 0x42f64e 0x406c62 0x40691b 0xe5ee92 0xe60115 0x45b211
#	0xe5ee91	github.com/juju/juju/internal/worker/fortress.(*fortress).Visit+0x191	/home/tim/go/src/github.com/juju/juju/internal/worker/fortress/fortress.go:63
#	0xe60114	github.com/juju/juju/internal/worker/fortress.Occupy.func2+0x44		/home/tim/go/src/github.com/juju/juju/internal/worker/fortress/occupy.go:50

11 @ 0x42f59a 0x42f64e 0x406c62 0x40695b 0x9c3813 0x9c6c73 0x45b211
#	0x9c3812	github.com/juju/juju/api/watcher.(*notifyWatcher).loop+0x1c2	/home/tim/go/src/github.com/juju/juju/api/watcher/watcher.go:180
#	0x9c6c72	github.com/juju/juju/api/watcher.NewNotifyWatcher.func1+0x52	/home/tim/go/src/github.com/juju/juju/api/watcher/watcher.go:160

7 @ 0x42f59a 0x43f2b0 0x9c0e35 0x9c1aca 0x9bfdd5 0x9c00a1 0x45b211
#	0x9c0e34	github.com/juju/juju/watcher.(*NotifyWorker).loop+0x154						/home/tim/go/src/github.com/juju/juju/watcher/notify.go:90
#	0x9c1ac9	github.com/juju/juju/watcher.(*NotifyWorker).(github.com/juju/juju/watcher.loop)-fm+0x29	/home/tim/go/src/github.com/juju/juju/watcher/notify.go:71
#	0x9bfdd4	github.com/juju/juju/internal/worker/catacomb.runSafely+0x54						/home/tim/go/src/github.com/juju/juju/internal/worker/catacomb/catacomb.go:289
#	0x9c00a0	github.com/juju/juju/internal/worker/catacomb.Invoke.func3+0x80						/home/tim/go/src/github.com/juju/juju/internal/worker/catacomb/catacomb.go:116

6 @ 0x42f59a 0x42f64e 0x406c62 0x40691b 0x951ada 0x9bf89d 0x9c11f1 0xe5e49f 0xe5bbd7 0x45b211
#	0x951ad9	gopkg.in/tomb%2ev1.(*Tomb).Wait+0x49					/home/tim/go/src/gopkg.in/tomb.v1/tomb.go:113
#	0x9bf89c	github.com/juju/juju/internal/worker/catacomb.(*Catacomb).Wait+0x2c		/home/tim/go/src/github.com/juju/juju/internal/worker/catacomb/catacomb.go:204
#	0x9c11f0	github.com/juju/juju/watcher.(*NotifyWorker).Wait+0x30			/home/tim/go/src/github.com/juju/juju/watcher/notify.go:138
#	0xe5e49e	github.com/juju/juju/internal/worker/dependency.(*Engine).runWorker.func2+0x4ce	/home/tim/go/src/github.com/juju/juju/internal/worker/dependency/engine.go:464
#	0xe5bbd6	github.com/juju/juju/internal/worker/dependency.(*Engine).runWorker+0x1c6	/home/tim/go/src/github.com/juju/juju/internal/worker/dependency/engine.go:468

6 @ 0x42f59a 0x42f64e 0x406c62 0x40691b 0x951ada 0x9bf89d 0x9c11f1 0xe600bb 0xe5f6e1 0x45b211
#	0x951ad9	gopkg.in/tomb%2ev1.(*Tomb).Wait+0x49				/home/tim/go/src/gopkg.in/tomb.v1/tomb.go:113
#	0x9bf89c	github.com/juju/juju/internal/worker/catacomb.(*Catacomb).Wait+0x2c	/home/tim/go/src/github.com/juju/juju/internal/worker/catacomb/catacomb.go:204
#	0x9c11f0	github.com/juju/juju/watcher.(*NotifyWorker).Wait+0x30		/home/tim/go/src/github.com/juju/juju/watcher/notify.go:138
#	0xe600ba	github.com/juju/juju/internal/worker/fortress.Occupy.func1+0xca		/home/tim/go/src/github.com/juju/juju/internal/worker/fortress/occupy.go:38
#	0xe5f6e0	github.com/juju/juju/internal/worker/fortress.guestTicket.complete+0x40	/home/tim/go/src/github.com/juju/juju/internal/worker/fortress/fortress.go:151

[extra bits snipped for brevity]
```

To call on a unit agent, the agent name as defined in `/var/lib/juju/agents/` should be specified as the second argument.

```bash
$ juju_goroutines unit-ubuntu-lite-2
Querying @jujud-unit-ubuntu-lite-2 introspection socket: /debug/pprof/goroutine?debug=1
goroutine profile: total 216
19 @ 0x42f59a 0x43f2b0 0x9c02d8 0x45b211
#	0x9c02d7	github.com/juju/juju/internal/worker/catacomb.(*Catacomb).add.func2+0x107	/home/tim/go/src/github.com/juju/juju/internal/worker/catacomb/catacomb.go:181

17 @ 0x42f59a 0x42f64e 0x406c62 0x40691b 0x951ada 0x9c34ed 0x9c0177 0x45b211
#	0x951ad9	gopkg.in/tomb%2ev1.(*Tomb).Wait+0x49				/home/tim/go/src/gopkg.in/tomb.v1/tomb.go:113
#	0x9c34ec	github.com/juju/juju/api/watcher.(*commonWatcher).Wait+0x2c	/home/tim/go/src/github.com/juju/juju/api/watcher/watcher.go:138
#	0x9c0176	github.com/juju/juju/internal/worker/catacomb.(*Catacomb).add.func1+0x86	/home/tim/go/src/github.com/juju/juju/internal/worker/catacomb/catacomb.go:175

17 @ 0x42f59a 0x42f64e 0x406c62 0x40691b 0x9599f9 0xa20249 0x9c6673 0x9c69e9 0x45b211
#	0x9599f8	github.com/juju/juju/rpc.(*Conn).Call+0x128				/home/tim/go/src/github.com/juju/juju/rpc/client.go:148
#	0xa20248	github.com/juju/juju/api.(*state).APICall+0x1c8				/home/tim/go/src/github.com/juju/juju/api/apiclient.go:917
#	0x9c6672	github.com/juju/juju/api/watcher.makeWatcherAPICaller.func1+0x142	/home/tim/go/src/github.com/juju/juju/api/watcher/watcher.go:54
#	0x9c69e8	github.com/juju/juju/api/watcher.(*commonWatcher).commonLoop.func2+0xe8	/home/tim/go/src/github.com/juju/juju/api/watcher/watcher.go:104

17 @ 0x42f59a 0x42f64e 0x406c62 0x40691b 0x9c6732 0x45b211
#	0x9c6731	github.com/juju/juju/api/watcher.(*commonWatcher).commonLoop.func1+0x71	/home/tim/go/src/github.com/juju/juju/api/watcher/watcher.go:88

17 @ 0x42f59a 0x42f64e 0x43ff34 0x43fb59 0x4646a2 0x9c3438 0x45b211
#	0x43fb58	sync.runtime_Semacquire+0x38						/snap/go/2130/src/runtime/sema.go:56
#	0x4646a1	sync.(*WaitGroup).Wait+0x71						/snap/go/2130/src/sync/waitgroup.go:129
#	0x9c3437	github.com/juju/juju/api/watcher.(*commonWatcher).commonLoop+0xf7	/home/tim/go/src/github.com/juju/juju/api/watcher/watcher.go:128

13 @ 0x42f59a 0x42f64e 0x406c62 0x40695b 0x9c3813 0x9c6c73 0x45b211
#	0x9c3812	github.com/juju/juju/api/watcher.(*notifyWatcher).loop+0x1c2	/home/tim/go/src/github.com/juju/juju/api/watcher/watcher.go:180
#	0x9c6c72	github.com/juju/juju/api/watcher.NewNotifyWatcher.func1+0x52	/home/tim/go/src/github.com/juju/juju/api/watcher/watcher.go:160

11 @ 0x42f59a 0x42f64e 0x406c62 0x40691b 0xe5ee92 0xe60115 0x45b211
#	0xe5ee91	github.com/juju/juju/internal/worker/fortress.(*fortress).Visit+0x191	/home/tim/go/src/github.com/juju/juju/internal/worker/fortress/fortress.go:63
#	0xe60114	github.com/juju/juju/internal/worker/fortress.Occupy.func2+0x44		/home/tim/go/src/github.com/juju/juju/internal/worker/fortress/occupy.go:50

10 @ 0x42f59a 0x42f64e 0x406c62 0x40695b 0xa2f2b8 0x45b211
#	0xa2f2b7	gopkg.in/natefinch/lumberjack%2ev2.(*Logger).millRun+0x57	/home/tim/go/src/gopkg.in/natefinch/lumberjack.v2/lumberjack.go:379

10 @ 0x42f59a 0x43f2b0 0x9bffcd 0x45b211
#	0x9bffcc	github.com/juju/juju/internal/worker/catacomb.Invoke.func2+0x14c	/home/tim/go/src/github.com/juju/juju/internal/worker/catacomb/catacomb.go:101

5 @ 0x42f59a 0x42f64e 0x406c62 0x40691b 0x951ada 0x9bf89d 0x9c11f1 0xe5e49f 0xe5bbd7 0x45b211
#	0x951ad9	gopkg.in/tomb%2ev1.(*Tomb).Wait+0x49					/home/tim/go/src/gopkg.in/tomb.v1/tomb.go:113
#	0x9bf89c	github.com/juju/juju/internal/worker/catacomb.(*Catacomb).Wait+0x2c		/home/tim/go/src/github.com/juju/juju/internal/worker/catacomb/catacomb.go:204
#	0x9c11f0	github.com/juju/juju/watcher.(*NotifyWorker).Wait+0x30			/home/tim/go/src/github.com/juju/juju/watcher/notify.go:138
#	0xe5e49e	github.com/juju/juju/internal/worker/dependency.(*Engine).runWorker.func2+0x4ce	/home/tim/go/src/github.com/juju/juju/internal/worker/dependency/engine.go:464
#	0xe5bbd6	github.com/juju/juju/internal/worker/dependency.(*Engine).runWorker+0x1c6	/home/tim/go/src/github.com/juju/juju/internal/worker/dependency/engine.go:468

5 @ 0x42f59a 0x42f64e 0x406c62 0x40691b 0x951ada 0x9bf89d 0x9c11f1 0xe600bb 0xe5f6e1 0x45b211
#	0x951ad9	gopkg.in/tomb%2ev1.(*Tomb).Wait+0x49				/home/tim/go/src/gopkg.in/tomb.v1/tomb.go:113
#	0x9bf89c	github.com/juju/juju/internal/worker/catacomb.(*Catacomb).Wait+0x2c	/home/tim/go/src/github.com/juju/juju/internal/worker/catacomb/catacomb.go:204
#	0x9c11f0	github.com/juju/juju/watcher.(*NotifyWorker).Wait+0x30		/home/tim/go/src/github.com/juju/juju/watcher/notify.go:138
#	0xe600ba	github.com/juju/juju/internal/worker/fortress.Occupy.func1+0xca		/home/tim/go/src/github.com/juju/juju/internal/worker/fortress/occupy.go:38
#	0xe5f6e0	github.com/juju/juju/internal/worker/fortress.guestTicket.complete+0x40	/home/tim/go/src/github.com/juju/juju/internal/worker/fortress/fortress.go:151
[extra bits snipped for brevity]
```
