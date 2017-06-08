#!/usr/bin/env python

import logging
import os
import yaml

from argparse import ArgumentParser
from jujupy import SimpleEnvironment
from substrate import AWSAccount


def parse_args(argv=None):
    parser = ArgumentParser('Clean up leftover resources.')
    parser.add_argument('env', help='The juju environment to use.')
    parser.add_argument('--verbose', '-v', action='count', default=0)
    return parser.parse_args(argv)


def get_regions():
    juju_home = os.getenv('JUJU_HOME', default='cloud-city')
    cloud_list = '{}/public-clouds.yaml'.format(juju_home)
    try:
        with open(cloud_list, 'r') as stream:
            try:
                data = yaml.load(stream) 
            except yaml.YAMLError as yaml_err:
                raise yaml_err
            # extract all AWS regions, align with juju regions <cloud>
            selected_regions = data['clouds']['aws']['regions'].keys()
        return selected_regions
    except IOError as io_err:
        print('Cloud list file {} does not exist.'.format(cloud_list))
        raise io_err


def clean(args):
    env = SimpleEnvironment.from_config(args.env)
    selected_regions = get_regions()
    logging.info(
        'The target regions for cleaning job are \n{}'.format(
        '\n'.join(selected_regions)))
    for region in selected_regions:
        with AWSAccount.from_boot_config(env, region=region) as substrate:
            if substrate is not None:
                logging.info(
                    'Cleaning resources in {}.'.format(substrate.region))
                all_groups = dict(substrate.iter_security_groups())
                instance_groups = dict(substrate.iter_instance_security_groups())
                logging.info(
                    '{} items found from instance groups on {} \n{}'.format(
                    str(len(instance_groups)),
                    region,
                    '\n'.join(instance_groups)))
                non_instance_groups = dict(
                    (k, v) for k, v in all_groups.items() if k not in instance_groups)
                logging.info(
                    '{} items found from non-instance groups on {} \n{}'.format(
                    str(len(non_instance_groups)),
                    region,
                    '\n'.join(non_instance_groups)))
                try:
                    logging.info(
                        '{} detached interfaces will be deleted from {} \n{}'.format(
                        str(len(non_instance_groups.keys())),
                        region,
                        '\n'.join(non_instance_groups.keys())))
                    unclean = substrate.delete_detached_interfaces(
                        non_instance_groups.keys())
                    logging.info(
                        'Unable to delete {} groups from {}'.format(
                            str(len(unclean)), region))
                except:
                    logging.info(
                        'Unable to delete non-instance groups {} from {}'.format(
                        non_instance_groups.keys(), region))
                for group_id in unclean:
                    logging.debug(
                        'Cannot delete {} from {}'.format(
                        all_groups[group_id], region))
                for group_id in unclean:
                    non_instance_groups.pop(group_id, None)
                try:
                    substrate.destroy_security_groups(non_instance_groups.values())
                    logging.info(
                        '{} non-instance groups have been deleted from {} \n{}'.format(
                        len(non_instance_groups.values()),
                        region,
                        '\n'.join(non_instance_groups.values())))
                except:
                    logging.debug(
                        'Failed to delete groups {} from {}'.format(
                        non_instance_groups.values(), region))
            else:
                logging.info(
                    'Skipping {}, substrate object returns NoneType'.format(region))


def main():
    args = parse_args()
    log_level = max(logging.WARN - args.verbose * 10, logging.DEBUG)
    logging.basicConfig(level=log_level)
    logging.getLogger('boto').setLevel(logging.CRITICAL)
    clean(args)


if __name__ == '__main__':
    main()
