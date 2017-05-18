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

from dateutil import tz


__metaclass__ = type


PERMANENT = 'permanent'
OLD_MACHINE_AGE = 14


# This logger strictly reports the activity of this script.
log = logging.getLogger("aws")
handler = logging.StreamHandler(sys.stderr)
handler.setFormatter(logging.Formatter(
    fmt='%(asctime)s %(levelname)s %(message)s',
    datefmt='%Y-%m-%d %H:%M:%S'))
log.addHandler(handler)


def is_permanent(node):
    """Return True of the node is permanent."""
    # the tags keys only exists if there are tags.
    tags = node.extra.get('tags', {})
    permanent = tags.get(PERMANENT, 'false')
    return permanent.lower() == 'true'


def is_young(node, old_age):
    """Return True if the node is young."""
    now = datetime.now(tz.gettz('UTC'))
    young = True
    # The value is not guaranteed, but is always present in running instances.
    created = node.created_at
    if created:
        creation_time = node.created_at
        age = now - creation_time
        hours = age.total_seconds() // 3600
        log.debug('{} is {} old'.format(node.name, hours))
        ago = timedelta(hours=old_age)
        if age > ago:
            young = False
    return young


def get_client(aws_access_key, aws_secret, region):
    """Delay imports and activation of AWS client as needed."""
    import libcloud
    aws = libcloud.compute.providers.get_driver(
        libcloud.compute.types.Provider.EC2)
    return aws(aws_access_key, aws_secret, region=region)


def list_instances(client, glob='*', print_out=False, states=['running']):
    """Return a list of cloud Nodes.

    Use print_out=True to print a listing of nodes.

    :param client: The AWS client.
    :param glob: The glob to find matching resource groups to delete.
    :param print_out: Print the found resources to STDOUT?
    :return: A list of Nodes
    """
    nodes = []
    for node in client.list_nodes():
        if not (fnmatch.fnmatch(node.name, glob) and node.state in states):
            log.debug('Skipping {}'.format(node.name))
            continue
        nodes.append(node)
    if print_out:
        for node in nodes:
            if node.created_at:
                created = node.created_at.isoformat()
            else:
                created = 'UNKNOWN'
            region_name = client.region_name
            print('{}\t{}\t{}\t{}'.format(
                node.name, region_name, created, node.state))
    return nodes


def delete_instances(client, name_id, old_age=OLD_MACHINE_AGE, dry_run=False):
    """Delete a node instance.

    :param client: The AWS client.
    :param name_id: A glob to match the aws name or Juju instance-id.
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
            try:
                success = client.destroy_node(node)
                if success:
                    log.debug('Deleted {}'.format(node_name))
                    deleted_count += 1
                else:
                    log.error('Cannot delete {}'.format(node_name))
            except:
                log.error('Cannot delete {}'.format(node_name))
    return deleted_count


def parse_args(argv):
    """Return the argument parser for this program."""
    parser = ArgumentParser(description='Query and manage AWS.')
    parser.add_argument(
        '-d', '--dry-run', action='store_true', default=False,
        help='Do not make changes.')
    parser.add_argument(
        '-v', '--verbose', action='store_const',
        default=logging.INFO, const=logging.DEBUG,
        help='Verbose test harness output.')
    parser.add_argument(
        '--aws-access-key',
        help=("The AWS EC2 access key."
              "Environment: $AWS_ACCESS_KEY."),
        default=os.environ.get('AWS_ACCESS_KEY'))
    parser.add_argument(
        '--aws-secret',
        help=("The AWS EC2 secret."
              "Environment: $AWS_SECRET_KEY."),
        default=os.environ.get('AWS_SECRET_KEY'))
    parser.add_argument('region', help="The EC2 region.")
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
        help='A glob pattern to select AWS name or juju instance-id')
    args = parser.parse_args(argv[1:])
    if not all([args.aws_access_key, args.aws_secret]):
        log.error("$AWS_ACCESS_KEY, $AWS_SECRET_KEY was not provided.")
    return args


def main(argv):
    args = parse_args(argv)
    log.setLevel(args.verbose)
    client = get_client(args.aws_access_key, args.aws_secret, args.region)
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
