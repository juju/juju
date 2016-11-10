#!/usr/bin/env python

from argparse import ArgumentParser
from copy import deepcopy


from jujupy import (
    EnvJujuClient,
    get_client_class,
    JujuData,
    )
from utility import (
    add_arg_juju_bin,
    temp_dir,
    )


example_openstack = {
        'type': 'openstack',
        'endpoint': 'http://bar.example.com',
        'auth-types': ['oauth1', 'oauth2'],
        'regions': {
            'harvey': {'endpoint': 'http://harvey.example.com'},
            'steve': {'endpoint': 'http://steve.example.com'},
            }
        }


def assess_openstack(client):
    assess_cloud(client, example_openstack)


def assess_cloud(client, example_cloud):
    client.env.load_yaml()
    if len(client.env.clouds['clouds']) > 0:
        raise AssertionError('Clouds already present!')
    client.env.clouds['clouds'].update({'foo': deepcopy(example_cloud)})
    client.add_cloud_interactive('foo')
    client.env.clouds.clear()
    client.env.load_yaml()
    if len(client.env.clouds['clouds']) == 0:
        raise AssertionError('Clouds missing!')
    if client.env.clouds['clouds']['foo'] != example_cloud:
        raise AssertionError('Cloud mismatch')


def parse_args():
    parser = ArgumentParser()
    add_arg_juju_bin(parser)
    return parser.parse_args()


def main():
    juju_bin = parse_args().juju_bin
    version = EnvJujuClient.get_version(juju_bin)
    client_class = get_client_class(version)
    if client_class.config_class is not JujuData:
        logging.warn('This test does not support old jujus.')
    with temp_dir() as juju_home:
        env = JujuData('foo', config=None, juju_home=juju_home)
        client = client_class(env, version, juju_bin)
        assess_openstack(client)



if __name__ == '__main__':
    main()
