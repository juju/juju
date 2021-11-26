SHELL = bash
.ONESHELL:

BOX=build-server
BOX_IMAGE=ubuntu:18.04
JUJU_CONTROLLER_BOX ?= $(shell juju status -m controller | grep juju- | head -1 | awk '{print $$4}')

.PHONY: bootstrap install-deps-on-controller build-and-install-jujud

bootstrap:
	@echo "Starting ${BOX_IMAGE} and mounting ${GOPATH}/src/github.com/juju/juju (host) -> /var/src/juju (guest)"
	@lxc launch ubuntu:18.04 ${BOX}
	@lxc config device add ${BOX} jujusrc disk source=${GOPATH}/src/github.com/juju/juju path=/var/src/juju

	@echo "Waiting for ${BOX} to become ready (get IP etc.)"
	@lxc exec ${BOX} -- bash -c 'while [ "$$(systemctl is-system-running 2>/dev/null)" != "running" ] && [ "$$(systemctl is-system-running 2>/dev/null)" != "degraded" ]; do :; done'

	@echo "Preparing provisioning script: /tmp/setup-build-farm.sh"
	@cat <<- 'EOF' >/tmp/setup-build-farm.sh
		#!/bin/bash

		# Setup build env
		apt-get -y -o Dpkg::Options::="--force-confdef" -o Dpkg::Options::="--force-confold" install \
		gcc automake libtool make liblz4-dev libuv1-dev pkg-config libsqlite3-dev

		# Setup Go
		cd /tmp
		wget https://go.dev/dl/go1.14.15.linux-amd64.tar.gz
		tar xf go1.14.15.linux-amd64.tar.gz
		rm -f go1.14.15.linux-amd64.tar.gz
		mv go /usr/local

		# Setup Go
		cd /root
		mkdir go
		echo >> /root/.bashrc
		echo 'export GOPATH=/root/go' >> /root/.bashrc
		echo 'export PATH=$${PATH}:/usr/local/go/bin:$${GOPATH}/bin' >> /root/.bashrc
		source /root/.bashrc
		env | grep GO

		# Setup juju sources
		mkdir -p /root/go/src/github.com/juju
		ln -s /var/src/juju /root/go/src/github.com/juju/juju

		# Install helper for building juju
		echo 'export GOPATH=/root/go; export PATH=$${PATH}:/usr/local/go/bin:$${GOPATH}/bin; cd $${GOPATH}/src/github.com/juju/juju; BUILD_DIR=/tmp/jujubuild make go-build' > /root/build-juju.sh
		chmod +x /root/build-juju.sh

		# Install dqlite deps for builds
		export GOPATH=/root/go
		cd $${GOPATH}/src/github.com/juju/juju
		make deps
	EOF

	@echo "Pushing provisioning script to ${BOX}"
	@lxc file push /tmp/setup-build-farm.sh ${BOX}/root/setup-build-farm.sh

	@echo "Executing provisioning script"
	@lxc exec -t ${BOX} bash /root/setup-build-farm.sh

	@echo "--------------------------------------------------"
	@echo "To install the required dqlite deps on the current controller:"
	@echo "  make install-deps-on-controller"
	@echo
	@echo "Or, to build juju with dqlite and push the binary to the controller:"
	@echo "  make build-and-install-jujud"
	@echo

install-deps-on-controller:
	@echo "[+] Installing dqlite dependencies to controller: ${JUJU_CONTROLLER_BOX}"
	lxc exec -t ${JUJU_CONTROLLER_BOX} -- add-apt-repository -y ppa:dqlite/dev
	lxc exec -t ${JUJU_CONTROLLER_BOX} -- apt-get update
	lxc exec -t ${JUJU_CONTROLLER_BOX} -- apt-get -y -o Dpkg::Options::="--force-confdef" -o Dpkg::Options::="--force-confold" install libuv1-dev libsqlite3-dev libdqlite-dev rlwrap socat

stop-jujud:
	@echo
	@echo "[+] Stopping jujud-machine service on controller: ${JUJU_CONTROLLER_BOX}"
	@lxc exec ${JUJU_CONTROLLER_BOX} -- bash -c 'systemctl stop $$(ls /etc/systemd/system/jujud-mach*.service | cut -d"/" -f5)'

build-and-install-jujud:
	@echo "[+] Building jujud on ${BOX}"
	@lxc exec -t ${BOX} -- bash /root/build-juju.sh

	@echo
	@echo "[+] Stopping jujud-machine service on controller: ${JUJU_CONTROLLER_BOX}"
	@lxc exec ${JUJU_CONTROLLER_BOX} -- bash -c 'systemctl stop $$(ls /etc/systemd/system/jujud-mach*.service | cut -d"/" -f5)'

	@echo "[+] Pushing jujud to controller: ${JUJU_CONTROLLER_BOX}"
	@lxc file pull ${BOX}/tmp/jujubuild/linux_amd64/bin/jujud /tmp/jujud
	@lxc exec ${JUJU_CONTROLLER_BOX} -- bash -c 'rm -rf /var/lib/juju/tools/$$(ls /var/lib/juju/tools | grep machine-)/jujud'
	lxc file push /tmp/jujud ${JUJU_CONTROLLER_BOX}/var/lib/juju/tools/`lxc exec ${JUJU_CONTROLLER_BOX} -- bash -c 'ls /var/lib/juju/tools | grep machine-'`/jujud

	@echo "[+] Starting jujud-machine service on controller: ${JUJU_CONTROLLER_BOX}"
	@lxc exec ${JUJU_CONTROLLER_BOX} -- bash -c 'systemctl start $$(ls /etc/systemd/system/jujud-mach*.service | cut -d"/" -f5)'

dqlite-repl:
	@echo "[+] Connecting to REPL interface on controller: ${JUJU_CONTROLLER_BOX}"
	@lxc exec -t ${JUJU_CONTROLLER_BOX} -- rlwrap -H /root/.dqlite_repl.history socat - /var/lib/juju/dqlite/juju.sock

