include $(GOROOT)/src/Make.inc

all: package

TARG=launchpad.net/juju/go/environ/ec2

GOFILES=\
	config.go\
	ec2.go\
	image.go\
	util.go\

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

# regenerate doesn't remove images that have disappeared
# but that's probably not a problem.
regenerate:
	gotest -regenerate-images
	bzr add images
