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


class StageAttempt:

    def do_stage(self, old, new):
        self.do_operation(old)
        self.do_operation(new)
        old_result = self.get_result(old)
        new_result = self.get_result(new)
        return old_result, new_result


class BootstrapAttempt:

    def do_operation(self, client):
        client.bootstrap()

    def get_result(self, client):
        try:
            client.wait_for_started()
        except Exception:
            return False
        else:
            return True


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
