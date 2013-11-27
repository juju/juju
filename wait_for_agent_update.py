#!/usr/bin/env python
__metaclass__ = type

from jujupy import Environment

from collections import defaultdict
import sys


def agent_update(environment):
    env = Environment(environment)
    env.wait_for_version(env.get_matching_agent_version())


def main():
    try:
       agent_update(sys.argv[1])
    except Exception as e:
        print e
        sys.exit(1)


if __name__ == '__main__':
    main()
