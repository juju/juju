import os
from mock import (
    Mock,
    patch,
    )
from assess_agent_metadata import (
    assert_metadata_is_correct,
    get_controller_url_and_sha256,
    verify_deployed_tool,
    assert_cloud_details_are_correct,
    get_local_url_and_sha256,
    get_cloud_details,
    deploy_charm_and_verify,
    verify_deployed_charm,
    deploy_machine_and_verify,
    get_controller_series_and_alternative_series,
    parse_args,
    )

from tests import (
    TestCase,
    )

from utility import (
    JujuAssertionError,
    )

from jujupy import (
    fake_juju_client,
    Status,
    )

AGENT_FILE = '/stream/juju-2.0.1-xenial-amd64.tgz'
SAMPLE_SHA256 = \
    "ce3c940bd7523d307ae546d2f46e722538b0972fbb77abc5ba6bd639400827a8"
VALID_URL = "file:///home/juju/juju-2.0.1-xenial-amd64.tgz"


class TestParseArgs(TestCase):

    def test_common_args(self):
        args = parse_args(["an-env", "/bin/juju", "/tmp/logs",
                           "an-env-mod", '--agent-file', AGENT_FILE])
        self.assertEqual("an-env", args.env)
        self.assertEqual("/bin/juju", args.juju_bin)
        self.assertEqual("/tmp/logs", args.logs)
        self.assertEqual("an-env-mod", args.temp_env_name)
        self.assertEqual(False, args.debug)


class TestAssessAddCloud(TestCase):
    def test_assert_cloud_details_are_correct(self):
        mock_client = Mock()
        expected_cloud = {'clouds': {'foo': {'type': 'lxd', 'config': {
            'agent-metadata-url': 'file:///juju-2.0.1-xenial-amd64.tgz'}}}}
        mock_client.env.read_clouds.return_value = expected_cloud
        assert_cloud_details_are_correct(mock_client, 'foo',
                                         expected_cloud['clouds']['foo'])

    def test_assert_cloud_details_are_correct_assertraises(self):
        mock_client = Mock()
        expected_cloud = {'clouds': {'foo': {'type': 'lxd', 'config': {
            'agent-metadata-url': 'file:///juju-2.0.1-xenial-amd64.tgz'}}}}
        sample_cloud = {'clouds': {'foo1': {'type': 'lxd', 'config': {
            'agent-metadata-url': 'file:///juju-2.0.1-xenial-amd64.tgz'}}}}
        mock_client.env.read_clouds.return_value = sample_cloud
        with self.assertRaises(JujuAssertionError):
            assert_cloud_details_are_correct(mock_client, 'foo',
                                             expected_cloud['clouds']['foo'])

    def test_get_cloud_details(self):
        mock_client = Mock()
        agent_metadata_url = "file:///juju-2.0.1-xenial-amd64.tgz"
        agent_stream = "develop"
        cloud_name = "testcloud"
        cloud_region = "localhost"
        mock_client.env.get_cloud.return_value = cloud_name
        mock_client.env.provider = "lxc"
        mock_client.env.get_region.return_value = cloud_region
        actual_cloud_details = \
            get_cloud_details(mock_client, agent_metadata_url, agent_stream)
        expected_cloud_details = {
            'clouds': {
                cloud_name: {
                    'type': 'lxc',
                    'regions': {cloud_region: {}},
                    'config': {
                        'agent-metadata-url': 'file://{}'.format(
                            agent_metadata_url),
                        'agent-stream': agent_stream,
                    }
                }
            }
        }
        self.assertEquals(actual_cloud_details, expected_cloud_details)


