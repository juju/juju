"""Tests for assess_model_config_tree module."""

import argparse
import logging
from mock import Mock, patch
import StringIO

import assess_model_config_tree as amct
from jujupy import fake_juju_client
from tests import (
    parse_error,
    TestCase,
    )
from utility import (
    JujuAssertionError,
    temp_dir,
    )


class TestParseArgs(TestCase):

    def test_default_args(self):
        args = amct.parse_args(
            ["an-env", "/bin/juju", "/tmp/logs", "an-env-mod"])
        self.assertEqual(
            args,
            argparse.Namespace(
                env="an-env",
                juju_bin='/bin/juju',
                logs='/tmp/logs',
                temp_env_name='an-env-mod',
                debug=False,
                agent_stream=None,
                agent_url=None,
                bootstrap_host=None,
                keep_env=False,
                machine=[],
                region=None,
                series=None,
                to=None,
                upload_tools=False,
                verbose=20,
                deadline=None,
                existing=None
                ))

    def test_help(self):
        fake_stdout = StringIO.StringIO()
        with parse_error(self) as fake_stderr:
            with patch("sys.stdout", fake_stdout):
                amct.parse_args(["--help"])
        self.assertEqual("", fake_stderr.getvalue())
        self.assertIn(
            "Test Model Tree Config", fake_stdout.getvalue())


class TestAssertConfigValue(TestCase):

    def test_raises_JujuAssertionError_when_value_doesnt_match(self):
        client = fake_juju_client()
        with patch.object(client, 'get_model_config') as gmc_mock:
            gmc_mock.return_value = {
                'testattr': {'source': 'default', 'value': False}}
            with self.assertRaises(JujuAssertionError):
                amct.assert_config_value(client, 'testattr', 'default', True)

    def test_raises_JujuAssertionError_when_source_doesnt_match(self):
        client = fake_juju_client()
        with patch.object(client, 'get_model_config') as gmc_mock:
            gmc_mock.return_value = {
                'testattr': {'source': 'default', 'value': False}}
            with self.assertRaises(JujuAssertionError):
                amct.assert_config_value(client, 'testattr', 'model', False)

    def test_passes_when_source_and_value_match(self):
        client = fake_juju_client()
        with patch.object(client, 'get_model_config') as gmc_mock:
            gmc_mock.return_value = {
                'testattr': {'source': 'default', 'value': False}}
            amct.assert_config_value(client, 'testattr', 'default', False)

    def test_raises_ValueError_when_attribute_not_present(self):
        client = fake_juju_client()
        with patch.object(client, 'get_model_config') as gmc_mock:
            gmc_mock.return_value = {}
            with self.assertRaises(ValueError):
                amct.assert_config_value(client, 'testattr', 'default', False)


class TestSetCloudsYamlConfig(TestCase):

    def test_appends_valid_cloud_details(self):
        client = fake_juju_client()
        with temp_dir() as juju_home:
            client.env.juju_home = juju_home
            client.env.load_yaml()

            config_details = {'test': 'abc'}
            amct.set_clouds_yaml_config(client, config_details)

        cloud_name = 'foo'
        cloud_details = client.env.clouds['clouds'][cloud_name]
        self.assertEqual(cloud_details['type'], 'foo')
        self.assertEqual(cloud_details['regions'], {'bar': {}})
        self.assertEqual(cloud_details['config'], config_details)


class TestMain(TestCase):

    def test_main(self):
        bs_manager = Mock()
        argv = ["an-env", "/bin/juju", "/tmp/logs", "an-env-mod", "--verbose"]
        with patch.object(amct, "configure_logging", autospec=True) as mock_cl:
            with patch.object(amct.BootstrapManager, 'from_args',
                              return_value=bs_manager):
                    with patch.object(amct, 'assess_model_config_tree',
                                      autospec=True) as mock_assess:
                        amct.main(argv)
        mock_cl.assert_called_once_with(logging.DEBUG)
        mock_assess.assert_called_once_with(bs_manager, False)
