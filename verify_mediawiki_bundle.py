#!/usr/bin/env python
from __future__ import print_function

import argparse
import logging
import pickle
import sys
import ssl
from time import sleep
import urllib2

from assess_min_version import JujuAssertionError
from utility import (
    configure_logging,
    until_timeout,
)


__metaclass__ = type


log = logging.getLogger("mediawiki_bundle")


def _get_ssl_ctx():
    try:
        ctx = ssl.create_default_context()
    except AttributeError:
        return None
    ctx.check_hostname = False
    ctx.verify_mode = ssl.CERT_NONE
    return ctx


def wait_for_http(url, timeout=600):
    ctx = _get_ssl_ctx()
    for _ in until_timeout(timeout):
        try:
            if ctx is None:
                req = urllib2.urlopen(url)
            else:
                req = urllib2.urlopen(url, context=ctx)
            if 200 == req.getcode():
                break
        except (urllib2.URLError, urllib2.HTTPError):
            pass
        sleep(.1)
    else:
        raise JujuAssertionError('{} is not reachable'.format(url))
    return req


def verify_services(client, expected_services, scheme='http', text=None,
                    haproxy_exposed=False):
    status = client.get_status()
    if sorted(status.status['services']) != sorted(expected_services):
        raise JujuAssertionError('Unexpected service configuration: {}'.format(
            status.status['services']))
    if not haproxy_exposed:
        if status.status['services']['haproxy']['exposed']:
            raise JujuAssertionError('haproxy is exposed.')
        client.juju('expose', ('haproxy',))
    status = client.get_status()
    if not status.status['services']['haproxy']['exposed']:
        raise JujuAssertionError('haproxy is not exposed.')
    machine_num = (
        status.status['services']['haproxy']['units']['haproxy/0']['machine'])
    haproxy_dns_name = status.status['machines'][machine_num]['dns-name']
    url = '{}://{}'.format(scheme, haproxy_dns_name)
    req = wait_for_http(url)
    if text and text not in req.read():
        raise JujuAssertionError(
            '{} is not found in {}'.format(text, haproxy_dns_name))


def assess_mediawiki_bundle(client):
    logging.info('Assessing mediawiki bundle.')
    status = client.get_status()
    expected_services = ['haproxy', 'mediawiki', 'mysql', 'memcached',
                         'mysql-slave']
    verify_services(client, expected_services)
    client.juju('add-unit', ('mediawiki',))
    client.juju('add-unit', ('mysql-slave',))
    client.wait_for_started()
    status = client.get_status()
    mediawiki_units = status.status['services']['mediawiki']['units'].values()
    if len(mediawiki_units) != 2:
        raise JujuAssertionError(
            'Unexpected mediawiki units: {}'.format(mediawiki_units))
    mysql_units = status.status['services']['mysql-slave']['units'].values()
    if len(mysql_units) != 2:
        raise JujuAssertionError(
            'Unexpected mysql-slave units: {}'.format(mysql_units))


def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser()
    parser.add_argument('client')
    parser.add_argument('--verbose', action='store_const',
                        default=logging.INFO, const=logging.DEBUG,
                        help='Verbose test harness output.')
    args = parser.parse_args(argv)
    args.client = pickle.loads(args.client)
    return args


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
    assess_mediawiki_bundle(args.client)


if __name__ == '__main__':
    sys.exit(main())
