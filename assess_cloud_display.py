#!/usr/bin/env python
from argparse import ArgumentParser
import json
import os

import yaml

from jujupy import client_from_config
from utility import add_arg_juju_bin


def get_clouds(client):
    cloud_list = json.loads(client.get_juju_output(
        'list-clouds', '--format', 'json', include_e=False))
    for cloud_name, cloud in cloud_list.items():
        if cloud['defined'] == 'built-in':
            del cloud_list[cloud_name]
    return cloud_list


def get_home_path(client, subpath):
    return os.path.join(client.env.juju_home, subpath)


def main():
    parser = ArgumentParser()
    parser.add_argument('clouds_file')
    add_arg_juju_bin(parser)
    args = parser.parse_args()
    client = client_from_config(None, args.juju_bin)
    with client.env.make_jes_home(client.env.juju_home, 'mytest',
                                  {}) as juju_home:
        with open(get_home_path(client, 'public-clouds.yaml'), 'w') as f:
            f.write('')
        cloud_list = get_clouds(client)
        if cloud_list != {}:
            print cloud_list
        with open(args.clouds_file) as f:
            supplied_clouds = yaml.load(f)['clouds']
        client.env.write_clouds(client.env.juju_home, supplied_clouds)
        cloud_list = get_clouds(client)
        if cloud_list != supplied_clouds:
            print cloud_list



if __name__ == '__main__':
    main()
