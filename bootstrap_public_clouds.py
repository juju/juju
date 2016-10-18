#!/usr/bin/env python
import logging
import os
import sys

import yaml

from jujuconfig import get_juju_home
from jujupy import client_from_config


def bootstrap_cloud(config, region):
    client = client_from_config(config, 'juju-2.0')
    client.env.environment = 'boot-cpc-{}-{}'.format(client.get_cloud(),
                                                     region)
    client.env.controller.name = client.env.environment
    client.env.config['region'] = region
    client.kill_controller()
    # Not using BootstrapManager, because it doesn't copy public-clouds.yaml
    # (bug #1634570)
    try:
        client.bootstrap()
    except Exception as e:
        logging.exception(e)
        raise
    try:
        try:
            client.wait_for_started()
            client.juju('destroy-controller')
        except Exception as e:
            logging.exception(e)
            raise
    except:
        client.kill_controller()
        raise


def iter_cloud_regions(public_clouds, credentials):
    configs = {
        'aws': 'default-aws',
        'aws-china': 'default-aws-cn',
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

def bootstrap_cloud_regions(public_clouds, credentials):
    cloud_regions = list(iter_cloud_regions(public_clouds, credentials))
    failures = []
    for num, (config, region) in enumerate(cloud_regions):
        logging.info('Bootstrapping {} {} #{}'.format(config, region, num))
        try:
            bootstrap_cloud(config, region)
        except Exception as e:
            yield(config, region, e)


def main():
    logging.basicConfig(level=logging.INFO)
    logging.warning('This is a quick hack to test 0052b26.  HERE BE DRAGONS!')
    public_clouds_name = os.path.join(get_juju_home(), 'public-clouds.yaml')
    with open(public_clouds_name) as public_clouds_file:
        public_clouds = yaml.safe_load(public_clouds_file)['clouds']
    credentials_name = os.path.join(get_juju_home(), 'credentials.yaml')
    with open(credentials_name) as credentials_file:
        credentials = yaml.safe_load(credentials_file)['credentials']
    failures = []
    try:
        for failure in bootstrap_cloud_regions(public_clouds, credentials):
            failures.apppend(failure)
    finally:
        if len(failures) == 0:
            print('No failures!')
        else:
            failure_str = ', '.join(
                '{} {} {}'.format(c, r, e) for c, r, e in failures)
            print('Failed: {}'.format(failure_str))

if __name__ == '__main__':
    sys.exit(main())
