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

DEFAULT_RESOURCE_PREFIX = 'default-'
JUJU_MACHINE_PREFIX = 'machine-'
OLD_MACHINE_AGE = 6


# The azure lib is very chatty even at the info level. This logger
# strictly reports the activity of this script.
log = logging.getLogger("winazurearm")
handler = logging.StreamHandler(sys.stderr)
handler.setFormatter(logging.Formatter(
    fmt='%(asctime)s %(levelname)s %(message)s',
    datefmt='%Y-%m-%d %H:%M:%S'))
log.addHandler(handler)


class ARMClient:
    """A collection of Azure RM clients."""

    def __init__(self, subscription_id, client_id, secret, tenant,
                 read_only=False):
        self.subscription_id = subscription_id
        self.client_id = client_id
        self.secret = secret
        self.tenant = tenant
        self.read_only = read_only
        self.credentials = None
        self.storage = None
        self.resource = None
        self.compute = None
        self.network = None

    def __eq__(self, other):
        # Testing is the common case for checking equality.
        return (
            type(other) == type(self) and
            self.subscription_id == other.subscription_id and
            self.client_id == other.client_id and
            self.secret == other.secret and
            self.tenant == other.tenant and
            self.read_only == other.read_only)

    def init_services(self):
        """Delay imports and activation of Azure RM services until needed."""
        from azure.common.credentials import ServicePrincipalCredentials
        from azure.mgmt.resource.resources import ResourceManagementClient
        from azure.mgmt.storage import StorageManagementClient
        from azure.mgmt.compute import ComputeManagementClient
        from azure.mgmt.network import NetworkManagementClient
        self.credentials = ServicePrincipalCredentials(
            client_id=self.client_id, secret=self.secret, tenant=self.tenant)
        self.storage = StorageManagementClient(
            self.credentials, self.subscription_id)
        self.resource = ResourceManagementClient(
            self.credentials, self.subscription_id)
        self.compute = ComputeManagementClient(
            self.credentials, self.subscription_id)
        self.network = NetworkManagementClient(
            self.credentials, self.subscription_id)


