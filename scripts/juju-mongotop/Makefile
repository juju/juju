PROJECT_DIR := $(shell dirname $(realpath $(firstword $(MAKEFILE_LIST))))

GOOS=$(shell go env GOOS)
GOARCH=$(shell go env GOARCH)

BUILD_DIR ?= $(abspath $(PROJECT_DIR)/../../_build)
BIN_DIR = ${BUILD_DIR}/${GOOS}_${GOARCH}/bin

build:
	echo ${BUILD_DIR}
	@go build -o ${BIN_DIR}/juju-mongotop

install:
	@go install