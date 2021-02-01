#!/usr/bin/env python3
"""Test Model Tree Config functionality.

Tests that default values can be overwritten and then unset.

Ensures that a model config overwrites a controller config and a controller
config overwrites a default.

Also ensures that when reverting/unsetting the controller config it reverts to
the most up to date model config setting.

"""

from __future__ import print_function

import argparse
import logging
import sys

from deploy_stack import (
    BootstrapManager,
)
from utility import (
    JujuAssertionError,
    add_basic_testing_arguments,
    configure_logging,
)


__metaclass__ = type


log = logging.getLogger("assess_model_config_tree")


def assess_model_config_tree(bs_manager, upload_tools):
    """Assess model config tree functionality."""
    # Need to update the cloud config details here so we can set something that
    # we can use after boostrap.
    config = {'ftp-proxy': 'abc.com'}
    set_clouds_yaml_config(bs_manager.client, config)

    with bs_manager.booted_context(upload_tools):
        client = bs_manager.client
        assert_config_value(client, 'ftp-proxy', 'controller', 'abc.com')
        client.set_env_option('ftp-proxy', 'abc-model.com')
        assert_config_value(client, 'ftp-proxy', 'model', 'abc-model.com')
        client.unset_env_option('ftp-proxy')
        assert_config_value(client, 'ftp-proxy', 'controller', 'abc.com')

        assert_config_value(client, 'development', 'default', False)
        client.set_env_option('development', True)
        assert_config_value(client, 'development', 'model', True)
        client.unset_env_option('development')
        assert_config_value(client, 'development', 'default', False)


def assert_config_value(client, attribute, source, value):
    config_values = client.get_model_config()
    try:
        source_value = config_values[attribute]['source']
        attr_value = config_values[attribute]['value']
        if attr_value != value:
            raise JujuAssertionError(
                'Unexpected value for {}.\nGot {} instead of {}'.format(
                    attribute, attr_value, value))

        if source_value != source:
            raise JujuAssertionError(
                'Unexpected source for {}.\nGot {} instead of {}'.format(
                    attribute, source_value, source))
    except KeyError:
        raise ValueError('Attribute {} not found in config values.'.format(
            attribute))


def set_clouds_yaml_config(client, config_details):
    """Setup cloud details so it gets written to clouds.yaml at bootstrap."""
    cloud_name = client.env.get_cloud()

    extra_conf = {
        'type': client.env.provider,
        'regions': {client.env.get_region(): {}},
        'config': config_details}
    client.env.clouds['clouds'][cloud_name] = extra_conf


def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser(description="Test Model Tree Config")
    # Modifies controller config, can't using existing safely.
    add_basic_testing_arguments(parser, existing=False)
    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
    bs_manager = BootstrapManager.from_args(args)
    assess_model_config_tree(bs_manager, args.upload_tools)
    return 0


if __name__ == '__main__':
    sys.exit(main())