class ResourceGroupDetails:

    def __init__(self, client, group, deployments=None):
        self.client = client
        self.is_loaded = False
        self.group = group
        self.deployments = deployments

    def __eq__(self, other):
        # Testing is the common case for checking equality.
        return (
            type(other) == type(self) and
            self.client == other.client and
            self.is_loaded is other.is_loaded and
            self.group is other.group and
            self.deployments == other.deployments)

    @property
    def name(self):
        return self.group.name

    def load_details(self):
        self.deployments = list(
            self.client.resource.deployments.list(self.name))
        self.is_loaded = True

    def print_out(self, recursive=False):
        print(self.name)
        if recursive:
            for deployment in self.deployments:
                print('    Deployment {}'.format(deployment.name))

    def is_old(self, now, old_age):
        """Return True if the resource group is old.

        :param now: The datetime object that is the basis for old age.
        :param old_age: The age of the resource group to must be.
        """
        if old_age == 0:
            # In the case of O hours old, the caller is stating any resource
            # group that exists is old.
            return True
        ago = timedelta(hours=old_age)
        if not self.deployments:
            # Juju resource groups have at least one deployment, so we can use
            # the timestamp of the oldest deployment in the group as the
            # group's age. If there are no deployments, we don't consider it to
            # be a valid group.
            log.debug('{} has no deployments'.format(self.name))
            return False
        creation_time = min([d.properties.timestamp for d in self.deployments])
        age = now - creation_time
        if age > ago:
            hours_old = (age.total_seconds() // 3600)
            log.debug('{} is {} hours old:'.format(self.name, hours_old))
            log.debug('  {}'.format(creation_time))
            return True
        return False

    def delete(self):
        """Delete the resource group and all subordinate resources.

        Returns a AzureOperationPoller.
        """
        return self.client.resource.resource_groups.delete(self.name)

    def delete_vm(self, name):
        """Delete the VirtualMachine.

        Returns a AzureOperationPoller.
        """
        return self.client.compute.virtual_machines.delete(self.name, name)


def list_resources(client, glob='*', recursive=False, print_out=False):
    """Return a list of ResourceGroupDetails.

    Use print_out=True to print a listing of resources.

    :param client: The ARMClient.
    :param glob: The glob to find matching resource groups to delete.
    :param recursive: Get the resources in the resource group?
    :param print_out: Print the found resources to STDOUT?
    :return: A list of ResourceGroupDetails
    """
    resource_groups = list(iter_resources(client, glob, recursive))
    if print_out:
        for group in resource_groups:
            group.print_out(recursive=recursive)
    return resource_groups


def iter_resources(client, glob='*', recursive=False):
    """Return an iterator of ResourceGroupDetails.

    :param client: The ARMClient.
    :param glob: The glob to find matching resource groups to delete.
    :param recursive: Get the resources in the resource group?
    :return: An iterator of ResourceGroupDetails
    """
    resource_groups = client.resource.resource_groups.list()
    for group in resource_groups:
        if group.name.lower().startswith(DEFAULT_RESOURCE_PREFIX):
            # This is not a resource group. Use the UI to delete Default
            # resources.
            log.debug('Skipping {}'.format(group.name))
            continue
        if not fnmatch.fnmatch(group.name, glob):
            log.debug('Skipping {}'.format(group.name))
            continue
        rgd = ResourceGroupDetails(client, group)
        if recursive:
            print(' - loading {}'.format(group.name))
            rgd.load_details()
        yield rgd


def delete_resources(client, glob='*', old_age=OLD_MACHINE_AGE, now=None):
    """Delete old resource groups and return the number deleted.

    :param client: The ARMClient.
    :param glob: The glob to find matching resource groups to delete.
    :param old_age: The age of the resource group to delete.
    :param now: The datetime object that is the basis for old age.
    """
    if not now:
        now = datetime.now(pytz.utc)
    resources = list_resources(client, glob=glob, recursive=True)
    pollers = []
    deleted_count = 0
    for rgd in resources:
        name = rgd.name
        if not rgd.is_old(now, old_age):
            continue
        log.debug('Deleting {}'.format(name))
        if not client.read_only:
            poller = rgd.delete()
            deleted_count += 1
            if poller:
                pollers.append((name, poller))
            else:
                # Deleting a group created using the old API might not return
                # a poller! Or maybe the resource was deleting already.
                log.debug(
                    'poller is None for {}.delete(). Already deleted?'.format(
                        name))
    for name, poller in pollers:
        log.debug('Waiting for {} to be deleted'.format(name))
        # It is an error to ask for a poller's result() when it is done.
        # Calling result() makes the poller wait for done, but the result
        # of a delete operation is None.
        if not poller.done():
            poller.result()
    return deleted_count


def find_vm_deployment(resource_group, name):
    """Return a matching DeploymentExtended, or None.

    Juju 2.x shows the machine's name in the resource group as the instance_id.

    :param resource_group: A ResourceGroupDetails.
    :param name: The name of a VM instance to find.
    :return: A DeploymentExtended
    """
    if not name.startswith(JUJU_MACHINE_PREFIX):
        return None
    for d in resource_group.deployments:
        if d.name == name:
            return d
    return None


def delete_instance(client, name_id, resource_group=None):
    """Delete a VM instance.

    When resource_group is provided, VM name is used to locate the VM.
    Otherwise, all resource groups are searched for a matching VM id.

    :param name_id: The name or id of a VM instance.
    :param resource_group: The optional name of the resource group the
        VM belongs to.
    """
    if resource_group:
        glob = resource_group
    else:
        glob = '*'
    resource_groups = iter_resources(client, glob=glob, recursive=True)
    group_names = []
    for resource_group in resource_groups:
        group_names.append(resource_group.name)
        deployment = find_vm_deployment(resource_group, name_id)
        if deployment:
            log.debug(
                'Found {} {}'.format(resource_group.name, deployment.name))
            if not client.read_only:
                poller = resource_group.delete_vm(deployment.name)
                log.debug(
                    'Waiting for {} to be deleted'.format(deployment.name))
                if not poller.done():
                    poller.result()
            return
    else:
        group_names = ', '.join(group_names)
        raise ValueError(
            'The vm name {} was not found in {}'.format(name_id, group_names))


def parse_args(argv):
    """Return the argument parser for this program."""
    parser = ArgumentParser(description='Query and manage azure.')
    parser.add_argument(
        '-d', '--dry-run', action='store_true', default=False,
        help='Do not make changes.')
    parser.add_argument(
        '-v', '--verbose', action='store_const',
        default=logging.INFO, const=logging.DEBUG,
        help='Verbose test harness output.')
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
        help='delete old resource groups and their vm, networks, etc.')
    dr_parser.add_argument(
        '-o', '--old-age', default=OLD_MACHINE_AGE, type=int,
        help='Set old machine age to n hours.')
    dr_parser.add_argument(
        'filter', default='*', nargs='?',
        help='A glob pattern to select resource groups to delete.')
    di_parser = subparsers.add_parser('delete-instance', help='Delete a vm.')
    di_parser.add_argument(
        'name_id', help='The name or id of an instance (name needs group).')
    di_parser.add_argument(
        'resource_group', default=None, nargs='?',
        help='The resource-group name of the machine name.')
    args = parser.parse_args(argv[1:])
    if not all(
            [args.subscription_id, args.client_id, args.secret, args.tenant]):
        log.error("$AZURE_SUBSCRIPTION_ID, $AZURE_CLIENT_ID, $AZURE_SECRET, "
                  "$AZURE_TENANT was not provided.")
    return args


def main(argv):
    args = parse_args(argv)
    log.setLevel(args.verbose)
    client = ARMClient(
        args.subscription_id, args.client_id, args.secret, args.tenant,
        read_only=args.dry_run)
    client.init_services()
    try:
        if args.command == 'list-resources':
            list_resources(
                client, glob=args.filter, recursive=args.recursive,
                print_out=True)
        elif args.command == 'delete-resources':
            delete_resources(client, glob=args.filter, old_age=args.old_age)
        elif args.command == 'delete-instance':
            delete_instance(
                client, args.name_id, resource_group=args.resource_group)
    except Exception as e:
        print(e)
        return 1
    return 0


if __name__ == '__main__':
    sys.exit(main(sys.argv))
