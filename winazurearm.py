#!/usr/bin/python

from __future__ import print_function

from argparse import ArgumentParser
from datetime import (
    datetime,
    timedelta,
)
import fnmatch
import logging
import os
import sys

import pytz

__metaclass__ = type


AZURE_SUBSCRIPTION_ID = "AZURE_SUBSCRIPTION_ID"
AZURE_CLIENT_ID = "AZURE_CLIENT_ID"
AZURE_SECRET = "AZURE_SECRET"
AZURE_TENANT = "AZURE_TENANT"

DEFAULT_RESOURCE_PREFIX = 'Default-'
OLD_MACHINE_AGE = 6


log = logging.getLogger("winazurearm")


class ARMClient:
    """A collection of Azure RM clients."""

    def __init__(self, subscription_id, client_id, secret, tenant,
                 verbose=False, dry_run=False):
        self.subscription_id = subscription_id
        self.client_id = client_id
        self.secret = secret
        self.tenant = tenant
        self.verbose = verbose
        self.dry_run = dry_run
        self.credentials = None
        self.storage = None
        self.resource = None
        self.compute = None
        self.network = None

    def __eq__(self, other):
        return (
            self.subscription_id == other.subscription_id and
            self.client_id == other.client_id and
            self.secret == other.secret and
            self.tenant == other.tenant and
            self.verbose == other.verbose and
            self.dry_run == other.dry_run)

    def init_services(self):
        """Delay imports and activation of Azure RM services until needed."""
        from azure.common.credentials import ServicePrincipalCredentials
        from azure.mgmt.resource.resources import (
            ResourceManagementClient,
            ResourceManagementClientConfiguration,
        )
        from azure.mgmt.storage import (
            StorageManagementClient,
            StorageManagementClientConfiguration,
        )
        from azure.mgmt.compute import (
            ComputeManagementClient,
            ComputeManagementClientConfiguration,
        )
        from azure.mgmt.network import (
            NetworkManagementClient,
            NetworkManagementClientConfiguration,
        )
        self.credentials = ServicePrincipalCredentials(
            client_id=self.client_id, secret=self.secret, tenant=self.tenant)
        self.storage = StorageManagementClient(
            StorageManagementClientConfiguration(
                self.credentials, self.subscription_id))
        self.resource = ResourceManagementClient(
            ResourceManagementClientConfiguration(
                self.credentials, self.subscription_id))
        self.compute = ComputeManagementClient(
            ComputeManagementClientConfiguration(
                self.credentials, self.subscription_id))
        self.network = NetworkManagementClient(
            NetworkManagementClientConfiguration(
                self.credentials, self.subscription_id))


class ResourceGroupDetails:
    def __init__(self, client, group, storage=None, vms=None,
                 addresses=None, networks=None):
        self.client = client
        self.is_loaded = False
        self.group = group
        self.storage = storage
        self.vms = vms
        self.addresses = addresses
        self.networks = networks

    @property
    def name(self):
        return self.group.name

    def load_details(self):
        self.storage = list(
            self.client.storage.storage_accounts.list_by_resource_group(
                self.name))
        self.vms = list(
            self.client.compute.virtual_machines.list(self.name))
        self.addresses = list(
            self.client.network.public_ip_addresses.list(self.name))
        self.networks = list(
            self.client.network.virtual_networks.list(self.name))
        self.is_loaded = True

    def print_out(self, recursive=False):
        print(self.name)
        if recursive:
            for vm in self.vms:
                print('    VM {} {}'.format(vm.name, vm.vm_id))
            for address in self.addresses:
                print('    {} {}'.format(address.name, address.ip_address))
            for network in self.networks:
                print('    Network {}'.format(network.name))
            for storage in self.storage:
                print('    Storage {} {}'.format(
                    storage.name, storage.creation_time))

    def is_old(self, now, old_age):
        """Return True if the resource group is old.

        There are ambiguous cases where a resource groups is not completely
        deleted. We know from logs that networks are are often that last
        resource to be deleted and may be delete quick enough for the test.
        A resource group with just a network is considered to be old.
        """
        if old_age == 0:
            # In the case of O hours old, the caller is stating any resource
            # group that exists is old.
            return True
        ago = timedelta(hours=old_age)
        # Contrary to Juju claims, the Azure RM provider always creates storage
        # and that one resource has a creation_time.
        if self.storage:
            # Azure allows many storage accounts per resource group, but Juju
            # only creates one.
            creation_time = self.storage[0].creation_time
            age = now - creation_time
            if age > ago:
                hours_old = (age.seconds / 3600) + (age.days * 24)
                if self.client.verbose:
                    print('{} is {} hours old:'.format(self.name, hours_old))
                    print('  {}'.format(creation_time))
                return True
        elif (self.networks and not
                all([self.vms, self.addresses, self.storage])):
            # There is a network, but no vms, storage or public addresses.
            if self.client.verbose:
                print('{} only has a network, likely a failed delete'.format(
                      self.name))
            return True
        return False

    def delete(self):
        self.client.resource.resource_groups.delete(self.name)


def list_resources(client, glob='*', recursive=False, print_out=False):
    """Return a list of ResourceGroupDetails.

    Use print_out=True to print a listing of resources.
    """
    groups = []
    resource_groups = client.resource.resource_groups.list()
    for group in resource_groups:
        if group.name.startswith(DEFAULT_RESOURCE_PREFIX):
            # This is not a resource group. Use the UI to delete Default
            # resources.
            continue
        if not fnmatch.fnmatch(group.name, glob):
            continue
        rgd = ResourceGroupDetails(client, group)
        if recursive:
            rgd.load_details()
        if print_out:
            rgd.print_out(recursive=recursive)
        groups.append(rgd)
    return groups


