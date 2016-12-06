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

from dateutil import parser as date_parser
from dateutil import tz


__metaclass__ = type


PERMANENT = 'permanent'
OLD_MACHINE_AGE = 14


# This logger strictly reports the activity of this script.
log = logging.getLogger("gce")
handler = logging.StreamHandler(sys.stderr)
handler.setFormatter(logging.Formatter(
    fmt='%(asctime)s %(levelname)s %(message)s',
    datefmt='%Y-%m-%d %H:%M:%S'))
log.addHandler(handler)


def is_permanent(node):
    """Return True of the node is permanent."""
    # the tags keys only exists if there are tags.
    tags = node.extra.get('tags', [])
    return PERMANENT in tags


def is_young(node, old_age):
    """Return True if the node is young."""
    now = datetime.now(tz.gettz('UTC'))
    young = True
    # The value is not guaranteed, but is always present in running instances.
    created = node.extra.get('creationTimestamp')
    if created:
        creation_time = date_parser.parse(created)
        age = now - creation_time
        hours = age.total_seconds() // 3600
        log.debug('{} is {} old'.format(node.name, hours))
        ago = timedelta(hours=old_age)
        if age > ago:
            young = False
    return young


def get_client(sa_email, pem_path, project_id, region=None):
    """Delay imports and activation of GCE client as needed."""
    import libcloud
    gce = libcloud.compute.providers.get_driver(
        libcloud.compute.types.Provider.GCE)
    client = gce(sa_email, pem_path, project=project_id, datacenter=region)
    if region and client.ex_get_zone(region) is None:
        raise ValueError("Unknown region: ", region)
    return client


def list_instances(client, glob='*', print_out=False):
    """Return a list of cloud Nodes.

    Use print_out=True to print a listing of nodes.

    :param client: The GCE client.
    :param glob: The glob to find matching resource groups to delete.
    :param print_out: Print the found resources to STDOUT?
    :return: A list of Nodes
    """
    nodes = []
    for node in client.list_nodes():
        if not fnmatch.fnmatch(node.name, glob):
            log.debug('Skipping {}'.format(node.name))
            continue
        nodes.append(node)
    if print_out:
        for node in nodes:
            created = node.extra.get('creationTimestamp')
            zone = node.extra.get('zone')
            if zone:
                zone_name = zone.name
            else:
                zone_name = 'UNKNOWN'
            print('{}\t{}\t{}\t{}'.format(
                node.name, zone_name, created, node.state))
    return nodes


def delete_instances(client, name_id, old_age=OLD_MACHINE_AGE, dry_run=False):
    """Delete a node instance.

    :param name_id: A glob to match the gce name or Juju instance-id.
    :param old_age: The minimum age to delete.
    :param dry_run: Do not make changes when True.
    """
    nodes = list_instances(client, glob=name_id)
    deleted_count = 0
    deletable = []
    for node in nodes:
        if is_permanent(node):
            log.debug('Skipping {} because it is permanent'.format(node.name))
            continue
        if is_young(node, old_age):
            log.debug('Skipping {} because it is young:'.format(node.name))
            continue
        deletable.append(node)
    if not deletable:
        log.warning(
            'The no machines match {} that are older than {}'.format(
                name_id, old_age))
    for node in deletable:
        node_name = node.name
        log.debug('Deleting {}'.format(node_name))
        if not dry_run:
            # Do not pass destroy_boot_disk=True unless the node has a special
            # boot disk that is not set to autodestroy.
            success = client.destroy_node(node)
            if success:
                log.debug('Deleted {}'.format(node_name))
                deleted_count += 1
            else:
                log.error('Cannot delete {}'.format(node_name))
    return deleted_count


def parse_args(argv):
    """Return the argument parser for this program."""
    parser = ArgumentParser(description='Query and manage GCE.')
    parser.add_argument(
        '-d', '--dry-run', action='store_true', default=False,
        help='Do not make changes.')
    parser.add_argument(
        '-v', '--verbose', action='store_const',
        default=logging.INFO, const=logging.DEBUG,
        help='Verbose test harness output.')
    parser.add_argument(
        '--sa-email',
        help=("The service account email address."
              "Environment: $GCE_SA_EMAIL."),
        default=os.environ.get('GCE_SA_EMAIL'))
    parser.add_argument(
        '--pem-path',
        help=("The path to the PEM file or a json file with PEM data. "
              "Environment: $GCE_PEM_PATH."),
        default=os.environ.get('GCE_PEM_PATH'))
    parser.add_argument(
        '--project-id',
        help=("The secret to make requests with. "
              "Environment: $GCE_PROJECT_ID."),
        default=os.environ.get('GCE_PROJECT_ID'))
    parser.add_argument('--region', help="The compute engine region.")
    subparsers = parser.add_subparsers(help='sub-command help', dest="command")
    ls_parser = subparsers.add_parser(
        'list-instances', help='List vm instances.')
    ls_parser.add_argument(
        'filter', default='*', nargs='?',
        help='A glob pattern to match services to.')
    di_parser = subparsers.add_parser(
        'delete-instances',
        help='delete old resource groups and their vm, networks, etc.')
    di_parser.add_argument(
        '-o', '--old-age', default=OLD_MACHINE_AGE, type=int,
        help='Set old machine age to n hours.')
    di_parser.add_argument(
        'filter',
        help='A glob pattern to select gce name or juju instance-id')
    args = parser.parse_args(argv[1:])
    if not all(
            [args.sa_email, args.pem_path, args.project_id]):
        log.error("$GCE_SA_EMAIL, $GCE_PEM_PATH, $GCE_PROJECT_ID "
                  "was not provided.")
    return args


def main(argv):
    args = parse_args(argv)
    log.setLevel(args.verbose)
    client = get_client(args.sa_email, args.pem_path, args.project_id,
                        region=args.region)
    try:
        if args.command == 'list-instances':
            list_instances(client, glob=args.filter, print_out=True)
        elif args.command == 'delete-instances':
            delete_instances(
                client, args.filter,
                old_age=args.old_age, dry_run=args.dry_run)
    except Exception as e:
        print(e)
        return 1
    return 0


if __name__ == '__main__':
    sys.exit(main(sys.argv))
