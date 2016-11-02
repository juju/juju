#!/usr/bin/env python
"""Tests for bootstrap_public_clouds."""

from contextlib import contextmanager
import logging
import StringIO

from mock import (
    call,
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


class TestBootstrapCloudRegions(TestCase):

    @contextmanager
    def patch_for_test(self, cloud_regions, error=None):
        """Handles all the patching for testing bootstrap_cloud_regions."""
        with patch('bootstrap_public_clouds.iter_cloud_regions',
                   autospec=True, return_value=cloud_regions) as iter_mock:
            with patch('bootstrap_public_clouds.bootstrap_cloud',
                       autospec=True, side_effect=error) as bootstrap_mock:
                with patch('logging.info', autospec=True) as info_mock:
                    yield (iter_mock, bootstrap_mock, info_mock)

    def run_test_bootstrap_cloud_regions(self, start=0, error=None):
        pc_key = 'public_clouds'
        cred_key = 'credentials'
        cloud_regions = [('foo.config', 'foo'), ('bar.config', 'bar')]
        args = StartArgs(start)
        expect_calls = [call('foo.config', 'foo', args),
                        call('bar.config', 'bar', args)]
        with self.patch_for_test(cloud_regions, error) as (
                iter_mock, bootstrap_mock, info_mock):
            errors = list(bootstrap_cloud_regions(pc_key, cred_key, args))

        iter_mock.assert_called_once_with(pc_key, cred_key)
        bootstrap_mock.assert_has_calls(expect_calls[start:])
        self.assertEqual(len(cloud_regions) - start, info_mock.call_count)
        if error is None:
            self.assertEqual([], errors)
        else:
            expect_errors = [cr + (error,) for cr in cloud_regions]
            self.assertEqual(expect_errors, errors)

    def test_bootstrap_cloud_regions(self):
        self.run_test_bootstrap_cloud_regions(start=0, error=None)

    def test_bootstrap_cloud_regions_start(self):
        self.run_test_bootstrap_cloud_regions(start=1, error=None)

    def test_bootstrap_cloud_regions_error(self):
        self.run_test_bootstrap_cloud_regions(error=Exception('test'))
