#!/usr/bin/env python

from argparse import ArgumentParser
import logging

from boto.ec2 import regions

from jujupy import SimpleEnvironment
from substrate import AWSAccount


def parse_args(argv=None):
    parser = ArgumentParser('Clean up leftover resources.')
    parser.add_argument('env', help='The juju environment to use.')
    parser.add_argument('--verbose', '-v', action='count', default=0)
    parser.add_argument('-a', '--all-regions', action='store_true',
                        help='Take action on all regions.')
    return parser.parse_args(argv)


def get_regions(args, env):
    if args.all_regions:
        return [region.name for region in filter(
                lambda x: '-gov-' not in x.name, regions())]
    return [env.get_region()]


def clean(args):
    env = SimpleEnvironment.from_config(args.env)
    # To Do: Read regions from public-clouds.yaml
    selected_regions = ['us-east-1', 'us-east-2', 'us-west-1', 'us-west-2',
                        'ca-central-1',
                        'eu-west-1', 'eu-west-2', 'eu-central-1',
                        'ap-south-1', 'ap-southeast-1', 'ap-southeast-2',
                        'ap-northeast-1', 'ap-northeast-2',
                        'sa-east-1']
    logging.info('The target regions for cleaning job are \n{}'
                 .format('\n'.join(selected_regions)))
    for region in selected_regions:
        with AWSAccount.from_boot_config(env, region=region) as substrate:
            if substrate is not None:
                logging.info('Cleaning resources in {}.'
                             .format(substrate.region))
                all_groups = dict(substrate.iter_security_groups())
                instance_groups = dict(substrate.iter_instance_security_groups())
                logging.info('{} items found from instance groups on {} \n{}'
                             .format(str(len(instance_groups)),
                                     region,
                                     '\n'.join(instance_groups)))
                non_instance_groups = dict((k, v) for k, v in all_groups.items()
                                           if k not in instance_groups)
                logging.info('{} items found from non-instance groups on {} \n{}'
                             .format(str(len(non_instance_groups)),
                                     region,
                                     '\n'.join(non_instance_groups)))
                try:
                    logging.info('{} detached interfaces will be deleted from {} \n{}'
                                 .format(str(len(non_instance_groups.keys())),
                                         region,
                                         '\n'.join(non_instance_groups.keys())))
                    unclean = substrate.delete_detached_interfaces(
                        non_instance_groups.keys())
                    logging.info('Unable to delete {} groups from {}'
                                 .format(str(len(unclean)), region))
                except:
                    logging.info('Unable to delete non-instance groups {} from {}'
                                 .format(non_instance_groups.keys(), region))
                for group_id in unclean:
                    logging.debug('Cannot delete {}'
                                  .format(all_groups[group_id]))
                for group_id in unclean:
                    non_instance_groups.pop(group_id, None)
                try:
                    substrate.destroy_security_groups(non_instance_groups.values())
                    logging.info('{} security groups have been deleted on region {} \n{}'
                                 .format(len(non_instance_groups.values()),
                                         region,
                                         '\n'.join(non_instance_groups.values())))
                except:
                    logging.debug('Failed to delete groups {} from {}'
                                  .format(non_instance_groups.values(), region))
            else:
                logging.info('Skipping {}, substrate object returns NoneType'
                             .format(region))


def main():
    args = parse_args()
    log_level = max(logging.WARN - args.verbose * 10, logging.DEBUG)
    logging.basicConfig(level=log_level)
    logging.getLogger('boto').setLevel(logging.CRITICAL)
    clean(args)


if __name__ == '__main__':
    main()
