ifndef GOPATH
	$(warning You need to set up a GOPATH.)
endif

PROJECT := gopkg.in/juju/jujusvg.v3
PROJECT_DIR := $(shell go list -e -f '{{.Dir}}' $(PROJECT))

help:
	@echo "Available targets:"
	@echo "  deps - fetch all dependencies"
	@echo "  build - build the project"
	@echo "  check - run tests"
	@echo "  install - install the library in your GOPATH"
	@echo "  clean - clean the project"

# Start of GOPATH-dependent targets. Some targets only make sense -
# and will only work - when this tree is found on the GOPATH.
ifeq ($(CURDIR),$(PROJECT_DIR))

deps:
	go get -v -t $(PROJECT)/...

build:
	go build $(PROJECT)/...

check:
	go test $(PROJECT)/...

install:
	go install $(INSTALL_FLAGS) -v $(PROJECT)/...

clean:
	go clean $(PROJECT)/...

else

deps:
	$(error Cannot $@; $(CURDIR) is not on GOPATH)
	
build:
	$(error Cannot $@; $(CURDIR) is not on GOPATH)

check:
	$(error Cannot $@; $(CURDIR) is not on GOPATH)

install:
	$(error Cannot $@; $(CURDIR) is not on GOPATH)

clean:
	$(error Cannot $@; $(CURDIR) is not on GOPATH)

endif
# End of GOPATH-dependent targets.

.PHONY: help deps build check install clean
