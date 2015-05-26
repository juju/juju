#!/usr/bin/env python
from __future__ import print_function


__metaclass__ = type

from argparse import ArgumentParser
import logging

from utility import configure_logging


def get_args(argv=None):
    parser = ArgumentParser()
    parser.add_argument('env', help='The name of the environment.')
    parser.add_argument('service', help='A service name to monkey with.')
    parser.add_argument(
        'health_checker',
        help='A binary for checking the health of the environment.')
    args = parser.parse_args(argv)
    return args


def main():
    configure_logging(logging.DEBUG)
    args = get_args()
    logging.debug(args)

if __name__ == '__main__':
    main()
