#!/usr/bin/env python
import logging
import os
import sys

import yaml

from jujuconfig import get_juju_home
from jujupy import client_from_config


def bootstrap_cloud(config, region):
    client = client_from_config(config, 'juju-2.0')
    client.env.environment = 'bootstrap-public-clouds-{}-{}'.format(config,
                                                                    region)
    client.env.controller.name = client.env.environment
    client.env.config['region'] = region
    client.kill_controller()
    client.bootstrap()
    try:
        client.wait_for_started()
        client.juju('destroy-controller')
    except:
        client.kill_controller()


def iter_cloud_regions(public_clouds, credentials):
    configs = {
        'aws': 'default-aws',
        'aws-china': 'default-aws-china',
        'azure': 'default-azure',
        'google': 'default-google',
        'joyent': 'default-joyent',
        'rackspace': 'default-rackspace',
    }
    for cloud, info in sorted(public_clouds.items()):
        if cloud not in credentials:
            logging.warning('No credentials for {}.  Skipping.'.format(cloud))
            continue
        config = configs[cloud]
        for region in sorted(info['regions']):
            yield config, region


def main():
    logging.basicConfig(level=logging.INFO)
    public_clouds_name = os.path.join(get_juju_home(), 'public-clouds.yaml')
    with open(public_clouds_name) as public_clouds_file:
        public_clouds = yaml.safe_load(public_clouds_file)['clouds']
    credentials_name = os.path.join(get_juju_home(), 'credentials.yaml')
    with open(credentials_name) as credentials_file:
        credentials = yaml.safe_load(credentials_file)['credentials']
    cloud_regions = list(iter_cloud_regions(public_clouds, credentials))
    for num, (config, region) in enumerate(cloud_regions):
        logging.info('Bootstrapping {} {} #{}'.format(config, region, num))
        bootstrap_cloud(config, region)

if __name__ == '__main__':
    sys.exit(main())
