#!/usr/bin/env python

from argparse import ArgumentParser
from copy import deepcopy
import logging
import sys

import yaml

from jujupy import (
    EnvJujuClient,
    get_client_class,
    JujuData,
    )
from utility import (
    add_arg_juju_bin,
    JujuAssertionError,
    temp_dir,
    )


def assess_cloud(client, example_cloud):
    client.env.load_yaml()
    if len(client.env.clouds['clouds']) > 0:
        raise AssertionError('Clouds already present!')
    client.env.clouds['clouds'].update({'foo': deepcopy(example_cloud)})
    client.add_cloud_interactive('foo')
    client.env.clouds.clear()
    client.env.load_yaml()
    if len(client.env.clouds['clouds']) == 0:
        raise JujuAssertionError('Clouds missing!')
    if client.env.clouds['clouds']['foo'] != example_cloud:
        sys.stderr.write('\nExpected:\n')
        yaml.dump(example_cloud, sys.stderr)
        sys.stderr.write('\nActual:\n')
        yaml.dump(client.env.clouds['clouds']['foo'], sys.stderr)
        raise JujuAssertionError('Cloud mismatch')


def assess_all_clouds(client, clouds):
    succeeded = set()
    failed = set()
    for cloud_name, cloud in clouds.items():
        sys.stdout.write('Testing {}.\n'.format(cloud_name))
        try:
            assess_cloud(client, cloud)
        except Exception as e:
            logging.exception(e)
            failed.add(cloud_name)
        else:
            succeeded.add(cloud_name)
        finally:
            client.env.clouds = {'clouds': {}}
            client.env.dump_yaml(client.env.juju_home, {})
    return succeeded, failed


def write_status(status, tests):
    if len(tests) == 0:
        test_str = 'none'
    else:
        test_str = ', '.join(sorted(tests))
    sys.stdout.write('{}: {}\n'.format(status, test_str))


def parse_args():
    parser = ArgumentParser()
    parser.add_argument('example_clouds',
                        help='A clouds.yaml file to use for testing.')
    add_arg_juju_bin(parser)
    return parser.parse_args()


def main():
    args = parse_args()
    juju_bin = args.juju_bin
    version = EnvJujuClient.get_version(juju_bin)
    client_class = get_client_class(version)
    if client_class.config_class is not JujuData:
        logging.warn('This test does not support old jujus.')
    with open(args.example_clouds) as f:
        clouds = yaml.safe_load(f)['clouds']
    with temp_dir() as juju_home:
        env = JujuData('foo', config=None, juju_home=juju_home)
        client = client_class(env, version, juju_bin)
        succeeded, failed = assess_all_clouds(client, clouds)
    write_status('Succeeded', succeeded)
    write_status('Failed', failed)


if __name__ == '__main__':
    main()
