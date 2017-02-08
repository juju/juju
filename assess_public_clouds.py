#!/usr/bin/env python
"""Assess basic quality of public clouds."""

from __future__ import print_function

from argparse import ArgumentParser
import logging
import os
import sys

import yaml

from deploy_stack import (
    BootstrapManager,
    )
from jujuconfig import get_juju_home
from jujupy import (
    client_from_config,
    )
from assess_cloud import (
    assess_cloud_combined,
    )
from utility import (
    _clean_dir,
    configure_logging,
    _generate_default_binary,
    LoggedException,
    _to_deadline,
    )


log = logging.getLogger('assess_public_clouds')


def make_logging_dir(base_dir, config, region):
    log_dir = os.path.join(base_dir, config, region)
    os.makedirs(log_dir)
    return log_dir


def make_bootstrap_manager(config, region, client, log_dir):
    env_name = 'boot-cpc-{}-{}'.format(client.env.get_cloud(), region)[:30]
    logging_dir = make_logging_dir(log_dir, config, region)
    bs_manager = BootstrapManager(
        env_name, client, client, bootstrap_host=None, machines=[],
        series=None, agent_url=None, agent_stream=None, region=region,
        log_dir=logging_dir, keep_env=False, permanent=True, jes_enabled=True,
        logged_exception_exit=False)
    return bs_manager


CLOUD_CONFIGS = {
        'aws': 'default-aws',
        # sinzui: We may lose this access. No one remaining at Canonical can
        # access the account. There is talk of terminating it.
        'aws-china': 'default-aws-cn',
        'azure': 'default-azure-arm',
        'google': 'default-gce',
        'joyent': 'default-joyent',
        'rackspace': 'default-rackspace',
        }


def iter_cloud_regions(public_clouds, credentials):
    for cloud, info in sorted(public_clouds.items()):
        if cloud not in credentials:
            logging.warning('No credentials for {}.  Skipping.'.format(cloud))
            continue
        for region in sorted(info['regions']):
            yield cloud, region


def bootstrap_cloud_regions(public_clouds, credentials, args):
    cloud_regions = args.cloud_region
    if cloud_regions is None:
        cloud_regions = list(iter_cloud_regions(public_clouds, credentials))
    for num, (cloud, region) in enumerate(cloud_regions):
        if num < args.start:
            continue
        config = CLOUD_CONFIGS[cloud]
        logging.info('Bootstrapping {} {} #{}'.format(config, region, num))
        try:
            client = client_from_config(
                config, args.juju_bin, args.debug, args.deadline)
            bs_manager = make_bootstrap_manager(config, region, client,
                                                args.logs)
            assess_cloud_combined(bs_manager)
        except LoggedException as error:
            yield config, region, error.exception
        except Exception as error:
            logging.exception(
                'Assessment of {} {} failed.'.format(config, region))
            yield config, region, error


def make_cloud_region(cloud_region):
    return tuple(cloud_region.split('/'))


def parse_args(argv):
    """Parse all arguments."""
    parser = ArgumentParser(
        description='Assess basic quality of public clouds.')
    parser.add_argument('juju_bin', nargs='?',
                        help='Full path to the Juju binary. By default, this'
                        ' will use $GOPATH/bin/juju or /usr/bin/juju in that'
                        ' order.', default=_generate_default_binary())
    parser.add_argument('logs', nargs='?', type=_clean_dir,
                        help='A directory in which to store logs. By default,'
                        ' this will use the current directory', default=None)
    parser.add_argument('--cloud-region', type=make_cloud_region, default=None,
                        action='append')
    parser.add_argument('--start', type=int, default=0)
    parser.add_argument('--debug', action='store_true', default=False,
                        help='Pass --debug to Juju.')
    parser.add_argument('--timeout', dest='deadline', type=_to_deadline,
                        help="The script timeout, in seconds.")
    return parser.parse_args(argv)


def yaml_file_load(file_name):
    with open(os.path.join(get_juju_home(), file_name)) as file:
        yaml_data = yaml.safe_load(file)
    return yaml_data


def default_log_dir(settings):
    if settings.logs is None:
        settings.logs = BootstrapManager._generate_default_clean_dir(
            'assess_public_clouds')


def main():
    configure_logging(logging.INFO)
    args = parse_args(None)
    default_log_dir(args)
    public_clouds = yaml_file_load('public-clouds.yaml')['clouds']
    credentials = yaml_file_load('credentials.yaml')['credentials']
    failures = []
    try:
        for failure in bootstrap_cloud_regions(public_clouds, credentials,
                                               args):
            failures.append(failure)
    finally:
        if len(failures) == 0:
            print('No failures!')
            return 0
        else:
            print('Failed:')
            for config, region, e in failures:
                print(' * {} {} {}'.format(config, region, e))
            return 1


if __name__ == '__main__':
    sys.exit(main())
