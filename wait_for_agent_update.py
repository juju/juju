#!/usr/bin/env python
from __future__ import print_function

__metaclass__ = type

from jujupy import Environment

import sys


def agent_update(environment):
    env = Environment.from_config(environment)
    env.wait_for_version(env.get_matching_agent_version())


def main():
    try:
       agent_update(sys.argv[1])
    except Exception as e:
        print(e)
        sys.exit(1)


if __name__ == '__main__':
    main()
