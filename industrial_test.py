#!/usr/bin/env python
__metaclass__ = type

from argparse import ArgumentParser

from jujupy import EnvJujuClient


class IndustrialTest:

    @classmethod
    def from_args(cls, env, new_juju_path):
        old_client = EnvJujuClient.by_version(env)
        new_client = EnvJujuClient.by_version(env, new_juju_path)
        return cls(old_client, new_client)

    def __init__(self, old_client, new_client):
        self.old_client = old_client
        self.new_client = new_client


def parse_args(args=None):
    parser = ArgumentParser()
    parser.add_argument('env')
    parser.add_argument('new_juju_path')
    return parser.parse_args(args)


def main():
    args = parse_args()
    industrial = IndustrialTest.from_args(args)


if __name__ == '__main__':
    main()
