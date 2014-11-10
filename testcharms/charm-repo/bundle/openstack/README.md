OpenStack Bundle for Juju
=========================

Overview
--------

This bundle deploys a reference OpenStack architecture including all core projects:

  - OpenStack Compute
  - OpenStack Networking (using Open vSwitch plugin)
  - OpenStack Block Storage (backed with Ceph storage)
  - OpenStack Image
  - OpenStack Object Storage
  - OpenStack Identity
  - OpenStack Dashboard
  - OpenStack Telemetry
  - OpenStack Orchestration

The charm configuration is an opinioned set for deploying OpenStack for testing on Cloud environments which support nested KVM.  Instance types also need to have ephemeral storage (these block devices are used for Ceph and Swift storage).

The Ubuntu Server Team use this bundle for testing OpenStack-on-OpenStack.

Usage
-----

Once deployed, the cloud can be accessed either using the OpenStack command line tools or using the OpenStack Dashboard:

    http://<IP of openstack-dashboard server>/horizon

The charms configure the 'admin' user with a password of 'openstack' by default.

The OpenStack cloud deployed is completely clean; the charms don't attempt to configure networking or upload images.  Read the OpenStack User Guide on how to configure your cloud for use:

    http://docs.openstack.org/user-guide/content/

Niggles
-------

The neutron-gateway service requires a service unit with two network interfaces to provide full functionality; this part of OpenStack provides L3 routing between tenant networks and the rest of the world.  Its possible todo this when testing on OpenStack by adding a second network interface to the neutron-gateway service:

    nova interface-attach --net-id <UUID for network>  <UUID of instance>
    juju set neutron-gateway ext-port=eth1

Note that you will need to be running this bundle on an OpenStack cloud that supports MAC address learning of some description; this includes using OpenStack Havana with the Neutron Open vSwitch plugin.

For actual OpenStack deployments, this service would reside of a physical server with network ports attached to both the internal network (for communication with nova-compute service units) and the external network (for inbound/outbound network access to/from instances within the cloud).