class TestAssessMetadata(TestCase):

    def test_assess_check_metadata(self):
        args = parse_args(['metadata', 'bars', '/foo',
                           '--agent-file', AGENT_FILE])
        mock_client = Mock(spec=["get_model_config"])
        mock_client.get_model_config.return_value = \
            {'agent-metadata-url': {'value': AGENT_FILE}}
        assert_metadata_is_correct(args.agent_file, mock_client)

    def test_assess_check_metadata_invalid(self):
        args = parse_args(['metadata', 'bars', '/foo',
                           '--agent-file', AGENT_FILE])
        mock_client = Mock(spec=["get_model_config"])
        mock_client.get_model_config.return_value = \
            {'agent-metadata-url': {'value': "INVALID"}}
        with self.assertRaises(JujuAssertionError):
            assert_metadata_is_correct(args.agent_file, mock_client)

    def test_get_local_url_and_sha256_valid(self):
        controller_url = \
            "https://example.com/juju-2.0.1-xenial-amd64.tgz"
        agent_dir = "/tmp/juju/"
        agent_stream = "release"
        local_url = os.path.join("file://", agent_dir, "tools", agent_stream,
                                 os.path.basename(controller_url))

        expected_lfp = "file://" + local_url
        expected_sha256 = SAMPLE_SHA256

        with patch('assess_agent_metadata.get_sha256_sum',
                   return_value=expected_sha256):
            local_file_path, local_sha256 = \
                get_local_url_and_sha256(agent_dir, controller_url,
                                         agent_stream)
            self.assertEquals(local_sha256, expected_sha256)
            self.assertEquals(local_file_path, expected_lfp)

    def test_get_local_url_and_sha256_invalid_sha256(self):
        controller_url = \
            "https://example.com/juju-2.0.1-xenial-amd64.tgz"
        agent_dir = "/tmp/juju/"
        agent_stream = "release"
        local_url = os.path.join("file://", agent_dir, "tools", agent_stream,
                                 os.path.basename(controller_url))

        expected_lfp = "file://" + local_url
        expected_sha256 = SAMPLE_SHA256

        with patch('assess_agent_metadata.get_sha256_sum',
                   return_value="ce3c940bd7523d307ae"):
            local_file_path, local_sha256 = \
                get_local_url_and_sha256(agent_dir, controller_url,
                                         agent_stream)
            self.assertNotEquals(local_sha256, expected_sha256)
            self.assertEquals(local_file_path, expected_lfp)

    def test_get_controller_url_and_sha256(self):
        expected_sha256 = SAMPLE_SHA256
        expected_url =\
            "https://example.com/juju-2.0.1-xenial-amd64.tgz"
        mock_client = Mock()
        controller_client = mock_client.get_controller_client()
        with patch.object(mock_client, 'get_controller_client',
                          return_value=controller_client, autospec=True):
            controller_client.run.return_value = [{
                u'MachineId': u'0',
                u'Stdout': u'{"version":"2.0.1-xenial-amd64",'
                           u'"url":'u'"%s",'
                           u'"sha256":"%s",'
                           u'"size":23539756}' % (expected_url, SAMPLE_SHA256)
            }]
            controller_url, controller_sha256 = \
                get_controller_url_and_sha256(mock_client)
            self.assertEqual(controller_url, expected_url)
            self.assertEqual(controller_sha256, expected_sha256)

    def test_verify_deployed_tool_valid(self):
        mock_client = Mock()
        controller_url = VALID_URL
        with patch('assess_agent_metadata.get_local_url_and_sha256',
                   return_value=[controller_url, SAMPLE_SHA256]):
            with patch('assess_agent_metadata.get_controller_url_and_sha256',
                       return_value=[controller_url, SAMPLE_SHA256]):
                verify_deployed_tool("/tmp", mock_client, "testing")

    def test_verify_deployed_tool_invalid_sha256(self):
        mock_client = Mock()
        controller_url = VALID_URL
        with patch('assess_agent_metadata.get_local_url_and_sha256',
                   return_value=[VALID_URL, SAMPLE_SHA256]):
            with patch('assess_agent_metadata.get_controller_url_and_sha256',
                       return_value=[controller_url, "INVALID_SHA256"]):
                with self.assertRaises(JujuAssertionError):
                    verify_deployed_tool("/tmp", mock_client, "testing")

    def test_verify_deployed_tool_empty_local(self):
        mock_client = Mock()
        controller_url = VALID_URL
        with patch('assess_agent_metadata.get_local_url_and_sha256',
                   return_value=[None, None]):
            with patch('assess_agent_metadata.get_controller_url_and_sha256',
                       return_value=[controller_url, "INVALID_SHA256"]):
                with self.assertRaises(JujuAssertionError):
                    verify_deployed_tool("/tmp", mock_client, "testing")

    def test_verify_deployed_tool_empty_controller(self):
        mock_client = Mock()
        with patch('assess_agent_metadata.get_local_url_and_sha256',
                   return_value=[VALID_URL, SAMPLE_SHA256]):
            with patch('assess_agent_metadata.get_controller_url_and_sha256',
                       return_value=[None, None]):
                with self.assertRaises(JujuAssertionError):
                    verify_deployed_tool("/tmp", mock_client, "testing")

    def test_verify_deployed_tool_invalid_url(self):
        mock_client = Mock()
        with patch('assess_agent_metadata.get_local_url_and_sha256',
                   return_value=[VALID_URL, SAMPLE_SHA256]):
            with patch('assess_agent_metadata.get_controller_url_and_sha256',
                       return_value=["file:///INVALID_URL", SAMPLE_SHA256]):
                with self.assertRaises(JujuAssertionError):
                    verify_deployed_tool("/tmp", mock_client, "testing")

    def test_deploy_machine_and_verify(self):
        download_tool_txt = '{' \
                            '"version":"trust-version",' \
                            '"url":"https://example.com",' \
                            '"sha256": "%s",' \
                            '"size":23539776' \
                            '}' % SAMPLE_SHA256
        controller_url_sha = ['url', SAMPLE_SHA256]
        mock_remote = Mock()
        client = fake_juju_client()
        client.bootstrap(bootstrap_series="xenial")
        mock_remote.cat.return_value = download_tool_txt

        with patch('assess_agent_metadata.get_controller_url_and_sha256',
                   return_value=controller_url_sha, autospec=True):
            with patch('assess_agent_metadata.remote_from_address',
                       autospec=True, return_value=mock_remote):
                # deploy add-machine of alternative series that of controller
                deploy_machine_and_verify(client, series="trusty")

    def test_deploy_machine_and_verify_invalid_sha256(self):
        download_tool_txt = '{' \
                            '"version":"trust-version",' \
                            '"url":"https://example.com",' \
                            '"sha256": "%s",' \
                            '"size":23539776' \
                            '}' % SAMPLE_SHA256
        controller_url_sha = ['url', "1234"]
        mock_remote = Mock()
        client = fake_juju_client()
        client.bootstrap()
        mock_remote.cat.return_value = download_tool_txt

        with patch('assess_agent_metadata.get_controller_url_and_sha256',
                   return_value=controller_url_sha, autospec=True):
            with patch('assess_agent_metadata.remote_from_address',
                       autospec=True, return_value=mock_remote):
                with self.assertRaises(JujuAssertionError):
                    deploy_machine_and_verify(client)

    def test_deploy_machine_and_verify_unknown_hostname(self):
        controller_url_sha = ['url', "1234"]
        client = fake_juju_client()
        status = Status({'machines': {"0": {'dns-name': None}}}, '')
        client.bootstrap()
        with patch('assess_agent_metadata.get_controller_url_and_sha256',
                   return_value=controller_url_sha, autospec=True):
            with patch('jujupy.ModelClient.wait_for_started', autospec=True,
                       return_value=status):
                with self.assertRaises(JujuAssertionError):
                    deploy_machine_and_verify(client)

    def test_deploy_charm_and_verify(self):
        mock_client = Mock()
        charm_app = 'dummy-source'
        series = 'xenial'
        with patch('assess_agent_metadata.verify_deployed_charm'):
            with patch('assess_agent_metadata.remote_from_unit'):
                deploy_charm_and_verify(mock_client)
                mock_client.deploy.assert_called_once_with(
                    'local:{}/{}'.format(series, charm_app))
                mock_client.wait_for_started.assert_called_once_with()
                mock_client.set_config.assert_called_once_with(
                    charm_app, {'token': 'one'})
                mock_client.wait_for_workloads.assert_called_once_with()

    def test_deploy_charm_and_verify_series_charm(self):
        mock_client = Mock()
        with patch('assess_agent_metadata.verify_deployed_charm'):
            with patch('assess_agent_metadata.remote_from_unit'):
                deploy_charm_and_verify(mock_client, "trusty", "demo")
                mock_client.deploy.assert_called_once_with('local:trusty/demo')
                mock_client.wait_for_started.assert_called_once_with()

    def test_verify_deployed_charm(self):
        mock_client = Mock()
        download_tool_txt = '{' \
                            '"version":"trust-version",' \
                            '"url":"https://example.com",' \
                            '"sha256": "%s",' \
                            '"size":23539776' \
                            '}' % SAMPLE_SHA256
        controller_url_sha = ['url', SAMPLE_SHA256]
        mock_remote = Mock()
        mock_remote.cat.return_value = download_tool_txt
        with patch('assess_agent_metadata.remote_from_unit', autospec=True,
                   return_value=mock_remote):
            with patch('assess_agent_metadata.get_controller_url_and_sha256',
                       return_value=controller_url_sha, autospec=True):
                verify_deployed_charm(mock_client, mock_remote)

    def test_verify_deployed_charm_invalid_sha256(self):
        mock_client = Mock()
        download_tool_txt = '{' \
                            '"version":"trust-version",' \
                            '"url":"https://example.com",' \
                            '"sha256": "%s",' \
                            '"size":23539776' \
                            '}' % SAMPLE_SHA256
        controller_url_sha = ['url', "12345"]
        mock_remote = Mock()
        mock_remote.cat.return_value = download_tool_txt
        with patch('assess_agent_metadata.remote_from_unit', autospec=True,
                   return_value=mock_remote):
            with patch('assess_agent_metadata.get_controller_url_and_sha256',
                       return_value=controller_url_sha, autospec=True):
                with self.assertRaises(JujuAssertionError):
                    verify_deployed_charm(mock_client, mock_remote)


