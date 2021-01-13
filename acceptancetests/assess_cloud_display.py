#!/usr/bin/env python3

from __future__ import print_function

import os
from argparse import ArgumentParser
from contextlib import contextmanager
from copy import deepcopy
from difflib import ndiff
from pprint import pformat

import sys
import yaml

from utility import (
    add_arg_juju_bin,
    JujuAssertionError,
)
from jujupy import client_from_config


def remove_display_attributes(cloud):
    """Remove the attributes added by display.

    The 'defined' attribute is asserted to be 'local'.
    The description attribute is asserted to be appropriate for the cloud type.
    """
    type_descriptions = {
        'openstack': 'Openstack Cloud',
        'vsphere': '',
        'manual': '',
        'maas': 'Metal As A Service',
        'lxd': 'LXD Container Hypervisor'
    }
    # The lack of built-in descriptions for vsphere and manual is
    # bug #1646128.  The inability to specify descriptions interactively is
    # bug #1645783.
    defined = cloud.pop('defined')
    assert_equal(defined, 'local')
    description = cloud.pop('description', "")

    # Delete None values, which are "errors" from parsing the yaml E.g. output
    # can show values which we show to the customers but should actually not
    # parsed and compared
    for key in cloud.keys():
        if cloud[key] is None:
            del cloud[key]

    try:
        expected_type = type_descriptions[cloud['type']]
    # We skip types we do not have yet, because this is not part of this test.
    # We only want to ensure the description of the above types
    except Exception:
        expected_type = None
    assert_equal(description, expected_type)


def get_clouds(client):
    cloud_list = yaml.safe_load(client.get_juju_output(
        'clouds', '--format', 'yaml', '--local', include_e=False))
    for cloud_name, cloud in cloud_list.items():
        if cloud['defined'] == 'built-in':
            del cloud_list[cloud_name]
            continue
        remove_display_attributes(cloud)
    return cloud_list


def get_home_path(client, subpath):
    return os.path.join(client.env.juju_home, subpath)


def assert_equal(first, second):
    """If two values are not the same, raise JujuAssertionError.

    The text of the error is a diff of the pretty-printed values.
    """
    if first != second:
        diff = ndiff(pformat(first).splitlines(), pformat(second).splitlines())
        raise JujuAssertionError('\n' + '\n'.join(diff))


def assess_clouds(client, expected):
    """Assess how clouds behaves when only expected clouds are defined."""
    cloud_list = get_clouds(client)
    assert_equal(cloud_list, expected)


def assess_show_cloud(client, expected):
    """Assess how show-cloud behaves."""
    for cloud_name, expected_cloud in expected.items():
        actual_cloud = yaml.safe_load(client.get_juju_output(
            'show-cloud', cloud_name, '--format', 'yaml',
            '--local', include_e=False))
        remove_display_attributes(actual_cloud)
        assert_equal(actual_cloud, expected_cloud)


def strip_redundant_endpoints(clouds):
    no_region_endpoint = deepcopy(clouds)
    for cloud in no_region_endpoint.values():
        for region in cloud.get('regions', {}).values():
            if region == {} or cloud.get('endpoint', {}) == {}:
                continue
            if region['endpoint'] == cloud['endpoint']:
                region.pop('endpoint')
    return no_region_endpoint


@contextmanager
def testing(test_name):
    try:
        yield
    except Exception:
        print('{}: FAIL'.format(test_name))
        raise
    else:
        print('{}: PASS'.format(test_name))


def main():
    parser = ArgumentParser()
    parser.add_argument('clouds_file')
    add_arg_juju_bin(parser)
    args = parser.parse_args()
    client = client_from_config(None, args.juju_bin)
    with client.env.make_juju_home(
            client.env.juju_home, 'mytest') as juju_home:
        client.env.juju_home = juju_home
        with open(get_home_path(client, 'public-clouds.yaml'), 'w') as f:
            f.write('')
        with testing('assess_clouds (no_clouds)'):
            assess_clouds(client, {})
        with open(args.clouds_file) as f:
            supplied_clouds = yaml.safe_load(f.read().decode('utf-8'))
        client.env.write_clouds(client.env.juju_home, supplied_clouds)
        no_region_endpoint = strip_redundant_endpoints(
            supplied_clouds['clouds'])
        with testing('assess_clouds'):
            assess_clouds(client, no_region_endpoint)
        with testing('assess_show_cloud'):
            assess_show_cloud(client, no_region_endpoint)


if __name__ == '__main__':
    sys.exit(main())
