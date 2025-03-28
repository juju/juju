(juju_heap_profile)=
# `juju_heap_profile`

The heap profile provides memory allocation samples. Helpful to monitor current memory usage and find memory leaks. This is primarily useful to developers to help debug problems that may be occurring in deployed systems. 



## Usage
Can be run on any juju machine. Suggest putting the output in a file, so it can be compared at different points in time.

```text
juju_heap_profile > heap_profile.01
```

## Example output

```text
heap profile: 31: 694464 [33638: 106713992] @ heap/1048576
1: 196608 [1: 196608] @ 0x2d3fa53 0x2d3f9f6 0x2d40545 0x2d4030f 0x2d05ddb 0x2d0479f 0x2d05bf1 0x2d26c32 0x8a3cef 0x2d0381d 0x2d04146 0x2d008ba 0x2d01fbe 0x8a779b 0x8a2797 0x468fc1
#       0x2d3fa52       github.com/juju/juju/core/logger.NewBufferedLogger+0x192                        /home/heather/work-test/src/github.com/juju/juju/core/logger/buf.go:43
#       0x2d3f9f5       github.com/juju/juju/apiserver.(*apiServerLoggers).getLogger+0x135              /home/heather/work-test/src/github.com/juju/juju/apiserver/logsink.go:66
#       0x2d40544       github.com/juju/juju/apiserver.(*agentLoggingStrategy).init+0x1c4               /home/heather/work-test/src/github.com/juju/juju/apiserver/logsink.go:175
#       0x2d4030e       github.com/juju/juju/apiserver.newAgentLogWriteCloserFunc.func1+0xae            /home/heather/work-test/src/github.com/juju/juju/apiserver/logsink.go:147
#       0x2d05dda       github.com/juju/juju/apiserver/logsink.(*logSinkHandler).ServeHTTP.func1+0x19a  /home/heather/work-test/src/github.com/juju/juju/apiserver/logsink/logsink.go:196
#       0x2d0479e       github.com/juju/juju/apiserver/websocket.Serve+0xde                             /home/heather/work-test/src/github.com/juju/juju/apiserver/websocket/websocket.go:55
#       0x2d05bf0       github.com/juju/juju/apiserver/logsink.(*logSinkHandler).ServeHTTP+0xb0         /home/heather/work-test/src/github.com/juju/juju/apiserver/logsink/logsink.go:270
#       0x2d26c31       github.com/juju/juju/apiserver.(*Server).trackRequests.func1+0x111              /home/heather/work-test/src/github.com/juju/juju/apiserver/apiserver.go:1015
#       0x8a3cee        net/http.HandlerFunc.ServeHTTP+0x2e                                             /snap/go/9605/src/net/http/server.go:2084
#       0x2d0381c       github.com/juju/juju/apiserver/httpcontext.(*BasicAuthHandler).ServeHTTP+0x3fc  /home/heather/work-test/src/github.com/juju/juju/apiserver/httpcontext/auth.go:168
#       0x2d04145       github.com/juju/juju/apiserver/httpcontext.(*QueryModelHandler).ServeHTTP+0x325 /home/heather/work-test/src/github.com/juju/juju/apiserver/httpcontext/model.go:52
#       0x2d008b9       github.com/bmizerany/pat.(*PatternServeMux).ServeHTTP+0x199                     /home/heather/work-test/pkg/mod/github.com/bmizerany/pat@v0.0.0-20160217103242-c068ca2f0aac/mux.go:117
#       0x2d01fbd       github.com/juju/juju/apiserver/apiserverhttp.(*Mux).ServeHTTP+0x9d              /home/heather/work-test/src/github.com/juju/juju/apiserver/apiserverhttp/mux.go:67
#       0x8a779a        net/http.serverHandler.ServeHTTP+0x43a                                          /snap/go/9605/src/net/http/server.go:2916
#       0x8a2796        net/http.(*conn).serve+0x5d6                                                    /snap/go/9605/src/net/http/server.go:1966

1: 196608 [1: 196608] @ 0x2ffcec5 0x2ffce0e 0x2ffd07c 0x778495 0x468fc1
#       0x2ffcec4       github.com/juju/juju/core/logger.NewBufferedLogger+0x264                                        /home/heather/work-test/src/github.com/juju/juju/core/logger/buf.go:43
#       0x2ffce0d       github.com/juju/juju/internal/worker/modelworkermanager.newModelLogger+0x1ad                             /home/heather/work-test/src/github.com/juju/juju/internal/worker/modelworkermanager/recordlogger.go:28
#       0x2ffd07b       github.com/juju/juju/internal/worker/modelworkermanager.(*modelWorkerManager).starter.func1+0x41b        /home/heather/work-test/src/github.com/juju/juju/internal/worker/modelworkermanager/modelworkermanager.go:261
#       0x778494        github.com/juju/worker/v3.(*Runner).runWorker+0x2d4                                             /home/heather/work-test/pkg/mod/github.com/juju/worker/v3@v3.0.0-20220204100750-e23db69a42d2/runner.go:580

# and many more
```

## Interesting Output
The output of the heap profile can be difficult to read on its own. Using the pprof go tool can help.

To find a memory leak, compare 2 heap profiles:

```text
go tool pprof -http localhost:8100 -base juju_heap_profile-2022-06-11.00 jujud-2.9.29/jujud juju_heap_profile-2022-06-12.16 
```
Find jujud binaries in the [streams](https://streams.canonical.com/juju/tools/agent/)
