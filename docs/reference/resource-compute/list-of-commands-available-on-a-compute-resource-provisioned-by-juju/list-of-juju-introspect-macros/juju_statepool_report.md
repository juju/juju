(juju_statepool_report)=
# `juju_statepool_report`

The statepool report shows the internal details of the pool of mongo sessions
used to provide mongo connectivity for each model. This primarily a diagnostic
tool for developers. As such, enabling the `developer-mode` feature flag results
in the addition of a stack trace for each pooled state showing the orign of
that state's acquisition.

## Usage
Must be run on a juju controller machine.
```code
juju_statepool_report
```

## Example output
```text
$ juju_statepool_report 
State Pool Report:

Model count: 1 models
Marked for removal: 0 models


Model: 73e18e39-035a-4ceb-8cd6-dbba16fd9c16
  Marked for removal: false
  Reference count: 3
    [1]
goroutine 857 [running]:
runtime/debug.Stack()
        /home/ian/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.24.4.linux-amd64/src/runtime/debug/stack.go:26 +0x5e
github.com/juju/juju/state.(*StatePool).Get(0xc000745770, {0xc00149aa0d, 0x24})
        /home/ian/juju/go/src/juju/juju/state/pool.go:258 +0x225
github.com/juju/juju/apiserver.(*Server).serveConn(0xc0008d66c8, {0x89fe3e8, 0xc000a91cb0}, 0xc000e775f8?, {0xc00149aa0d, 0x24}, 0x2, {0x8a000a8, 0xc0014a62e8}, {0xc0006066e0, ...})
        /home/ian/juju/go/src/juju/juju/apiserver/apiserver.go:1125 +0x2bb
github.com/juju/juju/apiserver.(*Server).apiHandler.func1(0xc0001a0a80)
        /home/ian/juju/go/src/juju/juju/apiserver/apiserver.go:1081 +0x14a
github.com/juju/juju/apiserver/websocket.Serve({0x89e72f0?, 0xc000000000?}, 0x2?, 0xc000e776d8)
        /home/ian/juju/go/src/juju/juju/apiserver/websocket/websocket.go:55 +0xd2
github.com/juju/juju/apiserver.(*Server).apiHandler(0xc0008d66c8, {0x89e72f0, 0xc000000000}, 0xc00043e3c0)
        /home/ian/juju/go/src/juju/juju/apiserver/apiserver.go:1078 +0x11c
net/http.HandlerFunc.ServeHTTP(0x24?, {0x89e72f0?, 0xc000000000?}, 0xc000e77808?)
        /home/ian/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.24.4.linux-amd64/src/net/http/server.go:2294 +0x29
github.com/juju/juju/apiserver.(*Server).endpoints.(*Server).monitoredHandler.func6({0x89e72f0, 0xc000000000}, 0xc00043e3c0)
        /home/ian/juju/go/src/juju/juju/apiserver/apiserver.go:1183 +0x12e
net/http.HandlerFunc.ServeHTTP(0xc0008d66c8?, {0x89e72f0?, 0xc000000000?}, 0x7c059a0?)
        /home/ian/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.24.4.linux-amd64/src/net/http/server.go:2294 +0x29
github.com/juju/juju/apiserver.(*Server).endpoints.(*Server).endpoints.func1.(*Server).trackRequests.func29({0x89e72f0, 0xc000000000}, 0xc00043e3c0)
        /home/ian/juju/go/src/juju/juju/apiserver/apiserver.go:1055 +0xf2
net/http.HandlerFunc.ServeHTTP(0x89fe420?, {0x89e72f0?, 0xc000000000?}, 0xd05af00?)
        /home/ian/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.24.4.linux-amd64/src/net/http/server.go:2294 +0x29
github.com/juju/juju/apiserver/httpcontext.validateModelAndServe({0x8980b80?, 0xc0003b1d80?}, {0xc00149aa0d?, 0x24?}, {0x89e72f0?, 0xc000000000?}, 0xc001396b40?)
        /home/ian/juju/go/src/juju/juju/apiserver/httpcontext/model.go:72 +0x208
github.com/juju/juju/apiserver/httpcontext.(*QueryModelHandler).ServeHTTP(0xc0003b1ea0, {0x89e72f0, 0xc000000000}, 0xc001396b40)
        /home/ian/juju/go/src/juju/juju/apiserver/httpcontext/model.go:41 +0x69
github.com/bmizerany/pat.(*PatternServeMux).ServeHTTP(0xc000a8cae0, {0x89e72f0, 0xc000000000}, 0xc001396b40)
        /home/ian/go/pkg/mod/github.com/bmizerany/pat@v0.0.0-20210406213842-e4b6760bdd6f/mux.go:117 +0x17f
github.com/juju/juju/apiserver/apiserverhttp.(*Mux).ServeHTTP(0x476bd9?, {0x89e72f0, 0xc000000000}, 0xc001396b40)
        /home/ian/juju/go/src/juju/juju/apiserver/apiserverhttp/mux.go:67 +0x97
net/http.serverHandler.ServeHTTP({0xc0014a3590?}, {0x89e72f0?, 0xc000000000?}, 0x1?)
        /home/ian/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.24.4.linux-amd64/src/net/http/server.go:3301 +0x8e
net/http.(*conn).serve(0xc0008d3e60, {0x89fe3e8, 0xc000bca120})
        /home/ian/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.24.4.linux-amd64/src/net/http/server.go:2102 +0x625
created by net/http.(*Server).Serve in goroutine 539
        /home/ian/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.24.4.linux-amd64/src/net/http/server.go:3454 +0x485

    [2]
goroutine 310480 [running]:
runtime/debug.Stack()
        /home/ian/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.24.4.linux-amd64/src/runtime/debug/stack.go:26 +0x5e
github.com/juju/juju/state.(*StatePool).Get(0xc000745770, {0xc00437380d, 0x24})
        /home/ian/juju/go/src/juju/juju/state/pool.go:258 +0x225
github.com/juju/juju/apiserver.(*Server).serveConn(0xc0008d66c8, {0x89fe3e8, 0xc0032f0d80}, 0xc001c895f8?, {0xc00437380d, 0x24}, 0x2e, {0x8a000a8, 0xc004aa26d8}, {0xc003086510, ...})
        /home/ian/juju/go/src/juju/juju/apiserver/apiserver.go:1125 +0x2bb
github.com/juju/juju/apiserver.(*Server).apiHandler.func1(0xc00150c1a8)
        /home/ian/juju/go/src/juju/juju/apiserver/apiserver.go:1081 +0x14a
github.com/juju/juju/apiserver/websocket.Serve({0x89e72f0?, 0xc004c181c0?}, 0x2e?, 0xc001c896d8)
        /home/ian/juju/go/src/juju/juju/apiserver/websocket/websocket.go:55 +0xd2
github.com/juju/juju/apiserver.(*Server).apiHandler(0xc0008d66c8, {0x89e72f0, 0xc004c181c0}, 0xc004c24c80)
        /home/ian/juju/go/src/juju/juju/apiserver/apiserver.go:1078 +0x11c
net/http.HandlerFunc.ServeHTTP(0xc001c897b0?, {0x89e72f0?, 0xc004c181c0?}, 0xc001c89808?)
        /home/ian/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.24.4.linux-amd64/src/net/http/server.go:2294 +0x29
github.com/juju/juju/apiserver.(*Server).endpoints.(*Server).monitoredHandler.func6({0x89e72f0, 0xc004c181c0}, 0xc004c24c80)
        /home/ian/juju/go/src/juju/juju/apiserver/apiserver.go:1183 +0x12e
net/http.HandlerFunc.ServeHTTP(0xc0008d66c8?, {0x89e72f0?, 0xc004c181c0?}, 0x73ae5cbe2f30?)
        /home/ian/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.24.4.linux-amd64/src/net/http/server.go:2294 +0x29
github.com/juju/juju/apiserver.(*Server).endpoints.(*Server).endpoints.func1.(*Server).trackRequests.func29({0x89e72f0, 0xc004c181c0}, 0xc004c24c80)
        /home/ian/juju/go/src/juju/juju/apiserver/apiserver.go:1055 +0xf2
net/http.HandlerFunc.ServeHTTP(0x89fe420?, {0x89e72f0?, 0xc004c181c0?}, 0xd05af00?)
        /home/ian/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.24.4.linux-amd64/src/net/http/server.go:2294 +0x29
github.com/juju/juju/apiserver/httpcontext.validateModelAndServe({0x8980b80?, 0xc0003b1d80?}, {0xc00437380d?, 0x24?}, {0x89e72f0?, 0xc004c181c0?}, 0xc004c24b40?)
        /home/ian/juju/go/src/juju/juju/apiserver/httpcontext/model.go:72 +0x208
github.com/juju/juju/apiserver/httpcontext.(*QueryModelHandler).ServeHTTP(0xc0003b1ea0, {0x89e72f0, 0xc004c181c0}, 0xc004c24b40)
        /home/ian/juju/go/src/juju/juju/apiserver/httpcontext/model.go:41 +0x69
github.com/bmizerany/pat.(*PatternServeMux).ServeHTTP(0xc000a8cae0, {0x89e72f0, 0xc004c181c0}, 0xc004c24b40)
        /home/ian/go/pkg/mod/github.com/bmizerany/pat@v0.0.0-20210406213842-e4b6760bdd6f/mux.go:117 +0x17f
github.com/juju/juju/apiserver/apiserverhttp.(*Mux).ServeHTTP(0x476bd9?, {0x89e72f0, 0xc004c181c0}, 0xc004c24b40)
        /home/ian/juju/go/src/juju/juju/apiserver/apiserverhttp/mux.go:67 +0x97
net/http.serverHandler.ServeHTTP({0xc0032f0990?}, {0x89e72f0?, 0xc004c181c0?}, 0x1?)
        /home/ian/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.24.4.linux-amd64/src/net/http/server.go:3301 +0x8e
net/http.(*conn).serve(0xc0039faab0, {0x89fe3e8, 0xc000bca120})
        /home/ian/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.24.4.linux-amd64/src/net/http/server.go:2102 +0x625
created by net/http.(*Server).Serve in goroutine 539
        /home/ian/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.24.4.linux-amd64/src/net/http/server.go:3454 +0x485

    [3]
goroutine 507 [running]:
runtime/debug.Stack()
        /home/ian/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.24.4.linux-amd64/src/runtime/debug/stack.go:26 +0x5e
github.com/juju/juju/state.(*StatePool).Get(0xc000745770, {0xc000ad96b0, 0x24})
        /home/ian/juju/go/src/juju/juju/state/pool.go:258 +0x225
github.com/juju/juju/internal/worker/state.(*modelStateWorker).loop(0xc0003ce700)
        /home/ian/juju/go/src/juju/juju/internal/worker/state/worker.go:158 +0x4b
gopkg.in/tomb%2ev2.(*Tomb).run(0xc0003ce700, 0xc000ea83c0?)
        /home/ian/go/pkg/mod/gopkg.in/tomb.v2@v2.0.0-20161208151619-d5d1b5820637/tomb.go:163 +0x2b
created by gopkg.in/tomb%2ev2.(*Tomb).Go in goroutine 452
        /home/ian/go/pkg/mod/gopkg.in/tomb.v2@v2.0.0-20161208151619-d5d1b5820637/tomb.go:159 +0xdb
```
