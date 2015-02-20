#!/usr/bin/env python

from argparse import ArgumentParser
import logging

from jujupy import SimpleEnvironment
from substrate import AWSAccount


def parse_args():
    parser = ArgumentParser('Clean up leftover resources.')
    parser.add_argument('env', help='The juju environment to use.')
    parser.add_argument('--verbose', '-v', action='count', default=0)
    return parser.parse_args()


def main():
    args = parse_args()
    log_level = max(logging.WARN - args.verbose * 10, logging.DEBUG)
    logging.basicConfig(level=log_level)
    logging.getLogger('boto').setLevel(logging.CRITICAL)
    env = SimpleEnvironment.from_config(args.env)
    with AWSAccount.manager_from_config(env.config) as substrate:
        all_groups = dict(substrate.iter_security_groups())
        instance_groups = dict(substrate.iter_instance_security_groups())
        non_instance_groups = dict((k, v) for k, v in all_groups.items()
                                   if k not in instance_groups)
        unclean = substrate.delete_detached_interfaces(
            non_instance_groups.keys())
        logging.info('Unable to delete {} groups'.format(len(unclean)))
        for group_id in unclean:
            logging.debug('Cannot delete {}'.format(all_groups[group_id]))
        for group_id in unclean:
            non_instance_groups.pop(group_id, None)
        substrate.destroy_security_groups(non_instance_groups.values())


if __name__ == '__main__':
    main()
