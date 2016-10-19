#!/usr/bin/env python
from argparse import ArgumentParser
import logging
import os
import sys

import yaml

from jujuconfig import get_juju_home
from jujupy import (
    client_from_config,
    get_teardown_timeout,
    )


def bootstrap_cloud(config, region):
    try:
        client = client_from_config(config, 'juju-2.0')
        client.env.environment = 'boot-cpc-{}-{}'.format(
            client.env.get_cloud(), region)[:30]
        client.env.controller.name = client.env.environment
        provider = client.env.config['type']
        if provider == 'azure':
            client.env.config['location'] = region
        elif client.env.config['type'] == 'joyent':
            client.env.config['sdc-url'] = (
                'https://{}.api.joyentcloud.com'.format(region))
        else:
            client.env.config['region'] = region
        if region != client.env.get_region():
            raise ValueError('Failed to set region: {} != {}'.format(
                client.env.get_region(), region))
        client.kill_controller()
        # Not using BootstrapManager, because it doesn't copy
        # public-clouds.yaml (bug #1634570)
        try:
            client.bootstrap()
        except:
            client.kill_controller()
            raise
    except Exception as e:
        logging.exception(e)
        raise
    try:
        try:
            client.wait_for_started()
            client.juju(
                'destroy-controller', (
                    client.env.controller.name, '-y'), include_e=False,
                    timeout=get_teardown_timeout(client))
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
        'google': 'default-gce',
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

def bootstrap_cloud_regions(public_clouds, credentials, start):
    cloud_regions = list(iter_cloud_regions(public_clouds, credentials))
    failures = []
    for num, (config, region) in enumerate(cloud_regions):
        if num < start:
            continue
        logging.info('Bootstrapping {} {} #{}'.format(config, region, num))
        try:
            bootstrap_cloud(config, region)
        except Exception as e:
            yield(config, region, e)


def main():
    logging.basicConfig(level=logging.INFO)
    parser = ArgumentParser()
    parser.add_argument('--start', type=int, default=0)
    args = parser.parse_args()
    logging.warning('This is a quick hack to test 0052b26.  HERE BE DRAGONS!')
    public_clouds_name = os.path.join(get_juju_home(), 'public-clouds.yaml')
    with open(public_clouds_name) as public_clouds_file:
        public_clouds = yaml.safe_load(public_clouds_file)['clouds']
    credentials_name = os.path.join(get_juju_home(), 'credentials.yaml')
    with open(credentials_name) as credentials_file:
        credentials = yaml.safe_load(credentials_file)['credentials']
    failures = []
    try:
        for failure in bootstrap_cloud_regions(public_clouds, credentials,
                                               args.start):
            failures.append(failure)
    finally:
        if len(failures) == 0:
            print('No failures!')
        else:
            failure_str = [
                '{} {} {}'.format(c, r, e) for c, r, e in failures]
            print('Failed:')
            for config, region, e in failures:
                print(' * {} {} {}'.format(config, region, e))


if __name__ == '__main__':
    sys.exit(main())
