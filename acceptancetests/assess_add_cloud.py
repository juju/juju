#!/usr/bin/env python

import logging
import re
from argparse import ArgumentParser
from collections import namedtuple
from copy import deepcopy

import sys
import yaml

from jujupy import (
    ModelClient,
    JujuData,
)
from jujupy.exceptions import (
    AuthNotAccepted,
    InvalidEndpoint,
    NameNotAccepted,
    TypeNotAccepted,
)
from utility import (
    add_arg_juju_bin,
    JujuAssertionError,
    temp_dir,
)

# URLs are limited to 2083 bytes in many browsers, anything more is excessive.
# Juju has set 4096 as being excessive, but it needs to be lowered
# https://bugs.launchpad.net/juju/+bug/1678833
EXCEEDED_LIMIT = 4096


class CloudMismatch(JujuAssertionError):
    """The clouds did not match in some way."""

    def __init__(self):
        super(CloudMismatch, self).__init__('Cloud mismatch')


class NameMismatch(JujuAssertionError):
    """The cloud names did not match."""

    def __init__(self):
        super(NameMismatch, self).__init__('Name mismatch')


class NotRaised(Exception):
    """An expected exception was not raised."""

    def __init__(self, cloud_spec):
        msg = 'Expected exception not raised: {}'.format(
            cloud_spec.exception)
        super(NotRaised, self).__init__(msg)


class CloudValidation:
    NONE = object
    BASIC = object()
    ENDPOINT = object()

    def __init__(self, version):
        """Initialize with the juju version."""
        self.version = version
        if re.match('2\.0[^\d]', version):
            self.support = self.NONE
        elif re.match('2\.1[^\d]', version):
            self.support = self.BASIC
        else:
            # re.match('2\.2[^\d]', version)
            # 2.2 retracted manual endpoint validation because it is entangled
            # with authentication.
            self.support = self.ENDPOINT

    @property
    def is_basic(self):
        return self.support is self.BASIC

    @property
    def is_endpoint(self):
        return self.support is self.ENDPOINT

    def has_endpoint(self, provider):
        """Return True if the juju provider supports endpoint validation.

        :param provider: The cloud provider type.
        """
        if self.support is self.ENDPOINT and provider != 'manual':
            return True
        return False


CloudSpec = namedtuple('CloudSpec', [
    'label', 'name', 'config', 'exception', 'xfail_bug'])


def cloud_spec(label, name, config, exception=None, xfail_bug=None):
    """Generate a CloudSpec, with defaults.

    :param label: The label to display in test results.
    :param name: The name to use for the cloud.
    :param config: The cloud-config.
    :param exception: The exception that is expected to be raised (if any).
    :param xfail_bug: If this CloudSpec represents an expected failure, the
        bug number.
    """
    return CloudSpec(label, name, config, exception, xfail_bug)


def xfail(spec, bug, xfail_exception):
    """Return a variant of a CloudSpec that is expected to fail.

    Wrapping the original spec improves maintainability, because the xfail can
    be removed to restore the original value.
    """
    return CloudSpec(spec.label, spec.name, spec.config, xfail_exception, bug)


def assess_cloud(client, cloud_name, example_cloud):
    """Assess interactively adding a cloud.

    Will raise an exception
    - If no clouds are present after interactive add-cloud.
    - If the resulting cloud name doesn't match the supplied cloud-name.
    - If the cloud data doesn't match the supplied cloud data.
    """
    clouds = client.env.read_clouds()
    if len(clouds['clouds']) > 0:
        raise AssertionError('Clouds already present!')
    client.add_cloud_interactive(cloud_name, example_cloud)
    clouds = client.env.read_clouds()
    if len(clouds['clouds']) == 0:
        raise JujuAssertionError('Clouds missing!')
    if clouds['clouds'].keys() != [cloud_name]:
        raise NameMismatch()
    if clouds['clouds'][cloud_name] != example_cloud:
        sys.stderr.write("\nMissmatch for cloud: {}\n".format(cloud_name))
        sys.stderr.write('\nExpected:\n')
        yaml.dump(example_cloud, sys.stderr)
        sys.stderr.write('\nActual:\n')
        yaml.dump(clouds['clouds'][cloud_name], sys.stderr)
        raise CloudMismatch()