class TestGetControllerAndAlternativeControllerSeries(TestCase):

    def test_xenial_controller_and_get_alternative_controller_series(self):
        fake_client = Mock(wraps=fake_juju_client())
        get_status_output = Status({
            'machines': {
                '0': {
                    'series': 'xenial'
                }
            }
        }, '')
        fake_client.get_status.return_value = get_status_output
        controller_series, alt_controller_series = \
            get_controller_series_and_alternative_series(fake_client)
        self.assertEquals(controller_series, "xenial")
        self.assertEquals(alt_controller_series, "trusty")

    def test_zesty_controller_to_get_alt_controller_xenial_or_trusty(self):
        fake_client = Mock(wraps=fake_juju_client())
        get_status_output = Status({
            'machines': {
                '0': {
                    'series': 'zesty'
                }
            }
        }, '')
        fake_client.get_status.return_value = get_status_output
        controller_series, alt_controller_series = \
            get_controller_series_and_alternative_series(fake_client)
        self.assertEquals(controller_series, "zesty")
        self.assertIn(alt_controller_series, ["xenial", "trusty"])

    def test_vivid_controller_to_raise_value_error(self):
        fake_client = Mock(wraps=fake_juju_client())
        get_status_output = Status({
            'machines': {
                '0': {
                    'series': 'vivid'
                }
            }
        }, '')
        fake_client.get_status.return_value = get_status_output
        with self.assertRaises(ValueError):
            get_controller_series_and_alternative_series(fake_client)
