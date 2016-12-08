#!/usr/bin/env python
"""Assess the model-defaults command."""

from __future__ import print_function

import argparse
from contextlib import contextmanager
import logging
import sys

import yaml

from deploy_stack import (
    BootstrapManager,
    )
from utility import (
    add_basic_testing_arguments,
    configure_logging,
    JujuAssertionError,
    )


__metaclass__ = type


log = logging.getLogger('assess_model_defaults')


# I am removing this: the constructor was the only real advantage over a dict
#     so I'm making a function to create the dict and using that instead.
class ModelDefault:

    def __init__(self, model_key, defaults):
        self.model_key = model_key
        self.defaults = defaults

    # Assuming one in the dictionary.
    @staticmethod
    def from_dict(model_kvp):
        for key, value in model_kvp.items():
            return ModelDefault(key, value)

    def to_dict(self):
        return {self.model_key: self.defaults}

    @staticmethod
    def assemble(model_key, default, controller=None, regions=None):
        defaults = {'default': default}
        if controller:
            defaults['controller'] = controller
        if regions:
            defaults['regions'] = [
                {'name': region, 'value': region_default}
                for (region, region_default) in regions.items()]
        return ModelDefault(model_key, defaults)

    @property
    def default(self):
        return self.defaults.get('default')

    @property
    def controller(self):
        return self.defaults.get('controller')

    def region(self, region):
        for current_region in self.defaults.get('regions', []):
            if current_region['name'] == region:
                return current_region['value']

    def __eq__(self, other):
        return (self.model_key == other.model_key and
                self.defaults == other.defaults)

    def __ne__(self, other):
        return (self.model_key != other.model_key or
                self.defaults != other.defaults)

    def __repr__(self):
        return 'ModelDefault({!r}, {!r})'.format(self.model_key,
                                                 self.defaults)


def assemble_model_default(model_key, default, controller=None, regions=None):
    # Ordering in the regions argument is lost.
    defaults = {'default': default}
    if controller:
        defaults['controller'] = controller
    if regions:
        defaults['regions'] = [
           {'name': region, 'value': region_default}
           for (region, region_default) in regions.items()]
    return ModelDefault(model_key, defaults)
    # => {model_key: defaults}


# The next four functions are slated be attached to the client.
def _format_cloud_region(cloud=None, region=None):
    if cloud and region:
        return ('{}/{}'.format(cloud, region),)
    elif region:
        return (region,)
    elif cloud:
        raise ValueError('The cloud must be followed by a region.')
    else:
        return ()


def get_model_defaults(client, model_key, cloud=None, region=None):
    cloud_region = _format_cloud_region(cloud, region)
    gjo_args = ('--format', 'yaml') + cloud_region + (model_key,)
    raw_yaml = client.get_juju_output('model-defaults', *gjo_args)
    return ModelDefault.from_dict(yaml.safe_load(raw_yaml))


def set_model_defaults(client, model_key, value, cloud=None, region=None):
    cloud_region = _format_cloud_region(cloud, region)
    client.juju('model-defaults',
                cloud_region + ('{}={}'.format(model_key, value),))


def unset_model_defaults(client, model_key, cloud=None, region=None):
    cloud_region = _format_cloud_region(cloud, region)
    client.juju('model-defaults',
                cloud_region + ('--reset', model_key))


def juju_assert_equal(lhs, rhs, msg):
    if (lhs != rhs):
        raise JujuAssertionError(msg, lhs, rhs)


@contextmanager
def assert_key_returns_to_base_line(client, model_key):
    base_line = get_model_defaults(client, model_key)
    yield base_line
    juju_assert_equal(
        base_line, get_model_defaults(client, model_key),
        'Defaults for {} did not return to base line.'.format(model_key))


def assess_model_defaults_controller(client, model_key, value):
    base_line = get_model_defaults(client, model_key)
    default = base_line.default

    set_model_defaults(client, model_key, value)
    juju_assert_equal(
        assemble_model_default(model_key, default, value),
        get_model_defaults(client, model_key),
        'model-defaults: Mismatch on setting controller.')

    unset_model_defaults(client, model_key)
    juju_assert_equal(
        base_line, get_model_defaults(client, model_key),
        'model-defaults: Mismatch after resetting controller.')


def assess_model_defaults_region(client, model_key, value,
                                 cloud=None, region=None):
    base_line = get_model_defaults(client, model_key, cloud, region)
    default = base_line.default

    set_model_defaults(client, model_key, value, cloud, region)
    juju_assert_equal(
        'model-defaults: Mismatch on setting region.',
        assemble_model_default(model_key, default, None, {region: value}),
        get_model_defaults(client, model_key, cloud, region))

    unset_model_defaults(client, model_key, cloud, region)
    juju_assert_equal(
        'model-defaults: Mismatch after resetting controller.',
        base_line, get_model_defaults(client, model_key, cloud, region))


def assess_model_defaults(client):
    log.info('Checking controller model-defaults.')
    assess_model_defaults_controller(
        client, 'automatically-retry-hooks', False)
    log.info('Checking region model-defaults.')
    assess_model_defaults_region(
        client, 'default-series', 'trusty', 'localhost', 'localhost')
    # Test on different region?


def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser(
        description='Assess the model-defaults command.')
    add_basic_testing_arguments(parser)
    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
    bs_manager = BootstrapManager.from_args(args)
    with bs_manager.booted_context(args.upload_tools):
        assess_model_defaults(bs_manager.client)
    return 0


if __name__ == '__main__':
    sys.exit(main())
