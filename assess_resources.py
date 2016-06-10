#!/usr/bin/env python

from __future__ import print_function

import argparse
from collections import namedtuple
import logging
import os
import sys
from tempfile import NamedTemporaryFile

from deploy_stack import (
    BootstrapManager,
)
from jujucharm import CharmCommand
from utility import (
    add_basic_testing_arguments,
    configure_logging,
    temp_dir,
    JujuAssertionError,
    local_charm_path,
    scoped_environ,
)


__metaclass__ = type
log = logging.getLogger("assess_resources")


# Stores credential details for a target charmstore
CharmstoreDetails = namedtuple(
    'CharmstoreDetails',
    ['email', 'username', 'password', 'api_url'])


def _resource_info(name, fingerprint, size):
    data = {}
    data['resourceid'] = "dummy-resource/{}".format(name)
    data['serviceid'] = 'dummy-resource'
    data['name'] = name
    data['type'] = 'file'
    data['description'] = '{} resource.'.format(name)
    data['fingerprint'] = fingerprint
    data['size'] = size
    data['origin'] = 'upload'
    data['used'] = True
    data['username'] = 'admin@local'
    data['path'] = '{}.txt'.format(name)
    return data


def verify_status(status, resource_id, name, fingerprint, size):
    resources = status['resources']
    for resource in resources:
        if resource['expected']['resourceid'] == resource_id:
            expected_values = _resource_info(name, fingerprint, size)
            if resource['expected'] != expected_values:
                raise JujuAssertionError(
                    'Unexpected resource list values: {} Expected: {}'.format(
                        resource['expected'], expected_values))
            if resource['unit'] != expected_values:
                raise JujuAssertionError(
                    'Unexpected unit resource list values: {} Expected: '
                    '{}'.format(resource['unit'], expected_values))
            break
    else:
        raise JujuAssertionError('Resource id not found.')


def push_resource(client, resource_name, finger_print, size, agent_timeout,
                  resource_timeout, deploy=True, resource_file=None):
    charm_name = 'dummy-resource'
    charm_path = local_charm_path(charm=charm_name, juju_ver=client.version)
    if resource_file is None:
        resource_file = os.path.join(
            charm_path, '{}.txt'.format(resource_name))
    else:
        resource_file = os.path.join(charm_path, resource_file)
    resource_arg = '{}={}'.format(resource_name, resource_file)
    log.info("Deploy charm with resource {} Size: {} File: {}".format(
        resource_name, size, resource_file))
    if deploy:
        client.deploy(charm_path, resource=resource_arg)
    else:
        client.attach(charm_name, resource=resource_arg)
    client.wait_for_started(timeout=agent_timeout)
    resource_id = '{}/{}'.format(charm_name, resource_name)
    client.wait_for_resource(
        resource_id, charm_name, timeout=resource_timeout)
    status = client.list_resources(charm_name)
    verify_status(status, resource_id, resource_name, finger_print, size)
    client.show_status()


def fill_dummy_file(file_path, size):
    with open(file_path, "wb") as f:
        f.seek(size - 1)
        f.write('\0')


def large_assess(client, agent_timeout, resource_timeout):
    tests = [
        {"size": 1024 * 1024 * 10,
         "finger_print": ('d7c014629d74ae132cc9f88e3ec2f31652f40a7a1fcc52c54b'
                          '04d6c0d089169bcd55958d1277b4cdf6262f21c712d0a7')},
        {"size": 1024 * 1024 * 100,
         "finger_print": ('c11e93892b66de781e4d0883efe10482f8d1642f3b6574ba2e'
                          'e0da6f8db03f53c0eadfb5e5e0463574c113024ded369e')},
        {"size": 1024 * 1024 * 200,
         "finger_print": ('77db39eca74c6205e31a7701e488a1df4b9b38a527a6084bd'
                          'bb6843fd430a0b51047378ee0255e633b32c0dda3cf43ab')}]
    for test in tests:
        with NamedTemporaryFile(suffix=".txt") as temp_file:
            fill_dummy_file(temp_file.name, size=test['size'])
            push_resource(
                client, 'bar', test['finger_print'], test['size'],
                agent_timeout, resource_timeout, deploy=False,
                resource_file=temp_file.name)


