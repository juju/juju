#!/usr/bin/env bash

# This setup script is responsible for making sure a ubuntu system has the
# necessary tools and config in place for compiling dqlite for use in jujud.

		set -eu

		# Setup build env
		apt-get update
		apt-get -y -o Dpkg::Options::="--force-confdef" -o Dpkg::Options::="--force-confold" install \
			automake \
			libtool \
			make \
			gettext \
			autopoint \
			pkg-config \
			tclsh tcl \
			libsqlite3-dev

		snap install zig --classic --channel beta
