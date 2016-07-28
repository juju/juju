#!/usr/bin/env python

from __future__ import print_function

import argparse
from collections import namedtuple
from datetime import datetime
import logging
import os
import sys
from tempfile import NamedTemporaryFile
from textwrap import dedent
from uuid import uuid1

import requests

from jujucharm import (
    CharmCommand,
    local_charm_path,
)
from utility import (
    configure_logging,
    JujuAssertionError,
    scoped_environ,
    temp_dir,
)

__metaclass__ = type
log = logging.getLogger("assess_resources_charmstore")

CHARMSTORE_API_VERSION = 'v5'

# Stores credential details for a target charmstore
CharmstoreDetails = namedtuple(
    'CharmstoreDetails',
    ['email', 'username', 'password', 'api_url'])


# Using a run id we can create a unique charm for each test run allowing us to
# test from fresh.
class RunId:
    _run_id = None

    def __call__(self):
        if self._run_id is None:
            self._run_id = str(uuid1()).replace('-', '')
        return self._run_id


get_run_id = RunId()


def get_charmstore_details(credentials_file=None):
    """Returns a CharmstoreDetails from `credentials_file` or env vars.

    Parses the credentials file (if supplied) and environment variables to
    retrieve the charmstore details and credentials.

    Note. any supplied detail via environment variables will overwrite anything
    supplied in the credentials file..

    """

    required_keys = ('api_url', 'password', 'email', 'username')

    details = {}
    if credentials_file is not None:
        details = parse_credentials_file(credentials_file)

    for key in required_keys:
        env_key = 'CS_{}'.format(key.upper())
        value = os.environ.get(env_key, details.get(key))
        # Can't have empty credential details
        if value is not None:
            details[key] = value

    if not set(details.keys()).issuperset(required_keys):
        raise ValueError('Unable to get all details from file.')

    return CharmstoreDetails(**details)


def split_line_details(string):
    safe_string = string.strip()
    return safe_string.split('=', 1)[-1].strip('"')


def parse_credentials_file(credentials_file):
    details = {}
    with open(credentials_file, 'r') as creds:
        for line in creds.readlines():
            if 'STORE_CREDENTIALS' in line:
                creds = split_line_details(line)
                email_address, password = creds.split(':', 1)
                details['email'] = email_address
                details['password'] = password
                raw_username = email_address.split('@', 1)[0]
                details['username'] = raw_username.replace('.', '-')
            elif 'STORE_URL' in line:
                details['api_url'] = split_line_details(line)
    return details


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
            populate_file_data(temp_foo)
            push_charm_with_resource(
                charm_command,
                temp_foo,
                charm_id,
                charm_path,
                resource_name='foo')

            # Need to grant permissions so we can access details via the http
            # api.
            grant_everyone_access(charm_command, charm_url)

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
            populate_file_data(temp_bar)
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


def populate_file_data(filepath):
    """Write unique data to file at `filepath`."""
    datestamp = datetime.utcnow().isoformat()
    with open(filepath, 'w') as f:
        f.write('{datestamp}:{uuid}'.format(
            datestamp=datestamp,
            uuid=get_run_id()))


def push_charm_with_resource(
        charm_command, temp_file, charm_id, charm_path, resource_name):

    output = charm_command.run(
        'push',
        charm_path,
        charm_id,
        '--resource', '{}={}'.format(resource_name, temp_file))
    log.info('Pushing charm "{id}": {output}'.format(
        id=charm_id, output=output))


def attach_resource_to_charm(
        charm_command, temp_file, charm_id, resource_name):

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
        'Failed to find named resource "{}" in output'.format(resource_name))


def grant_everyone_access(charm_command, charm_url):
    output = charm_command.run('grant', charm_url, 'everyone')
    log.info('Setting permissions on charm: {}'.format(output))


def check_resource_uploaded_contents(
        charm_command, charm_url, resource_name, src_file):
    charmname = charm_url.replace('cs:', '')
    api_url = '{apiurl}/{api_version}/{charmname}/resource/{name}'.format(
        apiurl=charm_command.api_url,
        api_version=CHARMSTORE_API_VERSION,
        charmname=charmname,
        name=resource_name,
    )
    log.info('Using api url: {}'.format(api_url))

    res = requests.get(api_url)

    if not res.ok:
        raise JujuAssertionError('Failed to retrieve details: {}'.format(
            res.content))

    with open(src_file, 'r') as f:
        file_contents = f.read()
    resource_contents = res.content

    raise_if_contents_differ(resource_contents, file_contents)


def raise_if_contents_differ(resource_contents, file_contents):
    if resource_contents != file_contents:
        raise JujuAssertionError(
            dedent("""\
            Resource contents mismatch.
            Expected:
            {}
            Got:
            {}""".format(
                file_contents,
                resource_contents)))


def assess_charmstore_resources(charm_bin, credentials_file):
    with temp_dir() as fake_home:
        temp_env = os.environ.copy()
        temp_env['HOME'] = fake_home
        with scoped_environ(temp_env):
            cs_details = get_charmstore_details(credentials_file)
            ensure_can_push_and_list_charm_with_resources(
                charm_bin, cs_details)


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


def check_resources():
    if os.environ.get('JUJU_REPOSITORY') is None:
        raise AssertionError('JUJU_REPOSITORY required')


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)

    check_resources()

    assess_charmstore_resources(args.charm_bin, args.credentials_file)
    return 0


if __name__ == '__main__':
    sys.exit(main())
