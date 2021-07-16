PROJECT_DIR := $(shell dirname $(realpath $(firstword $(MAKEFILE_LIST))))

GOOS=$(shell go env GOOS)
GOARCH=$(shell go env GOARCH)

BUILD_DIR ?= $(PROJECT_DIR)/_build
BIN_DIR = ${BUILD_DIR}/${GOOS}_${GOARCH}/bin

build:
	@go build -o ${BIN_DIR}/juju-inspect

install:
	@go install 