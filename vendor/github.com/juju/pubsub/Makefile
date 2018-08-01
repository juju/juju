default: check

check:
	go test

docs:
	godoc2md github.com/juju/pubsub > README.md
	sed -i 's|\[godoc-link-here\]|[![GoDoc](https://godoc.org/github.com/juju/pubsub?status.svg)](https://godoc.org/github.com/juju/pubsub)|' README.md


.PHONY: default check docs
