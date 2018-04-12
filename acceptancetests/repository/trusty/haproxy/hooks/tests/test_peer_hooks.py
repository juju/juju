import base64
import os
import yaml
import pwd

from testtools import TestCase
from mock import patch

import hooks
from utils_for_tests import patch_open


class PeerRelationTest(TestCase):

    def setUp(self):
        super(PeerRelationTest, self).setUp()

        self.relations_of_type = self.patch_hook("relations_of_type")
        self.log = self.patch_hook("log")
        self.unit_get = self.patch_hook("unit_get")

    def patch_hook(self, hook_name):
        mock_controller = patch.object(hooks, hook_name)
        mock = mock_controller.start()
        self.addCleanup(mock_controller.stop)
        return mock

    @patch.dict(os.environ, {"JUJU_UNIT_NAME": "haproxy/2"})
    def test_with_peer_same_services(self):
        self.unit_get.return_value = "1.2.4.5"
        self.relations_of_type.return_value = [
            {"__unit__": "haproxy/1",
             "hostname": "haproxy-1",
             "private-address": "1.2.4.4",
             "all_services": yaml.dump([
                 {"service_name": "foo_service",
                  "service_host": "0.0.0.0",
                  "service_options": ["balance leastconn"],
                  "service_port": 4242},
                 ])
             }
            ]

        services_dict = {
            "foo_service": {
                "service_name": "foo_service",
                "service_host": "0.0.0.0",
                "service_port": 4242,
                "service_options": ["balance leastconn"],
                "server_options": ["maxconn 4"],
                "servers": [("backend_1__8080", "1.2.3.4",
                             8080, ["maxconn 4"])],
                },
            }

        expected = {
            "foo_service": {
                "service_name": "foo_service",
                "service_host": "0.0.0.0",
                "service_port": 4242,
                "service_options": ["balance leastconn",
                                    "mode tcp",
                                    "option tcplog"],
                "servers": [
                    ("haproxy-1", "1.2.4.4", 4243, ["check"]),
                    ("haproxy-2", "1.2.4.5", 4243, ["check", "backup"])
                    ],
                },
            "foo_service_be": {
                "service_name": "foo_service_be",
                "service_host": "0.0.0.0",
                "service_port": 4243,
                "service_options": ["balance leastconn"],
                "server_options": ["maxconn 4"],
                "servers": [("backend_1__8080", "1.2.3.4",
                             8080, ["maxconn 4"])],
                },
            }
        self.assertEqual(expected, hooks.apply_peer_config(services_dict))

    @patch.dict(os.environ, {"JUJU_UNIT_NAME": "haproxy/2"})
    def test_inherit_timeout_settings(self):
        self.unit_get.return_value = "1.2.4.5"
        self.relations_of_type.return_value = [
            {"__unit__": "haproxy/1",
             "hostname": "haproxy-1",
             "private-address": "1.2.4.4",
             "all_services": yaml.dump([
                 {"service_name": "foo_service",
                  "service_host": "0.0.0.0",
                  "service_options": ["timeout server 5000"],
                  "service_port": 4242},
                 ])
             }
            ]

        services_dict = {
            "foo_service": {
                "service_name": "foo_service",
                "service_host": "0.0.0.0",
                "service_port": 4242,
                "service_options": ["timeout server 5000"],
                "server_options": ["maxconn 4"],
                "servers": [("backend_1__8080", "1.2.3.4",
                             8080, ["maxconn 4"])],
                },
            }

        expected = {
            "foo_service": {
                "service_name": "foo_service",
                "service_host": "0.0.0.0",
                "service_port": 4242,
                "service_options": ["balance leastconn",
                                    "mode tcp",
                                    "option tcplog",
                                    "timeout server 5000"],
                "servers": [
                    ("haproxy-1", "1.2.4.4", 4243, ["check"]),
                    ("haproxy-2", "1.2.4.5", 4243, ["check", "backup"])
                    ],
                },
            "foo_service_be": {
                "service_name": "foo_service_be",
                "service_host": "0.0.0.0",
                "service_port": 4243,
                "service_options": ["timeout server 5000"],
                "server_options": ["maxconn 4"],
                "servers": [("backend_1__8080", "1.2.3.4",
                             8080, ["maxconn 4"])],
                },
            }
        self.assertEqual(expected, hooks.apply_peer_config(services_dict))

    @patch.dict(os.environ, {"JUJU_UNIT_NAME": "haproxy/2"})
    def test_with_no_relation_data(self):
        self.unit_get.return_value = "1.2.4.5"
        self.relations_of_type.return_value = []

        services_dict = {
            "foo_service": {
                "service_name": "foo_service",
                "service_host": "0.0.0.0",
                "service_port": 4242,
                "service_options": ["balance leastconn"],
                "server_options": ["maxconn 4"],
                "servers": [("backend_1__8080", "1.2.3.4",
                             8080, ["maxconn 4"])],
                },
            }

        expected = services_dict
        self.assertEqual(expected, hooks.apply_peer_config(services_dict))

    @patch.dict(os.environ, {"JUJU_UNIT_NAME": "haproxy/2"})
    def test_with_missing_all_services(self):
        self.unit_get.return_value = "1.2.4.5"
        self.relations_of_type.return_value = [
            {"__unit__": "haproxy/1",
             "hostname": "haproxy-1",
             "private-address": "1.2.4.4",
             }
            ]

        services_dict = {
            "foo_service": {
                "service_name": "foo_service",
                "service_host": "0.0.0.0",
                "service_port": 4242,
                "service_options": ["balance leastconn"],
                "server_options": ["maxconn 4"],
                "servers": [("backend_1__8080", "1.2.3.4",
                             8080, ["maxconn 4"])],
                },
            }

        expected = services_dict
        self.assertEqual(expected, hooks.apply_peer_config(services_dict))

    @patch('hooks.create_listen_stanza')
    def test_writes_service_config(self, create_listen_stanza):
        create_listen_stanza.return_value = 'some content'
        services_dict = {
            'foo': {
                'service_name': 'bar',
                'service_host': 'some-host',
                'service_port': 'some-port',
                'service_options': 'some-options',
                'servers': (1, 2),
            },
        }

        with patch.object(os.path, "exists") as exists:
            exists.return_value = True
            with patch_open() as (mock_open, mock_file):
                hooks.write_service_config(services_dict)

                create_listen_stanza.assert_called_with(
                    'bar', 'some-host', 'some-port', 'some-options',
                    (1, 2), [], [], [])
                mock_open.assert_called_with(
                    '/var/run/haproxy/bar.service', 'w')
                mock_file.write.assert_called_with('some content')

    @patch('hooks.create_listen_stanza')
    def test_writes_errorfiles(self, create_listen_stanza):
        create_listen_stanza.return_value = 'some content'

        content = ("HTTP/1.0 403 Forbidden\r\n"
                   "Content-Type: text/html\r\n"
                   "\r\n"
                   "<html></html>")
        services_dict = {
            'foo': {
                'service_name': 'bar',
                'service_host': 'some-host',
                'service_port': 'some-port',
                'service_options': 'some-options',
                'servers': (1, 2),
                'errorfiles': [{
                    'http_status': 403,
                    'content': base64.b64encode(content)
                }]
            },
        }

        with patch.object(os.path, "exists") as exists:
            exists.return_value = True
            with patch_open() as (mock_open, mock_file):
                hooks.write_service_config(services_dict)

                mock_open.assert_any_call(
                    '/var/lib/haproxy/service_bar/403.http', 'w')
                mock_file.write.assert_any_call(content)
        self.assertTrue(create_listen_stanza.called)

    @patch('hooks.create_listen_stanza')
    def test_writes_crts(self, create_listen_stanza):
        create_listen_stanza.return_value = 'some content'

        content = ("-----BEGIN CERTIFICATE-----\n"
                   "<data>\n"
                   "-----END CERTIFICATE-----\n")
        services_dict = {
            'foo': {
                'service_name': 'bar',
                'service_host': 'some-host',
                'service_port': 'some-port',
                'service_options': 'some-options',
                'servers': (1, 2),
                'crts': [base64.b64encode(content)]
            },
        }

        with patch.object(os.path, "exists") as exists:
            exists.return_value = True
            with patch_open() as (mock_open, mock_file):
                with patch.object(pwd, "getpwnam") as getpwnam:
                    class DB(object):
                        pw_uid = 9999
                    getpwnam.return_value = DB()
                    with patch.object(os, "chown") as chown:
                        hooks.write_service_config(services_dict)
                        path = '/var/lib/haproxy/service_bar/0.pem'
                        mock_open.assert_any_call(path, 'w')
                        mock_file.write.assert_any_call(content)
                        chown.assert_called_with(path, 9999, - 1)
        self.assertTrue(create_listen_stanza.called)

    @patch('hooks.create_listen_stanza')
    def test_skip_crts_default(self, create_listen_stanza):
        create_listen_stanza.return_value = 'some content'
        services_dict = {
            'foo': {
                'service_name': 'bar',
                'service_host': 'some-host',
                'service_port': 'some-port',
                'service_options': 'some-options',
                'servers': (1, 2),
                'crts': ["DEFAULT"]
            },
        }

        with patch.object(os.path, "exists") as exists:
            exists.return_value = True
            with patch.object(os, "makedirs"):
                with patch_open() as (mock_open, mock_file):
                    hooks.write_service_config(services_dict)
                    self.assertNotEqual(
                        mock_open.call_args,
                        ('/var/lib/haproxy/service_bar/0.pem', 'w'))
        self.assertTrue(create_listen_stanza.called)
