import os
import base64
import yaml

from testtools import TestCase
from mock import patch, call

import hooks


class ReverseProxyRelationTest(TestCase):

    def setUp(self):
        super(ReverseProxyRelationTest, self).setUp()

        self.config_get = self.patch_hook("config_get")
        self.config_get.return_value = {"monitoring_port": "10000",
                                        "peering_mode": "active-passive"}
        self.relations_of_type = self.patch_hook("relations_of_type")
        self.get_config_services = self.patch_hook("get_config_services")
        self.log = self.patch_hook("log")
        self.write_service_config = self.patch_hook("write_service_config")
        self.apply_peer_config = self.patch_hook("apply_peer_config")
        self.apply_peer_config.side_effect = lambda value: value

    def patch_hook(self, hook_name):
        mock_controller = patch.object(hooks, hook_name)
        mock = mock_controller.start()
        self.addCleanup(mock_controller.stop)
        return mock

    def test_relation_data_returns_none(self):
        self.get_config_services.return_value = {
            "service": {
                "service_name": "service",
                },
            }
        self.relations_of_type.return_value = []
        self.assertIs(None, hooks.create_services())
        self.log.assert_called_once_with("No backend servers, exiting.")
        self.write_service_config.assert_not_called()

    def test_relation_data_returns_no_relations(self):
        self.get_config_services.return_value = {
            "service": {
                "service_name": "service",
                },
            }
        self.relations_of_type.return_value = []
        self.assertIs(None, hooks.create_services())
        self.log.assert_called_once_with("No backend servers, exiting.")
        self.write_service_config.assert_not_called()

    def test_relation_no_services(self):
        self.get_config_services.return_value = {}
        self.relations_of_type.return_value = [
            {"port": 4242,
             "__unit__": "foo/0",
             "hostname": "backend.1",
             "private-address": "1.2.3.4"},
        ]
        self.assertIs(None, hooks.create_services())
        self.log.assert_called_once_with("No services configured, exiting.")
        self.write_service_config.assert_not_called()

    def test_no_port_in_relation_data(self):
        self.get_config_services.return_value = {
            "service": {
                "service_name": "service",
                },
            }
        self.relations_of_type.return_value = [
            {"private-address": "1.2.3.4",
             "__unit__": "foo/0"},
        ]
        self.assertIs(None, hooks.create_services())
        self.log.assert_has_calls([call.log(
            "No port in relation data for 'foo/0', skipping.")])
        self.write_service_config.assert_not_called()

    def test_no_private_address_in_relation_data(self):
        self.get_config_services.return_value = {
            "service": {
                "service_name": "service",
                },
            }
        self.relations_of_type.return_value = [
            {"port": 4242,
             "__unit__": "foo/0"},
        ]
        self.assertIs(None, hooks.create_services())
        self.log.assert_has_calls([call.log(
            "No private-address in relation data for 'foo/0', skipping.")])
        self.write_service_config.assert_not_called()

    def test_relation_unknown_service(self):
        self.get_config_services.return_value = {
            "service": {
                "service_name": "service",
                },
            }
        self.relations_of_type.return_value = [
            {"port": 4242,
             "hostname": "backend.1",
             "service_name": "invalid",
             "private-address": "1.2.3.4",
             "__unit__": "foo/0"},
        ]
        self.assertIs(None, hooks.create_services())
        self.log.assert_has_calls([call.log(
            "Service 'invalid' does not exist.")])
        self.write_service_config.assert_not_called()

    def test_no_relation_but_has_servers_from_config(self):
        self.get_config_services.return_value = {
            None: {
                "service_name": "service",
                },
            "service": {
                "service_name": "service",
                "servers": [
                    ("legacy-backend", "1.2.3.1", 4242, ["maxconn 42"]),
                    ]
                },
            }
        self.relations_of_type.return_value = []

        expected = {
            'service': {
                'service_name': 'service',
                'service_host': '0.0.0.0',
                'service_port': 10002,
                'servers': [
                    ("legacy-backend", "1.2.3.1", 4242, ["maxconn 42"]),
                    ],
                },
            }
        self.assertEqual(expected, hooks.create_services())
        self.write_service_config.assert_called_with(expected)

    def test_relation_default_service(self):
        self.get_config_services.return_value = {
            None: {
                "service_name": "service",
                },
            "service": {
                "service_name": "service",
                },
            }
        self.relations_of_type.return_value = [
            {"port": 4242,
             "hostname": "backend.1",
             "private-address": "1.2.3.4",
             "__unit__": "foo/0"},
        ]

        expected = {
            'service': {
                'service_name': 'service',
                'service_host': '0.0.0.0',
                'service_port': 10002,
                'servers': [('foo-0-4242', '1.2.3.4', 4242, [])],
                },
            }
        self.assertEqual(expected, hooks.create_services())
        self.write_service_config.assert_called_with(expected)

    def test_with_service_options(self):
        self.get_config_services.return_value = {
            None: {
                "service_name": "service",
                },
            "service": {
                "service_name": "service",
                "server_options": ["maxconn 4"],
                },
            }
        self.relations_of_type.return_value = [
            {"port": 4242,
             "hostname": "backend.1",
             "private-address": "1.2.3.4",
             "__unit__": "foo/0"},
        ]

        expected = {
            'service': {
                'service_name': 'service',
                'service_host': '0.0.0.0',
                'service_port': 10002,
                'server_options': ["maxconn 4"],
                'servers': [('foo-0-4242', '1.2.3.4',
                             4242, ["maxconn 4"])],
                },
            }
        self.assertEqual(expected, hooks.create_services())
        self.write_service_config.assert_called_with(expected)

    def test_with_service_name(self):
        self.get_config_services.return_value = {
            None: {
                "service_name": "service",
                },
            "foo_service": {
                "service_name": "foo_service",
                "server_options": ["maxconn 4"],
                },
            }
        self.relations_of_type.return_value = [
            {"port": 4242,
             "hostname": "backend.1",
             "service_name": "foo_service",
             "private-address": "1.2.3.4",
             "__unit__": "foo/0"},
        ]

        expected = {
            'foo_service': {
                'service_name': 'foo_service',
                'service_host': '0.0.0.0',
                'service_port': 10002,
                'server_options': ["maxconn 4"],
                'servers': [('foo-0-4242', '1.2.3.4',
                             4242, ["maxconn 4"])],
                },
            }
        self.assertEqual(expected, hooks.create_services())
        self.write_service_config.assert_called_with(expected)

    def test_no_service_name_unit_name_match_service_name(self):
        self.get_config_services.return_value = {
            None: {
                "service_name": "foo_service",
                },
            "foo_service": {
                "service_name": "foo_service",
                "server_options": ["maxconn 4"],
                },
            }
        self.relations_of_type.return_value = [
            {"port": 4242,
             "hostname": "backend.1",
             "private-address": "1.2.3.4",
             "__unit__": "foo/1"},
        ]

        expected = {
            'foo_service': {
                'service_name': 'foo_service',
                'service_host': '0.0.0.0',
                'service_port': 10002,
                'server_options': ["maxconn 4"],
                'servers': [('foo-1-4242', '1.2.3.4',
                             4242, ["maxconn 4"])],
                },
            }
        self.assertEqual(expected, hooks.create_services())
        self.write_service_config.assert_called_with(expected)

    def test_with_sitenames_match_service_name(self):
        self.get_config_services.return_value = {
            None: {
                "service_name": "service",
                },
            "foo_srv": {
                "service_name": "foo_srv",
                "server_options": ["maxconn 4"],
                },
            }
        self.relations_of_type.return_value = [
            {"port": 4242,
             "hostname": "backend.1",
             "sitenames": "foo_srv bar_srv",
             "private-address": "1.2.3.4",
             "__unit__": "foo/0"},
        ]

        expected = {
            'foo_srv': {
                'service_name': 'foo_srv',
                'service_host': '0.0.0.0',
                'service_port': 10002,
                'server_options': ["maxconn 4"],
                'servers': [('foo-0-4242', '1.2.3.4',
                             4242, ["maxconn 4"])],
                },
            }
        self.assertEqual(expected, hooks.create_services())
        self.write_service_config.assert_called_with(expected)

    def test_with_juju_services_match_service_name(self):
        self.get_config_services.return_value = {
            None: {
                "service_name": "service",
                },
            "foo_service": {
                "service_name": "foo_service",
                "server_options": ["maxconn 4"],
                },
            }
        self.relations_of_type.return_value = [
            {"port": 4242,
             "hostname": "backend.1",
             "private-address": "1.2.3.4",
             "__unit__": "foo/1"},
        ]

        expected = {
            'foo_service': {
                'service_name': 'foo_service',
                'service_host': '0.0.0.0',
                'service_port': 10002,
                'server_options': ["maxconn 4"],
                'servers': [('foo-1-4242', '1.2.3.4',
                             4242, ["maxconn 4"])],
                },
            }

        result = hooks.create_services()

        self.assertEqual(expected, result)
        self.write_service_config.assert_called_with(expected)

    def test_with_sitenames_no_match_but_unit_name(self):
        self.get_config_services.return_value = {
            None: {
                "service_name": "service",
                },
            "foo": {
                "service_name": "foo",
                "server_options": ["maxconn 4"],
                },
            }
        self.relations_of_type.return_value = [
            {"port": 4242,
             "hostname": "backend.1",
             "sitenames": "bar_service baz_service",
             "private-address": "1.2.3.4",
             "__unit__": "foo/0"},
        ]

        expected = {
            'foo': {
                'service_name': 'foo',
                'service_host': '0.0.0.0',
                'service_port': 10002,
                'server_options': ["maxconn 4"],
                'servers': [('foo-0-4242', '1.2.3.4',
                             4242, ["maxconn 4"])],
                },
            }
        self.assertEqual(expected, hooks.create_services())
        self.write_service_config.assert_called_with(expected)

    def test_with_multiple_units_in_relation(self):
        """
        Have multiple units specifying "services" in the relation.
        Make sure data is created correctly with create_services()
        """
        self.get_config_services.return_value = {
            None: {
                "service_name": "service",
                },
            }
        self.relations_of_type.return_value = [
            {"port": 4242,
             "private-address": "1.2.3.4",
             "__unit__": "foo/0",
             "services": yaml.safe_dump([{
                 "service_name": "service",
                 "servers": [('foo-0', '1.2.3.4',
                              4242, ["maxconn 4"])]
                 }])
             },
            {"port": 4242,
             "private-address": "1.2.3.5",
             "__unit__": "foo/1",
             "services": yaml.safe_dump([{
                 "service_name": "service",
                 "servers": [('foo-0', '1.2.3.5',
                              4242, ["maxconn 4"])]
                 }])
             },
        ]

        expected = {
            'service': {
                'service_name': 'service',
                'service_host': '0.0.0.0',
                'service_port': 10002,
                'servers': [
                    ['foo-0', '1.2.3.4', 4242, ["maxconn 4"]],
                    ['foo-0', '1.2.3.5', 4242, ["maxconn 4"]]
                    ]
                },
            }
        self.assertEqual(expected, hooks.create_services())
        self.write_service_config.assert_called_with(expected)

    def test_with_multiple_units_and_backends_in_relation(self):
        """
        Have multiple units specifying "services" in the relation
        using the "backends" option. Make sure data is created correctly
        with create_services()
        """
        self.get_config_services.return_value = {
            None: {
                "service_name": "service",
                },
            }
        self.relations_of_type.return_value = [
            {"port": 4242,
             "private-address": "1.2.3.4",
             "__unit__": "foo/0",
             "services": yaml.safe_dump([{
                 "service_name": "service",
                 "servers": [('foo-0', '1.2.3.4',
                              4242, ["maxconn 4"])],
                 "backends": [
                     {"backend_name": "foo-bar",
                      "servers": [('foo-bar-0', '2.2.2.2',
                                   2222, ["maxconn 4"])],
                      },
                 ]
                 }])
             },
            {"port": 4242,
             "private-address": "1.2.3.5",
             "__unit__": "foo/1",
             "services": yaml.safe_dump([{
                 "service_name": "service",
                 "servers": [('foo-0', '1.2.3.5',
                              4242, ["maxconn 4"])],
                 "backends": [
                     {"backend_name": "foo-bar",
                      "servers": [('foo-bar-1', '2.2.2.3',
                                   3333, ["maxconn 4"])],
                      },
                 ]
                 }])
             },
        ]

        expected = {
            'service': {
                'service_name': 'service',
                'service_host': '0.0.0.0',
                'service_port': 10002,
                'servers': [
                    ['foo-0', '1.2.3.4', 4242, ["maxconn 4"]],
                    ['foo-0', '1.2.3.5', 4242, ["maxconn 4"]]
                    ],
                'backends': [
                    {"backend_name": "foo-bar",
                     "servers": [
                         ['foo-bar-0', '2.2.2.2', 2222, ["maxconn 4"]],
                         ['foo-bar-1', '2.2.2.3', 3333, ["maxconn 4"]],
                     ],
                     },
                ]
                },
            }
        self.assertEqual(expected, hooks.create_services())
        self.write_service_config.assert_called_with(expected)

    @patch.dict(os.environ, {"JUJU_UNIT_NAME": "foo/1"})
    def test_with_multiple_units_in_relation_scaleout(self):
        """
        Test multiple units in scaleout mode.
        Ensure no indirection layer gets created.
        """
        self.config_get.return_value["peering_mode"] = "active-active"
        self.get_config_services.return_value = {
            None: {
                "service_name": "service",
                },
            }
        unit_get = self.patch_hook("unit_get")
        unit_get.return_value = "1.2.4.5"
        self.relations_of_type.return_value = [
            {"port": 4242,
             "private-address": "1.2.4.4",
             "__unit__": "foo/0",
             "services": yaml.safe_dump([{
                 "service_name": "service",
                 "servers": [('foo-0', '1.2.3.4',
                              4242, ["maxconn 4"])]
                 }])
             },

            {"__unit__": "foo/1",
             "hostname": "foo-1",
             "private-address": "1.2.4.4",
             "all_services": yaml.dump([
                 {"service_name": "service",
                  "service_host": "0.0.0.0",
                  "service_options": ["balance leastconn"],
                  "service_port": 4242},
                 ])
             },
        ]

        expected = {
            'service': {
                'service_name': 'service',
                'service_host': '0.0.0.0',
                'service_port': 10002,
                'servers': [
                    ['foo-0', '1.2.3.4', 4242, ["maxconn 4"]],
                    ]
                },
            }
        self.assertEqual(expected, hooks.create_services())
        self.write_service_config.assert_called_with(expected)

    def test_merge_service(self):
        """ Make sure merge_services maintains "server" entries. """
        s1 = {'service_name': 'f', 'servers': [['f', '4', 4, ['maxconn 4']]]}
        s2 = {'service_name': 'f', 'servers': [['f', '5', 5, ['maxconn 4']]]}

        expected = {'service_name': 'f', 'servers': [
            ['f', '4', 4, ['maxconn 4']],
            ['f', '5', 5, ['maxconn 4']]]}

        self.assertEqual(expected, hooks.merge_service(s1, s2))

    def test_merge_service_removes_duplicates(self):
        """
        Make sure merge services strips strict duplicates from the
        'servers' entries.
        """
        s1 = {'servers': [['f', '4', 4, ['maxconn 4']]]}
        s2 = {'servers': [['f', '4', 4, ['maxconn 4']]]}
        expected = {'servers': [['f', '4', 4, ['maxconn 4']]]}
        self.assertEqual(expected, hooks.merge_service(s1, s2))

    def test_merge_service_merge_order(self):
        """ Make sure merge_services prefers the left side. """
        s1 = {'service_name': 'left', 'foo': 'bar'}
        s2 = {'service_name': 'right', 'bar': 'baz'}

        expected = {'service_name': 'left', 'foo': 'bar', 'bar': 'baz'}
        self.assertEqual(expected, hooks.merge_service(s1, s2))

    def test_merge_service_old_backend_without_name(self):
        """Backends in old_service without name raise an exception."""

        s1 = {'backends': [{'servers': []}]}
        s2 = {'backends': []}
        self.assertRaises(
            hooks.InvalidRelationDataError, hooks.merge_service, s1, s2)

    def test_merge_service_new_backend_without_name(self):
        """Backends in new_service without name raise an exception."""

        s1 = {'backends': []}
        s2 = {'backends': [{'servers': []}]}
        self.assertRaises(
            hooks.InvalidRelationDataError, hooks.merge_service, s1, s2)

    def test_merge_service_no_old_backend(self):
        """
        If the old service config has no backends, the backends from the
        new config is used..
        """

        s1 = {}
        s2 = {'backends': [
            {'backend_name': 'webapp',
             'servers': [['webapp-1', '10.0.0.2', 8090, []]]},
        ]}
        self.assertEqual(s2, hooks.merge_service(s1, s2))

    def test_merge_service_no_new_backend(self):
        """
        If the new service config has no backends, the backends from the
        old config is used..
        """

        s1 = {'backends': [
            {'backend_name': 'webapp',
             'servers': [['webapp-1', '10.0.0.2', 8090, []]]},
        ]}
        s2 = {}
        self.assertEqual(s1, hooks.merge_service(s1, s2))

    def test_merge_service_backend_name_matching(self):
        """Backends are merged by backend_name."""

        s1 = {'backends': [
            {'backend_name': 'api',
             'servers': [['api-0', '10.0.0.1', 9080, []]]},
            {'backend_name': 'webapp',
             'servers': [['webapp-0', '10.0.0.1', 8090, []]]},
        ]}
        s2 = {'backends': [
            {'backend_name': 'webapp',
             'servers': [['webapp-1', '10.0.0.2', 8090, []]]},
        ]}
        expected = {
            'backends': [
                {'backend_name': 'api',
                 'servers': [['api-0', '10.0.0.1', 9080, []]]},
                {'backend_name': 'webapp',
                 'servers': [['webapp-0', '10.0.0.1', 8090, []],
                             ['webapp-1', '10.0.0.2', 8090, []]]},
            ]
        }
        self.assertEqual(expected, hooks.merge_service(s1, s2))

    def test_join_reverseproxy_relation(self):
        """
        When haproxy joins a reverseproxy relation it advertises its public
        IP and public certificate by setting values on the relation.
        """
        ssl_cert = base64.b64encode("<cert data>")
        self.config_get.return_value = {"ssl_cert": ssl_cert}
        unit_get = self.patch_hook("unit_get")
        unit_get.return_value = "1.2.3.4"
        relation_id = self.patch_hook("relation_id")
        relation_id.return_value = "reverseproxy:1"
        relation_set = self.patch_hook("relation_set")
        hooks.reverseproxy_interface(hook_name="joined")
        unit_get.assert_called_once_with("public-address")
        relation_set.assert_called_once_with(
            relation_id="reverseproxy:1",
            relation_settings={
                "public-address": "1.2.3.4",
                "ssl_cert": ssl_cert})

    def test_join_reverseproxy_relation_with_selfsigned_cert(self):
        """
        When haproxy joins a reverseproxy relation and a self-signed
        certificate is configured, then it's included in the relation.
        """
        self.config_get.return_value = {"ssl_cert": "SELFSIGNED"}
        unit_get = self.patch_hook("unit_get")
        unit_get.return_value = "1.2.3.4"
        relation_id = self.patch_hook("relation_id")
        relation_id.return_value = "reverseproxy:1"
        get_selfsigned_cert = self.patch_hook("get_selfsigned_cert")
        get_selfsigned_cert.return_value = ("<self-signed>", None)
        relation_set = self.patch_hook("relation_set")
        hooks.reverseproxy_interface(hook_name="joined")
        unit_get.assert_called_once_with("public-address")
        ssl_cert = base64.b64encode("<self-signed>")
        relation_set.assert_called_once_with(
            relation_id="reverseproxy:1",
            relation_settings={
                "public-address": "1.2.3.4",
                "ssl_cert": ssl_cert})
