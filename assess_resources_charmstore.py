#!/usr/bin/env python

from __future__ import print_function

import argparse
from collections import namedtuple
import logging
import os
import sys
from tempfile import NamedTemporaryFile
from uuid import uuid1

from assess_resources import fill_dummy_file
from jujucharm import CharmCommand
from utility import (
    configure_logging,
    temp_dir,
    JujuAssertionError,
    local_charm_path,
    scoped_environ,
)


__metaclass__ = type
log = logging.getLogger("assess_resources_charmstore")


# Stores credential details for a target charmstore
CharmstoreDetails = namedtuple(
    'CharmstoreDetails',
    ['email', 'username', 'password', 'api_url'])

current_run_uuid = None


def get_run_id():
    global current_run_uuid
    if current_run_uuid is None:
        current_run_uuid = str(uuid1())
        log.info('Generated run id of {}'.format(current_run_uuid))
    return current_run_uuid


def get_charmstore_details(credentials_file):
    """Returns a CharmstoreDetails populated with details from
    `credentials_file`

    """

    def split_line_details(string):
        safe_string = string.strip()
        return safe_string.split('=', 1)[-1].strip('"')

    required_keys = ('api_url', 'password', 'email_address', 'username')

    details = {}
    with open(credentials_file, 'r') as creds:
        for line in creds.readlines():
            if 'STORE_CREDENTIALS' in line:
                creds = split_line_details(line)
                email_address, password = creds.split(':', 1)
                details['email_address'] = email_address
                details['password'] = password
                raw_username = email_address.split('@', 1)[0]
                details['username'] = raw_username.replace('.', '-')
            elif 'STORE_URL' in line:
                details['api_url'] = split_line_details(line)

    if not all(k in details for k in required_keys):
        raise ValueError('Unable to get all details from file.')

    return CharmstoreDetails(
        details['email_address'],
        details['username'],
        details['password'],
        details['api_url'])


def ensure_can_push_and_list_charm_with_resources(charm_bin, cs_details):
    """Ensure that a charm can be pushed to a charm store with a resource.

    Checks that:
      - A charm can be pushed with a resource populated with a file
      - A charm can be updated (attach) after being pushed
      - A charms resources revision is updated after a push or attach

    """
    charm_command = CharmCommand(charm_bin, cs_details.api_url)
    with charm_command.logged_in_user(cs_details.email, cs_details.password):
        charm_id = 'juju-qa-resources-{id}'.format(id=get_run_id())
        # Only available for juju 2.x
        charm_path = local_charm_path('dummy-resource', '2.x')
        charm_url = 'cs:~{username}/{id}-0'.format(
            username=cs_details.username, id=charm_id)

        # Ensure we can publish a charm with a resource
        with NamedTemporaryFile(suffix='.txt') as temp_foo_resource:
            temp_foo = temp_foo_resource.name
            push_charm_with_resource(
                charm_command,
                temp_foo,
                charm_id,
                charm_path,
                resource_name='foo')

            expected_resource_details = {'foo': 0, 'bar': -1}
            check_resource_uploaded(
                charm_command,
                charm_url,
                'foo',
                temp_foo,
                expected_resource_details)

        # Ensure we can attach a resource independently of pushing a charm.
        with NamedTemporaryFile(suffix='.txt') as temp_bar_resource:
            temp_bar = temp_bar_resource.name
            output = attach_resource_to_charm(
                charm_command, temp_bar, charm_url, resource_name='bar')
            log.info(output)

            expected_resource_details = {'foo': 0, 'bar': 0}
            check_resource_uploaded(
                charm_command,
                charm_url,
                'bar',
                temp_bar,
                expected_resource_details)


def push_charm_with_resource(
        charm_command, temp_file, charm_id, charm_path, resource_name):
    half_meg = 1024 * 512
    fill_dummy_file(temp_file, half_meg)

    output = charm_command.run(
        'push',
        charm_path,
        charm_id,
        '--resource', '{}={}'.format(resource_name, temp_file))
    log.info(output)


def attach_resource_to_charm(
        charm_command, temp_file, charm_id, resource_name):
    half_meg = 1024 * 512
    fill_dummy_file(temp_file, half_meg)

    return charm_command.run('attach', charm_id, '{}={}'.format(
        resource_name, temp_file))


def check_resource_uploaded(
        charm_command, charm_url, resource_name, src_file, resource_details):
    for check_name, check_revno in resource_details.items():
        check_resource_uploaded_revno(
            charm_command, charm_url, check_name, check_revno)
    check_resource_uploaded_contents(
        charm_command, charm_url, resource_name, src_file)


def check_resource_uploaded_revno(
        charm_command, charm_url, resource_name, revno):
    """Parse list-resources and ensure resource revno is equal to `revno`.

    :raises JujuAssertionError: If the resources revision is not equal to
      `revno`

    """
    revno = int(revno)
    output = charm_command.run('list-resources', charm_url)

    for line in output.split('\n'):
        if line.startswith(resource_name):
            rev = int(line.split(None, 1)[-1])
            if rev != revno:
                raise JujuAssertionError(
                    'Failed to upload resource and increment revision number.')
            return
    raise JujuAssertionError(
        'Failed to find named resource \'{}\' in output'.format(resource_name))


def check_resource_uploaded_contents(
        charm_command, charm_url, resource_name, src_file):
    # Pull the the charm to a temp file and compare the contents of the pulled
    # resource and those that were pushed.
    # This isn't working as expected so following this up.
    pass


def assess_charmstore_resources(args):
    with temp_dir() as fake_home:
        temp_env = os.environ.copy()
        temp_env['HOME'] = fake_home
        with scoped_environ(temp_env):
            cs_details = get_charmstore_details(args.credentials_file)
            ensure_can_push_and_list_charm_with_resources(
                args.charm_bin,
                cs_details)


def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser(description="Assess resources charmstore")
    parser.add_argument('charm_bin', help='Full path to charn binary')
    parser.add_argument(
        'credentials_file',
        help='Path to the file containing the charm store credentials and url')
    parser.add_argument(
        '--verbose', action='store_const',
        default=logging.INFO, const=logging.DEBUG,
        help='Verbose test harness output.')
    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
    assess_charmstore_resources(args)
    return 0


if __name__ == '__main__':
    sys.exit(main())