def assess_resources(client, args):
    finger_print = ('4ddc48627c6404e538bb0957632ef68618c0839649d9ad9e41ad94472'
                    'c1589f4b7f9d830df6c4b209d7eb1b4b5522c4d')
    size = 27
    push_resource(client, 'foo', finger_print, size, args.agent_timeout,
                  args.resource_timeout)
    finger_print = ('ffbf43d68a6960de63908bb05c14a026abeda136119d3797431bdd7b'
                    '469c1f027e57a28aeec0df01a792e9e70aad2d6b')
    size = 17
    push_resource(client, 'bar', finger_print, size, args.agent_timeout,
                  args.resource_timeout, deploy=False)
    finger_print = ('2a3821585efcccff1562efea4514dd860cd536441954e182a764991'
                    '0e21f6a179a015677a68a351a11d3d2f277e551e4')
    size = 27
    push_resource(client, 'bar', finger_print, size, args.agent_timeout,
                  args.resource_timeout, deploy=False, resource_file='baz.txt')
    with NamedTemporaryFile(suffix=".txt") as temp_file:
        size = 1024 * 1024
        finger_print = ('3164673a8ac27576ab5fc06b9adc4ce0aca5bd3025384b1cf2128'
                        'a8795e747c431e882785a0bf8dc70b42995db388575')
        fill_dummy_file(temp_file.name, size=size)
        push_resource(client, 'bar', finger_print, size, args.agent_timeout,
                      args.resource_timeout, deploy=False,
                      resource_file=temp_file.name)
    if args.large_test_enabled:
        large_assess(client, args.agent_timeout, args.resource_timeout)


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
                details['username'] = email_address.split('@', 1)[0]
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
        charm_id = 'juju-qa-veebers-testing'
        # Only available for juju 2.x
        charm_path = local_charm_path('dummy-resource', '2.x')
        charm_url = 'cs:{username}/{id}'.format(
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
            attach_resource_to_charm(
                charm_command, temp_bar, charm_id, resource_name='bar')

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

    charm_command.run(
        'push',
        charm_path,
        charm_id,
        '--resource', '{}={}'.format(resource_name, temp_file))


def attach_resource_to_charm(
        charm_command, temp_file, charm_id, resource_name):
    half_meg = 1024 * 512
    fill_dummy_file(temp_file, half_meg)

    charm_command.run('attach', charm_id, '{}={}'.format(
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
    output = charm_command.run('list-resources', charm_url)

    for line in output.split('\n'):
        if line.startswith(resource_name):
            rev = line.split(None, 1)[-1]
            if rev != revno:
                raise JujuAssertionError(
                    'Failed to upload resource and increment revision number.')
            return
    raise JujuAssertionError(
        'Failed to find named resource \'{}\' in output'.format(resource_name))


def check_resource_uploaded_contents(charm_command, charm_url, resource_name):
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
    parser = argparse.ArgumentParser(description="Assess resources")
    add_basic_testing_arguments(parser)
    parser.add_argument('--large-test-enabled', action='store_true',
                        help="Uses large file for testing.")
    parser.add_argument('--agent-timeout', type=int, default=1800,
                        help='The time to wait for agents to start')
    parser.add_argument('--resource-timeout', type=int, default=1800,
                        help='The time to wait for agents to start')
    parser.add_argument('charm_bin',
                        help='Path to binary to use for charm actions.')
    parser.add_argument(
        'credentials_file',
        help='Path to the file containing the charm store credentials and url')
    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
    # bs_manager = BootstrapManager.from_args(args)
    # with bs_manager.booted_context(args.upload_tools):
    #     assess_resources(bs_manager.client, args)

    assess_charmstore_resources(args)
    return 0


if __name__ == '__main__':
    sys.exit(main())
