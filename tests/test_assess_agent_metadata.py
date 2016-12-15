from mock import (
    Mock,
    patch,
    )

from assess_agent_metadata import (
    assert_metadata_are_correct,
    get_controller_url_and_sha256,
    verify_deployed_tool,
    assert_cloud_details_are_correct,
    parse_args,
    )

from tests import (
    TestCase,
    )

from utility import (
    JujuAssertionError,
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


class TestAssess(TestCase):
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

    def test_assess_check_metadata(self):
        args = parse_args(['metadata', 'bars', '/foo',
                           '--agent-file', AGENT_FILE])
        mock_client = Mock(spec=["get_model_config"])
        mock_client.get_model_config.return_value = \
            {'agent-metadata-url': {'value': AGENT_FILE}}
        assert_metadata_are_correct(args.agent_file, mock_client)

    def test_assess_check_metadata_invalid(self):
        args = parse_args(['metadata', 'bars', '/foo',
                           '--agent-file', AGENT_FILE])
        mock_client = Mock(spec=["get_model_config"])
        mock_client.get_model_config.return_value = \
            {'agent-metadata-url': {'value': "INVALID"}}
        with self.assertRaises(JujuAssertionError):
            assert_metadata_are_correct(args.agent_file, mock_client)

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
                verify_deployed_tool("/tmp", mock_client)

    def test_verify_deployed_tool_invalid_sha256(self):
        mock_client = Mock()
        controller_url = VALID_URL
        with patch('assess_agent_metadata.get_local_url_and_sha256',
                   return_value=[VALID_URL, SAMPLE_SHA256]):
            with patch('assess_agent_metadata.get_controller_url_and_sha256',
                       return_value=[controller_url, "INVALID_SHA256"]):
                with self.assertRaises(JujuAssertionError):
                    verify_deployed_tool("/tmp", mock_client)

    def test_verify_deployed_tool_empty_local(self):
        mock_client = Mock()
        controller_url = VALID_URL
        with patch('assess_agent_metadata.get_local_url_and_sha256',
                   return_value=[None, None]):
            with patch('assess_agent_metadata.get_controller_url_and_sha256',
                       return_value=[controller_url, "INVALID_SHA256"]):
                with self.assertRaises(JujuAssertionError):
                    verify_deployed_tool("/tmp", mock_client)

    def test_verify_deployed_tool_empty_controller(self):
        mock_client = Mock()
        with patch('assess_agent_metadata.get_local_url_and_sha256',
                   return_value=[VALID_URL, SAMPLE_SHA256]):
            with patch('assess_agent_metadata.get_controller_url_and_sha256',
                       return_value=[None, None]):
                with self.assertRaises(JujuAssertionError):
                    verify_deployed_tool("/tmp", mock_client)

    def test_verify_deployed_tool_invalid_url(self):
        mock_client = Mock()
        with patch('assess_agent_metadata.get_local_url_and_sha256',
                   return_value=[VALID_URL, SAMPLE_SHA256]):
            with patch('assess_agent_metadata.get_controller_url_and_sha256',
                       return_value=["file:///INVALID_URL", SAMPLE_SHA256]):
                with self.assertRaises(JujuAssertionError):
                    verify_deployed_tool("/tmp", mock_client)
