#!/usr/bin/env python

from __future__ import print_function

import argparse
import logging
import os
import sys

from deploy_stack import (
    BootstrapManager,
)
from utility import (
    add_basic_testing_arguments,
    configure_logging,
    JujuAssertionError,
    local_charm_path,
)


__metaclass__ = type
log = logging.getLogger("assess_resources")


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


def push_resource(client, resource_name, finger_print, size, deploy=True):
    charm_name = 'dummy-resource'
    charm_path = local_charm_path(charm=charm_name, juju_ver=client.version)
    resource_file = os.path.join(charm_path, '{}.txt'.format(resource_name))
    resource_arg = '{}={}'.format(resource_name, resource_file)
    log.info("Deploy charm with resource {}".format(resource_name))
    if deploy:
        client.deploy(charm_path, resource=resource_arg)
    else:
        client.attach(charm_name, resource=resource_arg)
        # TODO: maybe need to add a wait until resource get is executed.
    client.wait_for_started()
    status = client.list_resources(charm_name)
    resource_id = '{}/{}'.format(charm_name, resource_name)
    verify_status(status, resource_id, resource_name, finger_print, size)


def assess_resources(client):
    finger_print = ('4ddc48627c6404e538bb0957632ef68618c0839649d9ad9e41ad94472'
                    'c1589f4b7f9d830df6c4b209d7eb1b4b5522c4d')
    size = 27
    push_resource(client, 'foo', finger_print, size)
    finger_print = ('ffbf43d68a6960de63908bb05c14a026abeda136119d3797431bdd7b'
                    '469c1f027e57a28aeec0df01a792e9e70aad2d6b')
    size = 17
    push_resource(client, 'bar', finger_print, size, deploy=False)


def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser(description="Assess resources")
    add_basic_testing_arguments(parser)
    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
    bs_manager = BootstrapManager.from_args(args)
    with bs_manager.booted_context(args.upload_tools):
        assess_resources(bs_manager.client)
    return 0


if __name__ == '__main__':
    sys.exit(main())
