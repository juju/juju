The **tomb** (`tomb`) package is a Go library that provides a type, `Tomb`, that has multiple methods that help manage
the goroutine(s) of a [worker](worker.md). The most important such methods are
[`Kill`](https://pkg.go.dev/gopkg.in/tomb.v2#Tomb.Kill) and [`Wait`](https://pkg.go.dev/gopkg.in/tomb.v2#Tomb.Wait).

> See more: [Go packages | `tomb`](https://pkg.go.dev/gopkg.in/tomb.v2)