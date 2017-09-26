#!/usr/bin/env python

import logging
import os
import yaml

from argparse import ArgumentParser
from jujupy import SimpleEnvironment
from substrate import AWSAccount


def parse_args(argv=None):
    parser = ArgumentParser('Clean up leftover security groups.')
    parser.add_argument('env', help='The juju environment to use.')
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
        logging.info('Cloud list file {} does not exist.'.format(cloud_list))
        raise io_err


def get_security_groups(grp, instgrp, region):
    """Fetch security groups information in a certain region from AWS:
       ID of All security groups
       Security groups attached to existing instances
       Security groups does not attach to any instances
       """
    all_groups = dict(grp)
    instance_groups = dict(instgrp)
    logging.info(
        '{} instance groups found on {}\n{}'.format(
            str(len(instance_groups)),
            region,
            '\n'.join(instance_groups)))
    non_instance_groups = dict(
        (k, v) for k, v in all_groups.items() if k not in instance_groups)
    logging.info(
        '{} non-instance groups found on {}\n{}'.format(
            str(len(non_instance_groups)),
            region,
            '\n'.join(non_instance_groups)))
    return (all_groups, non_instance_groups)


def remove_detached_interfaces(non_instgrp, substrate, region):
    """Remove detached interfaces from non-instance security groups"""
    # TO DO: Would be better to loop non_instgrp.keys() to get detailed output
    logging.info(
        'Detached interfaces will be deleted in region {} '
        'from {} non-instance security groups\n{}'.format(
            region,
            str(len(non_instgrp.keys())),
            '\n'.join(non_instgrp.keys())))
    try:
        unclean = substrate.delete_detached_interfaces(
            non_instgrp.keys())
        logging.info(
            'Detached interfaces have been deleted in region {} '
            'from {} non-instance security groups\n{}'.format(
                region,
                str(len(non_instgrp.keys())),
                '\n'.join(non_instgrp.keys())))
        logging.info(
            'Unable to clean {} groups from {}'.format(
                str(len(unclean)), region))
    except:
        logging.info(
            'Unable to clean non-instance groups in region {} from\n{}'.format(
                region,
                '\n'.join(non_instgrp.keys())))
    return unclean


def remove_security_groups(unclean, grp, non_instgrp, substrate, region):
    """Remove non-instance security groups by group name"""
    for group_id in unclean:
        logging.info(
            'Cannot delete {} from {}'.format(
                grp[group_id], region))
        non_instgrp.pop(group_id, None)
    try:
        substrate.destroy_security_groups(non_instgrp.values())
        logging.info(
            '{} non-instance groups have been deleted from {}\n{}'.format(
                len(non_instgrp.values()),
                region,
                '\n'.join(non_instgrp.values())))
    except:
        logging.info(
            'Failed to delete groups {} from {}'.format(
                non_instgrp.values(), region))


def clean(args):
    env = SimpleEnvironment.from_config(args.env)
    selected_regions = get_regions()
    logging.info(
        'The target regions for cleaning job are \n{}'.format(
            '\n'.join(selected_regions)))
    for region in selected_regions:
        with AWSAccount.from_boot_config(env, region=region) as substrate:
            if substrate is None:
                logging.info(
                    'Skipping {}, substrate object returns NoneType'.format(
                        region))
                continue
            logging.info(
                'Cleaning resources in {}.'.format(substrate.region))
            all_groups, non_instance_groups = get_security_groups(
                grp=substrate.iter_security_groups(),
                instgrp=substrate.iter_instance_security_groups(),
                region=region)
            unclean = remove_detached_interfaces(
                non_instgrp=non_instance_groups,
                substrate=substrate,
                region=region)
            remove_security_groups(
                unclean=unclean,
                grp=all_groups,
                non_instgrp=non_instance_groups,
                substrate=substrate,
                region=region)


def main():
    args = parse_args()
    logging.basicConfig(level=logging.INFO)
    logging.getLogger('clean_resources').setLevel(logging.INFO)
    clean(args)


if __name__ == '__main__':
    main()
