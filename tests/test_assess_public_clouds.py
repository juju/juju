#!/usr/bin/env python
"""Tests for assess_public_clouds."""

from argparse import Namespace
from contextlib import contextmanager
import os

from mock import (
    call,
    Mock,
    patch,
    )
import yaml

from assess_public_clouds import (
    bootstrap_cloud_regions,
    CLOUD_CONFIGS,
    default_log_dir,
    iter_cloud_regions,
    main,
    make_bootstrap_manager,
    make_logging_dir,
    parse_args,
    yaml_file_load,
    )
from deploy_stack import BootstrapManager
from jujupy import (
    fake_juju_client,
    )
from tests import (
    FakeHomeTestCase,
    TestCase,
    )
from utility import (
    temp_dir,
    )


_LOCAL = 'assess_public_clouds'


def patch_local(target, **kwargs):
    return patch(_LOCAL + '.' + target, **kwargs)


class TestParseArgs(TestCase):

    def test_parse_args(self):
        args = parse_args([])
        self.assertEqual(Namespace(
            deadline=None, debug=False, juju_bin='/usr/bin/juju', logs=None,
            start=0, cloud_region=None,
            ), args)

    def test_parse_args_start(self):
        args = parse_args(['--start', '7'])
        self.assertEqual(7, args.start)

    def test_parse_args_cloud_region(self):
        args = parse_args(['--cloud-region', 'foo/bar',
                           '--cloud-region', 'baz/qux'])
        self.assertEqual([('foo', 'bar'), ('baz', 'qux')], args.cloud_region)


class TestMain(TestCase):

    @contextmanager
    def patch_for_test(self, test_iterator, args=[], clouds={}, creds={}):
        fake_yaml = {'clouds': clouds, 'credentials': creds}
        with patch_local('configure_logging', autospec=True):
            with patch_local('parse_args', return_value=parse_args(args)):
                with patch_local('yaml_file_load', return_value=fake_yaml):
                    with patch_local('bootstrap_cloud_regions', autospec=True,
                                     return_value=test_iterator):
                        with patch_local('print') as print_mock:
                            yield print_mock

    def test_main(self):
        with self.patch_for_test([]) as print_mock:
            retval = main()
        self.assertEqual(0, retval)
        print_mock.assert_called_once_with('No failures!')

    def test_main_failures(self):
        errors = [('config', 'region', 'one'), ('config', 'region', 'two')]
        with self.patch_for_test(errors) as print_mock:
            retval = main()
        self.assertEqual(1, retval)
        print_mock.assert_has_calls([
            call('Failed:'),
            call(' * config region one'),
            call(' * config region two'),
            ])
        self.assertEqual(3, print_mock.call_count)


class TestHelpers(TestCase):

    def test_make_logging_dir(self):
        with temp_dir() as root_dir:
            expected_path = os.path.join(root_dir, 'config/region')
            log_dir = make_logging_dir(root_dir, 'config', 'region')
            self.assertTrue(os.path.isdir(expected_path))
        self.assertEqual(expected_path, log_dir)

    def test_yaml_file_load(self):
        expected_data = {'data': {'alpha': 'A', 'beta': 'B'}}
        with temp_dir() as root_dir:
            src_file = os.path.join(root_dir, 'test.yaml')
            with open(src_file, 'w') as yaml_file:
                yaml.safe_dump(expected_data, yaml_file)
            with patch_local('get_juju_home', autospec=True,
                             return_value=root_dir) as get_home_mock:
                data = yaml_file_load('test.yaml')
        get_home_mock.assert_called_once_with()
        self.assertEqual(data, expected_data)

    def test_default_log_dir(self):
        settings = Namespace(logs=None)
        with patch(
                'deploy_stack.BootstrapManager._generate_default_clean_dir',
                return_value='/tmp12345') as clean_dir_mock:
            default_log_dir(settings)
        self.assertEqual('/tmp12345', settings.logs)
        clean_dir_mock.assert_called_once_with(_LOCAL)

    def test_default_log_dir_provided(self):
        settings = Namespace(logs='/tmpABCDE')
        with patch(
                'deploy_stack.BootstrapManager._generate_default_clean_dir',
                autospec=True) as clean_dir_mock:
            default_log_dir(settings)
        self.assertEqual('/tmpABCDE', settings.logs)
        self.assertFalse(clean_dir_mock.called)