def iter_clouds(clouds, cloud_validation):
    """Iterate through CloudSpecs.

    :param clouds: cloud data as defined in $JUJU_DATA/clouds.yaml
    :param cloud_validation: an instance of CloudValidation.
    """
    yield cloud_spec('bogus-type', 'bogus-type', {'type': 'bogus'}, exception=TypeNotAccepted)

    long_text = 'A' * EXCEEDED_LIMIT
    for cloud_name, cloud in clouds.items():
        yield cloud_spec(cloud_name, cloud_name, cloud)
        yield cloud_spec('slash-in-name-{}'.format(cloud_name), 'invalid/name', cloud, NameNotAccepted, 1641981)
        yield cloud_spec('numeral-prefix-{}'.format(cloud_name), '99invalid/name', cloud, NameNotAccepted, 1641981)

        if cloud['type'] not in ('maas', 'manual', 'vsphere'):
            auth_config = deepcopy(cloud)
            auth_config['auth-types'] = ['asdf']
            variant_name = 'bogus-auth-{}'.format(cloud_name)
            yield cloud_spec(variant_name, cloud_name, auth_config, AuthNotAccepted, 1641970)

        if cloud['type'] == 'vsphere':
            continue

        regions = list(cloud.get('regions', {}).keys())

        expected_exception = CloudMismatch
        if cloud_validation.has_endpoint(cloud['type']):
            expected_exception = InvalidEndpoint
        illegal_endpoint_config = deepcopy(cloud)
        illegal_endpoint_config['endpoint'] = long_text
        illegal_endpoint_name = 'long-endpoint-{}'.format(cloud_name)
        for region_name in regions:
            illegal_endpoint_config['regions'][region_name]['endpoint'] = long_text
        yield cloud_spec(illegal_endpoint_name, cloud_name, illegal_endpoint_config, expected_exception, 1641970)

        for region_name in regions:
            regional_long_endpoint_name = 'long-endpoint-{}-{}'.format(cloud_name, region_name)
            regional_long_endpoint_config = deepcopy(cloud)
            # test each region independently of others
            regional_long_endpoint_config['regions'] = {region_name: {'endpoint': long_text}}
            yield cloud_spec(regional_long_endpoint_name, cloud_name, regional_long_endpoint_config, expected_exception,
                             1641970)


def assess_all_clouds(client, cloud_specs):
    """Test all the supplied cloud_specs and return the results.

    Returns a tuple of succeeded, expected_failed, and failed.
    succeeded and failed are sets of cloud labels.  expected_failed is a dict
    linking a given bug to its associated failures.
    """
    succeeded = set()
    xfailed = {}
    failed = set()
    client.env.load_yaml()
    for cloud_spec in cloud_specs:
        sys.stdout.write('\n Testing {}.\n'.format(cloud_spec.label))
        try:
            if cloud_spec.exception is None:
                assess_cloud(client, cloud_spec.name, cloud_spec.config)
            else:
                try:
                    assess_cloud(client, cloud_spec.name, cloud_spec.config)
                except cloud_spec.exception:
                    pass
                else:
                    raise NotRaised(cloud_spec)
        except Exception as e:
            logging.exception(e)
            failed.add(cloud_spec.label)
        else:
            if cloud_spec.xfail_bug is not None:
                xfailed.setdefault(
                    cloud_spec.xfail_bug, set()).add(cloud_spec.label)
            else:
                succeeded.add(cloud_spec.label)
        finally:
            client.env.clouds = {'clouds': {}}
            client.env.dump_yaml(client.env.juju_home)
    return succeeded, xfailed, failed


def write_status(status, tests):
    if len(tests) == 0:
        test_str = 'none'
    else:
        test_str = ', '.join(sorted(tests))
    sys.stdout.write('{}: {}\n'.format(status, test_str))


def parse_args():
    parser = ArgumentParser()
    parser.add_argument('example_clouds',
                        help='A clouds.yaml file to use for testing.')
    add_arg_juju_bin(parser)
    return parser.parse_args()


def main():
    args = parse_args()
    juju_bin = args.juju_bin
    version = ModelClient.get_version(juju_bin)
    with open(args.example_clouds) as f:
        clouds = yaml.safe_load(f)['clouds']
    cloud_validation = CloudValidation(version)
    cloud_specs = iter_clouds(clouds, cloud_validation)
    with temp_dir() as juju_home:
        env = JujuData('foo', config=None, juju_home=juju_home)
        client = ModelClient(env, version, juju_bin)
        succeeded, xfailed, failed = assess_all_clouds(client, cloud_specs)
    write_status('Succeeded', succeeded)
    for bug, failures in sorted(xfailed.items()):
        write_status('Expected fail (bug #{})'.format(bug), failures)
    write_status('Failed', failed)
    if len(failed) > 0:
        return 1
    return 0


if __name__ == '__main__':
    sys.exit(main())
