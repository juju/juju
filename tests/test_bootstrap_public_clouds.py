#!/usr/bin/env python
"""Tests for bootstrap_public_clouds."""

import logging
import StringIO

from mock import (
    Mock,
    patch,
    )

from bootstrap_public_clouds import (
    bootstrap_cloud_regions,
    iter_cloud_regions,
    parse_args,
    )
from fakejuju import fake_juju_client
from tests import (
    parse_error,
    TestCase,
    )


class TestParseArgs(TestCase):

    def test_parse_args_start(self):
        args = parse_args(['--start', '7'])
        self.assertEqual(7, args.start)


class TestIterCloudRegions(TestCase):

    @staticmethod
    def collect_cloud_regions(public_clouds, credentials):
        return [config_region for config_region in
                iter_cloud_regions(public_clouds, credentials)]

    def test_iter_cloud_regions(self):
        credentials = {'aws', 'google', 'rackspace'}
        public_clouds = {
            'aws': {'regions': ['north', 'south']},
            'google': {'regions': ['west']},
            }
        regions = self.collect_cloud_regions(public_clouds, credentials)
        self.assertEqual([('default-aws', 'north'), ('default-aws', 'south'),
                          ('default-gce', 'west')], regions)

    def test_iter_cloud_regions_credential_skip(self):
        credentials = {}
        public_clouds = {'rackspace': {'regions': 'none'}}
        with patch('logging.warning') as warning_mock:
            regions = self.collect_cloud_regions(public_clouds, credentials)
        self.assertEqual(1, warning_mock.call_count)
        self.assertEqual([], regions)


class StartArgs:

    def __init__(self, start):
        self.start = start


class TestBootstrapCloud(TestCase):

    def test_bootstrap_cloud_regions(self):
        pc_key = 'public_clouds'
        cred_key = 'credentials'
        cloud_regions = [('foo.config', 'foo'), ('bar.config', 'bar')]
        with patch('bootstrap_public_clouds.iter_cloud_regions',
                   autospec=True, return_value=cloud_regions) as iter_mock:
            with patch('bootstrap_public_clouds.bootstrap_cloud',
                       autospec=True) as bootstrap_mock:
                with patch('logging.info') as info_mock:
                    bootstrap_cloud_regions(pc_key, cred_key, StartArgs(0))
        iter_mock.assert_called_once_with(pc_key, cred_key)
        bootstrap_mock.assert_has_calls([call('foo.config', 'foo'), call('bar.config', 'bar')])
        self.assertEqual(2, info_mock)
