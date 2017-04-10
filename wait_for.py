#!/usr/bin/env python

from __future__ import print_function


from argparse import ArgumentParser
import sys

from jujupy import (
    client_for_existing,
    get_juju_data,
    )
from jujupy.client import (
    NoActiveModel,
    WaitAgentsStarted,
    )


def main():
    parser = ArgumentParser()
    subparsers = parser.add_subparsers(dest='cmd')
    started_parser = subparsers.add_parser(
        'agents-started', description=WaitAgentsStarted.__doc__,
        )
    started_parser.set_defaults(factory=WaitAgentsStarted)
    started_parser.add_argument('--timeout', type=int)
    args = parser.parse_args()
    try:
        client = client_for_existing(None, get_juju_data())
    except NoActiveModel as e:
        print(e, file=sys.stderr)
        sys.exit(1)
    kwargs = dict((k, v) for k, v in vars(args).items()
                  if k not in ('factory', 'cmd'))
    client.wait_for(args.factory(**kwargs))


if __name__ == '__main__':
    main()
