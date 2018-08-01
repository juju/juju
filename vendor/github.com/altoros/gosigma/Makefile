#
# Makefile for gosigma
#

PROJECT := github.com/altoros/gosigma
PROJECT_DIR := $(shell go list -e -f '{{.Dir}}' $(PROJECT))

ifeq ($(shell uname -p | sed -r 's/.*(x86|armel|armhf).*/golang/'), golang)
	GO_C := golang
	INSTALL_FLAGS := 
else
	GO_C := gccgo-4.9  gccgo-go
	INSTALL_FLAGS := -gccgoflags=-static-libgo
endif

default: build

# Start of GOPATH-dependent targets. Some targets only make sense -
# and will only work - when this tree is found on the GOPATH.
ifeq ($(CURDIR),$(PROJECT_DIR))

build:
	go build $(PROJECT)/...

check test: check-license
	go test $(PROJECT)/...

check-license:
	@(fgrep "Copyright 2014 ALTOROS" -rl | grep -v Makefile ; \
	 find -name "*.go" | cut -b3-) | sort | uniq -u | xargs -I {} echo FAIL: license missed: {}

install:
	go install $(INSTALL_FLAGS) -v $(PROJECT)/...

clean:
	go clean $(PROJECT)/...
	find -name "*.test" | xargs rm -f
	find -name "*.out" | xargs rm -f

coverage.out: *.go data/*.go https/*.go
	-rm -rf *cover.out
	go test -coverprofile=data.cover.out -coverpkg=./,./data,./https ./data
	go test -coverprofile=https.cover.out -coverpkg=./,./data,./https ./https
	go test -coverprofile=gosigma.cover.out -coverpkg=./,./data,./https ./
	echo "mode: set" > coverage.out && cat *.cover.out | grep -v mode: | sort -r | \
		awk '{if($$1 != last) {print $$0;last=$$1}}' >> coverage.out
	rm data.cover.out
	rm https.cover.out
	rm gosigma.cover.out

cover-html: coverage.out
	go tool cover -html=$<

cover: coverage.out
	go tool cover -func=$<

update:
	go get -u -v ./...

else # --------------------------------

build check test install clean:
	$(error Cannot $@; $(CURDIR) is not on GOPATH)

endif
# End of GOPATH-dependent targets.

# Reformat source files.
format:
	gofmt -w -l .

lc:
	find -name "*.go" | xargs cat | wc -l

.PHONY: build check test check-license install clean cover-html cover update
.PHONY: format


