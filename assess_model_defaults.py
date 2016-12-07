#!/usr/bin/env python
"""Assess the model-defaults command."""

from __future__ import print_function

import argparse
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


class FakeBackend:

    model_defaults = {}

    test_mode_example = {
        'test-mode': {
            'default': False, # Was false, not sure why it was unquoted.
            'controller': 'true',
            'regions': [{'name': 'localhost', 'value': 'true'}]
            }
        }

    def get_model_defaults(client, model_key):
        return {model_key: model_defaults[model_key]}

    def set_model_defaults(client, model_key, value):
        model_defaults[model_key] = value

    def unset_model_defaults(client, model_key):
        del model_defaults[model_key]


# I might not actually use this, it is just part of my thought process.
class ModelDefault:

    def __init__(self, model_key, defaults):
        self.model_key = model_key
        self.defaults = defaults

    # Assuming one in the dictionary.
    @staticmethod
    def from_dict(model_kvp):
        for key, value in model_kvp.items():
            return ModelDefault(key, value)

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


# All three might be made part of JujuEnvClient
# maybe even beside get/set/unset_env_option
# ... Should this become part of assess_model_config_tree.py?
def model_defaults(client):
    client.get_juju_output('model-defaults', ())


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
#def get_model_defaults(client, model_key, cloud_region=None):
    cloud_region = _format_cloud_region(cloud, region)
    gjo_args = ('--format', 'yaml') + cloud_region + (model_key,)
    raw_yaml = client.get_juju_output('model-defaults', gjo_args)
    return yaml.safe_load(ram_yaml)


def set_model_defaults(client, model_key, value):
    client.juju('model-defaults', _format_cloud_region(cloud, region) +
                                  ('{}={}'.format(model_key, value),))


# Produces output (of post-reset information).
def unset_model_defaults(client, model_key):
    client.juju('model-defaults', _format_cloud_region(cloud, region) +
                                  ('--reset', model_key))

def get_true_default(client, model_key, cloud=None, region=None):
    defaults = get_model_defaults(client, model_key, cloud, region)
    return defaults[model_key]['default']


def assess_model_defaults_controller(client, model_key, value):
    default = get_true_default(client, model_key)
    if ({model_key: {'default': default}} !=
            get_model_defaults(client, model_key)):
        raise JujuAssertionError(
            'model-defaults format does not match expected.')
    set_model_defaults(client, model_key, value)
    if ({model_key: {'default': default, 'controller': value}} !=
            get_model_defaults(client, model_key)):
        raise JujuAssertionError(
            'model-defaults: Mismatch on setting controller.')
    unset_model_defaults(client, model_key)
    if ({model_key: {'default': default}} !=
            get_model_defaults(client, model_key)):
        raise JujuAssertionError(
            'model-defaults: Mismatch after resetting controller.')



def assess_model_defaults_region(client, model_key, value,
                                 cloud=None, region=None):
    default = get_true_default(client, model_key, cloud, region)
    if ({model_key: {'default': default}} !=
            get_model_defaults(client, model_key, cloud, region)):
        raise JujuAssertionError(
            'model-defaults format does not match expected.')
    set_model_defaults(client, model_key, value, cloud, region)
    if ({model_key: {'default': default,
                     'region': [{'name': region, 'value': value}]}} !=
            get_model_defaults(client, model_key, cloud, region)):
        raise JujuAssertionError(
            'model-defaults: Mismatch on setting region.')
    unset_model_defaults(client, model_key, cloud, region)
    if ({model_key: {'default': default}} !=
            get_model_defaults(client, model_key, cloud, region)):
        raise JujuAssertionError(
            'model-defaults: Mismatch after resetting region.')


def assess_model_defaults(client):
    # Deploy charms, there are several under ./repository
    client.deploy('local:trusty/my-charm')
    # Wait for the deployment to finish.
    client.wait_for_started()
    log.info("TO-DO: Add log line about any test")
    # TODO: Add specific functional testing actions here.
    # Test on controller
    # Test on region
    # Test on different region?


def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser(
        description='Assess the model-defaults command.')
    # TODO: Add additional positional arguments.
    add_basic_testing_arguments(parser)
    # TODO: Add additional optional arguments.
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
