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
    env = SimpleEnvironment.from_config(args.env)
    substrate = AWSAccount.from_config(env.config)
    all_groups = dict(substrate.list_security_groups())
    instance_groups = dict(substrate.list_instance_security_groups())
    non_instance_groups = [v for k, v in all_groups.items()
                           if k not in instance_groups]
    substrate.destroy_security_groups(non_instance_groups)


if __name__ == '__main__':
    main()
