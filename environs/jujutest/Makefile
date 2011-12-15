include $(GOROOT)/src/Make.inc

all: package

TARG=launchpad.net/juju/go/environ/jujutest

GOFILES=\
	test.go\
	tests.go\
	livetests.go\

GOFMT=gofmt
BADFMT:=$(shell $(GOFMT) -l $(GOFILES) $(CGOFILES) $(wildcard *_test.go))

gofmt: $(BADFMT)
	@for F in $(BADFMT); do $(GOFMT) -w $$F && echo $$F; done

ifneq ($(BADFMT),)
ifneq ($(MAKECMDGOALS),gofmt)
$(warning WARNING: make gofmt: $(BADFMT))
endif
endif

include $(GOROOT)/src/Make.pkg