def delete_resources(client, glob='*', old_age=OLD_MACHINE_AGE):
    """Delete old resource groups."""
    now = datetime.now(pytz.utc)
    resources = list_resources(client, glob=glob, recursive=True)
    pollers = []
    for rgd in resources:
        name = rgd.name
        if not rgd.is_old(now, old_age):
            continue
        if client.verbose:
            print('Deleting {}'.format(name))
        if not client.dry_run:
            poller = rgd.delete()
            pollers.append((name, poller))
    for name, poller in pollers:
        if client.verbose:
            print('Waiting for {} to be deleted'.format(name))
        if poller:
            # Deleting a group created using the old might not return
            # a poller!. We just hope the async operation is successful.
            # A poller is returned in most cases, and it is is not done,
            # we an ask for results to wait for done.
            if not poller.done():
                poller.result()


def find_vm_instance(resources, name_id, resource_group):
    """Return a tuple of ResourceGroupDetauls and VirtualMachine."""
    for rgd in resources:
        for vm in rgd.vms:
            if resource_group and vm.name == name_id or vm.vm_id == name_id:
                return rgd, vm
    return None, None


def delete_instance(client, name_id, resource_group=None):
    """Delete a VM instance.

    When resource_group is provides, VM name is used to locate the VM.
    Otherwise, all resource groups are searched for a matching VM id.

    :param name_id: The name or id of a VM instance.
    :param resource_group: The name of the resource group the VM belongs to.
    """
    if resource_group:
        glob = resource_group
    else:
        glob = '*'
    resources = list_resources(client, glob=glob, recursive=True)
    rgd, vm = find_vm_instance(resources, name_id, resource_group)
    if vm:
        if client.verbose:
            print('Found {} {}'.format(rgd.name, vm.name))
        if not client.dry_run:
            poller = client.compute.virtual_machines.delete(
                rgd.name, vm.name)
            if client.verbose:
                print('Waiting for {} to be deleted'.format(vm.name))
            if not poller.done():
                poller.result()
    else:
        group_names = ', '.join([rgd.name for rdg in resources])
        print('The vm name {} was not found in {}'.format(
              name_id, group_names))


def parse_args(args=None):
    """Return the argument parser for this program."""
    parser = ArgumentParser('Query and manage azure.')
    parser.add_argument(
        '-d', '--dry-run', action='store_true', default=False,
        help='Do not make changes.')
    parser.add_argument(
        '-v', '--verbose', action="store_true", help='Increse verbosity.')
    parser.add_argument(
        '--subscription-id',
        help=("The subscription id to make requests with. "
              "Environment: $AZURE_SUBSCRIPTION_ID."),
        default=os.environ.get(AZURE_SUBSCRIPTION_ID))
    parser.add_argument(
        '--client-id',
        help=("The client id to make requests with. "
              "Environment: $AZURE_CLIENT_ID."),
        default=os.environ.get(AZURE_CLIENT_ID))
    parser.add_argument(
        '--secret',
        help=("The secret to make requests with. "
              "Environment: $AZURE_SECRET."),
        default=os.environ.get(AZURE_SECRET))
    parser.add_argument(
        '--tenant',
        help=("The tenant to make requests with. "
              "Environment: $AZURE_TENANT."),
        default=os.environ.get(AZURE_TENANT))
    subparsers = parser.add_subparsers(help='sub-command help', dest="command")
    ls_parser = subparsers.add_parser(
        'list-resources', help='List resource groups.')
    ls_parser.add_argument(
        '-r', '--recursive', default=False, action='store_true',
        help='Show resources with a resources group.')
    ls_parser.add_argument(
        'filter', default='*', nargs='?',
        help='A glob pattern to match services to.')
    dr_parser = subparsers.add_parser(
        'delete-resources',
        help='delete resource groups and their vm, networks, etc.')
    dr_parser.add_argument(
        '-o', '--old-age', default=OLD_MACHINE_AGE, type=int,
        help='Set old machine age to n hours.')
    dr_parser.add_argument(
        'filter', help='An exact name or glob pattern to match services to.')
    di_parser = subparsers.add_parser('delete-instance', help='Delete a vm.')
    di_parser.add_argument(
        'name_id', help='The name or id of an instance (name needs group).')
    di_parser.add_argument(
        'resource_group', default=None, nargs='?',
        help='The resource-group name of the machine name.')
    args = parser.parse_args(args)
    if not all(
            [args.subscription_id, args.client_id, args.secret, args.tenant]):
        print("$AZURE_SUBSCRIPTION_ID, $AZURE_CLIENT_ID, $AZURE_SECRET, "
              "$AZURE_TENANT was not provided.")
    return args


def main(argv):
    args = parse_args(argv)
    client = ARMClient(
        args.subscription_id, args.client_id, args.secret, args.tenant,
        verbose=args.verbose, dry_run=args.dry_run)
    client.init_services()
    if args.command == 'list-resources':
        list_resources(
            client, glob=args.filter, recursive=args.recursive, print_out=True)
    elif args.command == 'delete-resources':
        delete_resources(client, glob=args.filter, old_age=args.old_age)
    elif args.command == 'delete-instance':
        delete_instance(
            client, args.name_id, resource_group=args.resource_group)
    return 0


if __name__ == '__main__':
    sys.exit(main(sys.argv[1:]))