class TestMakeBootstrapManager(FakeHomeTestCase):

    def test_make_bootstrap_manager(self):
        client = fake_juju_client()
        bs_manager = Mock()
        with patch_local('make_logging_dir', autospec=True,
                         side_effect=os.path.join):
            with patch_local('assess_cloud_combined', autospec=True):
                bs_manager = make_bootstrap_manager('config', 'region', client,
                                                    'log_dir')
        self.assertEqual(bs_manager.temp_env_name, 'boot-cpc-foo-region')
        self.assertIs(bs_manager.client, client)
        self.assertIs(bs_manager.tear_down_client, client)
        self.assertIs(bs_manager.bootstrap_host, None)
        self.assertEqual(bs_manager.machines, [])
        self.assertIs(bs_manager.series, None)
        self.assertIs(bs_manager.agent_url, None)
        self.assertIs(bs_manager.agent_stream, None)
        self.assertEqual(bs_manager.region, 'region')
        self.assertEqual(bs_manager.log_dir, 'log_dir/config/region')
        self.assertIs(bs_manager.keep_env, False)
        self.assertIs(bs_manager.permanent, True)
        self.assertIs(bs_manager.jes_enabled, True)
        self.assertIs(bs_manager.logged_exception_exit, False)


class TestIterCloudRegions(TestCase):

    def test_iter_cloud_regions(self):
        credentials = {'aws', 'google', 'rackspace'}
        public_clouds = {
            'aws': {'regions': ['north', 'south']},
            'google': {'regions': ['west']},
            }
        regions = list(iter_cloud_regions(public_clouds, credentials))
        self.assertEqual([('aws', 'north'), ('aws', 'south'),
                          ('google', 'west')], regions)

    def test_iter_cloud_regions_credential_skip(self):
        credentials = {}
        public_clouds = {'rackspace': {'regions': 'none'}}
        with patch('logging.warning') as warning_mock:
            regions = list(iter_cloud_regions(public_clouds, credentials))
        self.assertEqual(1, warning_mock.call_count)
        self.assertEqual([], regions)


class TestBootstrapCloudRegions(FakeHomeTestCase):

    @contextmanager
    def patch_for_test(self, cloud_regions, client, error=None):
        """Handles all the patching for testing bootstrap_cloud_regions."""
        with patch_local('iter_cloud_regions', autospec=True,
                         return_value=cloud_regions) as iter_mock:
            with patch_local('assess_cloud_combined', autospec=True,
                             side_effect=error) as bootstrap_mock:
                with patch_local('client_from_config', autospec=True,
                                 return_value=client):
                    with patch('logging.info', autospec=True) as info_mock:
                        with patch_local('make_logging_dir', autospec=True,
                                         side_effect=lambda x, y, z: y):
                            yield (iter_mock, bootstrap_mock, info_mock)

    def run_test_bootstrap_cloud_regions(self, start=0, error=None,
                                         cloud_regions=None):
        pc_key = 'public_clouds'
        cred_key = 'credentials'
        default_cloud_regions = [('aws', 'foo'), ('google', 'bar')]
        args = Namespace(start=start, debug=True, deadline=None,
                         juju_bin='juju', logs='/tmp/log',
                         cloud_region=cloud_regions)
        if cloud_regions is None:
            cloud_regions = default_cloud_regions
        fake_client = fake_juju_client()
        config_regions = [(CLOUD_CONFIGS[c], r) for (c, r) in cloud_regions]
        with self.patch_for_test(default_cloud_regions, fake_client,
                                 error) as (
                iter_mock, bootstrap_mock, info_mock):
            errors = list(bootstrap_cloud_regions(pc_key, cred_key, args))

        if cloud_regions is default_cloud_regions:
            iter_mock.assert_called_once_with(pc_key, cred_key)
        else:
            self.assertEqual(0, iter_mock.call_count)
        for num, (config, region) in enumerate(config_regions[start:]):
            acc_call = bootstrap_mock.mock_calls[num]
            name, args, kwargs = acc_call
            self.assertEqual({}, kwargs)
            (bs_manager,) = args
            self.assertIsInstance(bs_manager, BootstrapManager)
            self.assertEqual(bs_manager.region, region)
            self.assertEqual(bs_manager.log_dir, config)
        self.assertEqual(len(default_cloud_regions) - start,
                         info_mock.call_count)
        if error is None:
            self.assertEqual([], errors)
        else:
            expect_errors = [cr + (error,) for cr in config_regions]
            self.assertEqual(expect_errors, errors)

    def test_bootstrap_cloud_regions(self):
        self.run_test_bootstrap_cloud_regions(start=0, error=None)

    def test_bootstrap_cloud_regions_start(self):
        self.run_test_bootstrap_cloud_regions(start=1, error=None)

    def test_bootstrap_cloud_regions_error(self):
        self.run_test_bootstrap_cloud_regions(error=Exception('test'))

    def test_bootstrap_cloud_regions_specific_regions(self):
        self.run_test_bootstrap_cloud_regions(cloud_regions=[
            ('aws', 'us-perpendicular-1'),
            ('rackspace', 'tla'),
            ])
