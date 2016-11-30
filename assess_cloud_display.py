#!/usr/bin/env python
from __future__ import print_function

from argparse import ArgumentParser
from contextlib import contextmanager
from copy import deepcopy
from difflib import ndiff
import os
from pprint import pformat

import yaml

from jujupy import client_from_config
from utility import (
    add_arg_juju_bin,
    JujuAssertionError,
    )


def get_clouds(client):
    type_descriptions = {
        'openstack': 'Openstack Cloud',
        'vsphere': '',
        'manual': '',
        'maas': 'Metal As A Service',
        }
    cloud_list = yaml.load(client.get_juju_output(
        'clouds', '--format', 'yaml', include_e=False))
    for cloud_name, cloud in cloud_list.items():
        if cloud['defined'] == 'built-in':
            del cloud_list[cloud_name]
            continue
        defined = cloud.pop('defined')
        assert_equal(defined, 'local')
        description = cloud.pop('description')
        assert_equal(description, type_descriptions[cloud['type']])
    return cloud_list


def get_home_path(client, subpath):
    return os.path.join(client.env.juju_home, subpath)


def assert_equal(first, second):
    if first != second:
        diff = ndiff(pformat(first).splitlines(), pformat(second).splitlines())
        raise JujuAssertionError('\n' + '\n'.join(diff))


def assess_clouds_no_clouds(client):
    """Assess how clouds behaves when no clouds are defined."""
    with open(get_home_path(client, 'public-clouds.yaml'), 'w') as f:
        f.write('')
    cloud_list = get_clouds(client)
    assert_equal(cloud_list, {})


def assess_clouds(client, expected):
    """Assess how clouds behaves when only expected clouds are defined."""
    cloud_list = get_clouds(client)
    assert_equal(cloud_list, expected)


def strip_redundant_endpoints(clouds):
    no_region_endpoint = deepcopy(clouds)
    for cloud in no_region_endpoint.values():
        for region in cloud.get('regions', {}).values():
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
    with client.env.make_jes_home(client.env.juju_home, 'mytest',
                                  {}) as juju_home:
        client.env.juju_home = juju_home
        with testing('assess_clouds_no_clouds'):
            assess_clouds_no_clouds(client)
        with open(args.clouds_file) as f:
            supplied_clouds = yaml.safe_load(f.read().decode('utf-8'))
        client.env.write_clouds(client.env.juju_home, supplied_clouds)
        no_region_endpoint = strip_redundant_endpoints(
            supplied_clouds['clouds'])
        with testing('assess_clouds'):
            assess_clouds(client, no_region_endpoint)


if __name__ == '__main__':
    main()
