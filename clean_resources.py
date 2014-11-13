#!/usr/bin/env python

from argparse import ArgumentParser
import logging

from jujupy import SimpleEnvironment
from substrate import AWSAccount


def parse_args():
    parser = ArgumentParser('Clean up leftover resources.')
    parser.add_argument('env', help='The juju environment to use.')
    return parser.parse_args()


def main():
    args = parse_args()
    logging.basicConfig(level=logging.INFO)
    logging.getLogger('boto').setLevel(logging.CRITICAL)
    env = SimpleEnvironment.from_config(args.env)
    substrate = AWSAccount.from_config(env.config)
    all_groups = dict(substrate.list_security_groups())
    instance_groups = dict(substrate.list_instance_security_groups())
    non_instance_groups = dict((k, v) for k, v in all_groups.items()
                               if k not in instance_groups)
    unclean = substrate.delete_detached_interfaces(
        non_instance_groups.keys())
    for group_id in unclean:
        non_instance_groups.pop(group_id, None)
    substrate.destroy_security_groups(non_instance_groups.values())
    print "Unable to delete {} groups.".format(len(unclean))
    for group_id in unclean:
        print all_groups[group_id]


if __name__ == '__main__':
    main()
