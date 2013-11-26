#!/usr/bin/env python
__metaclass__ = type

from jujupy import Environment

from collections import defaultdict
import sys


def agent_update(environment, version):
    env = Environment(environment)
    env.wait_for_version(version)


def main():
    try:
       agent_update(sys.argv[1], sys.argv[2])
    except Exception as e:
        print e
        sys.exit(1)


if __name__ == '__main__':
    main()
