#!/usr/bin/env python3

from __future__ import print_function

import argparse
import logging
import os
import sys
from tempfile import NamedTemporaryFile

from deploy_stack import (
    BootstrapManager,
)
from jujucharm import (
    local_charm_path,
)
from utility import (
    add_basic_testing_arguments,
    assert_dict_is_subset,
    configure_logging,
    JujuAssertionError,
)


__metaclass__ = type
log = logging.getLogger("assess_resources")


def _resource_info(name, fingerprint, size, service_app_id):
    data = {}
    data['resourceid'] = "dummy-resource/{}".format(name)
    data[service_app_id] = 'dummy-resource'
    data['name'] = name
    data['type'] = 'file'
    data['description'] = '{} resource.'.format(name)
    data['fingerprint'] = fingerprint
    data['size'] = size
    data['origin'] = 'upload'
    data['used'] = True
    data['username'] = 'admin'
    data['path'] = '{}.txt'.format(name)
    return data


def verify_status(status, resource_id, name, fingerprint, size):
    resources = status['resources']
    for resource in resources:
        if resource['expected']['resourceid'] == resource_id:
            if 'serviceid' in resource['unit']:
                service_app_id = 'serviceid'
            else:
                service_app_id = 'applicationId'
            expected_values = _resource_info(name, fingerprint,
                                             size, service_app_id)
            assert_dict_is_subset(expected_values, resource['expected'])
            assert_dict_is_subset(expected_values, resource['unit'])
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

    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
    bs_manager = BootstrapManager.from_args(args)
    with bs_manager.booted_context(args.upload_tools):
        assess_resources(bs_manager.client, args)
    return 0


if __name__ == '__main__':
    sys.exit(main())
