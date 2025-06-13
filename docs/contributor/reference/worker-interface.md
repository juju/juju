(worker-interface)=
# Worker interface
In Juju, the **`worker` interface** is a Go interface in {ref}`the Go worker package <worker-package>` that defines the
`Kill()` and
`Wait()` methods implemented by any Juju {ref}`worker <worker>`.


> See more: [Go packages | `worker` > `worker.go`](https://github.com/juju/worker/blob/HEAD/worker.go)