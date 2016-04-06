#!/usr/bin/env python
from __future__ import print_function

import logging
import sys


from utility import configure_logging
from verify_mediawiki_bundle import (
    parse_args,
    verify_services,
)


__metaclass__ = type


def assess_landscape_bundle(client):
    logging.info('Assessing landscaple bundle.')
    expected_services = ['haproxy', 'landscape-server', 'postgresql',
                         'rabbitmq-server']
    verify_services(client, expected_services, scheme='https',
                    text='Landscape')


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
    assess_landscape_bundle(args.client)


if __name__ == '__main__':
    sys.exit(main())
