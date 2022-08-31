// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

/*
Package kvm provides the facilities to deploy to kvm instances.

kvm implements the container interface for worker/provisioner to manage
applications deployed to kvm based 'containers' (see
juju/worker/provisioner/provisioner.go:containerProvisioner and
juju/container/container.go:container.Manager). The worker provisioner
specifics are in juju/worker/provisioner/kvm-broker.go and
juju/worker/container_initilisation.go.

The provisioner worker manages kvm containers through the interface provided in
this package, see: containerfactory.go, container.go, instance, and
initialization.go.  That is to say those files provide the container.Manager
interface while the rest of this package are the implementation of
container.Manager for kvm instances.

This package originally depended on the ubuntu uvtool apt package. This meant
that kvm would only work on ubuntu on amd64. The goal of removing Juju's
dependency on uvtool is to allow kvm to also work on arm64 and ppc64el.
However, it is still only expected to work on ubuntu.

When removing uvtool we (redir) performed a survey of the libvirt and qemu go
package landscape. There are a number of cgo based libraries and two possibly
promising ones once they are further developed:
github.com/digitalocean/go-libvirt and github.com/digitalocean/go-qemu. Those
packages are nascent and alpha at the time of this writing. They implement pure
go interfaces to libvirt and qemu by way of libvirt/qemu's custom RPC protocol.
While this would reduce the number of commands that require shelling out, it
wouldn't provide a way to create and manage disk images. So unless someone
implements qemu-utils and genisoimage in go, we'll always need to shell out for
those calls. The wrapped commands exist, shockingly, in wrappedcmds.go with
the exception of libvirt pool initialisation bits which are in
initialization.go.

After the provisioner initializes the kvm environment, we synchronise (fetch if
we don't have one) an ubuntu qcow image for the appropriate series and
architecture. This happens in sync.go and uses Juju's simplestreams
implementation in juju/environs/simplestreams and juju/environs/imagedownloads.
Once we fetch a compressed ubuntu image we then uncompress and convert it for
use into the libvirt storage pool. The storage pool is named 'juju-pool' and it
is located in $JUJU_DATADIR/kvm/guests, where JUJU_DATADIR is the value
returned by paths.DataDir. This ubuntu image is then used as a backing store
for our kvm instances for given series.

NB: Sharing a backing store across multiple instances allow us to save
significant disk space, but comes at a price too. The backing store is read
only to the volumes which use it and it cannot be updated. So we cannot easily
update common elements the way that lxd and snappy do with squashfs based
backing stores.This is to the best of my understanding, so corrections or
updates are welcome.

Once the backing store is ready, we create a system disk and a datasource disk.
The system disk is a sparse file with a maximum file size which uses the
aforementioned backing store as its base image. The data source disk is an iso
image with user-data and meta-data for cloud-init's NoCloud method to configure
our system. The cloud init data is written in kvm.go, via a call to machinery
in juju/cloudconfig/containerinit/container_userdata.go.  Destruction of a
container removes the system and data source disk files, but leaves the backing
store alone as it may be in use by other domains.

TBD: Put together something to send progress through a reader to the callback
function. We need to follow along with the method as implemented by LXD.
*/
package kvm
