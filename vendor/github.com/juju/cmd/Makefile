# Copyright 2014 Canonical Ltd.
# Licensed under the LGPLv3, see LICENSE file for details.

default: check

check:
	go test && go test -compiler gccgo

docs:
	godoc2md github.com/juju/cmd > README.md

