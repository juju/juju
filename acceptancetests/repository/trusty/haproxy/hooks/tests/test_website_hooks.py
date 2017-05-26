from testtools import TestCase
from mock import patch, call

import hooks


class WebsiteRelationTest(TestCase):

    def setUp(self):
        super(WebsiteRelationTest, self).setUp()
        self.notify_website = self.patch_hook("notify_website")

    def patch_hook(self, hook_name):
        mock_controller = patch.object(hooks, hook_name)
        mock = mock_controller.start()
        self.addCleanup(mock_controller.stop)
        return mock

    def test_website_interface_none(self):
        self.assertEqual(None, hooks.website_interface(hook_name=None))
        self.notify_website.assert_not_called()

    def test_website_interface_joined(self):
        hooks.website_interface(hook_name="joined")
        self.notify_website.assert_called_once_with(
            changed=False, relation_ids=(None,))

    def test_website_interface_changed(self):
        hooks.website_interface(hook_name="changed")
        self.notify_website.assert_called_once_with(
            changed=True, relation_ids=(None,))


class NotifyRelationTest(TestCase):

    def setUp(self):
        super(NotifyRelationTest, self).setUp()

        self.relations_of_type = self.patch_hook("relations_of_type")
        self.relations_for_id = self.patch_hook("relations_for_id")
        self.relation_set = self.patch_hook("relation_set")
        self.config_get = self.patch_hook("config_get")
        self.get_relation_ids = self.patch_hook("get_relation_ids")
        self.get_hostname = self.patch_hook("get_hostname")
        self.log = self.patch_hook("log")
        self.get_config_service = self.patch_hook("get_config_service")

    def patch_hook(self, hook_name):
        mock_controller = patch.object(hooks, hook_name)
        mock = mock_controller.start()
        self.addCleanup(mock_controller.stop)
        return mock

    def test_notify_website_relation_no_relation_ids(self):
        hooks.notify_relation("website")
        self.get_relation_ids.return_value = ()
        self.relation_set.assert_not_called()
        self.get_relation_ids.assert_called_once_with("website")

    def test_notify_website_relation_with_default_relation(self):
        self.get_relation_ids.return_value = ()
        self.get_hostname.return_value = "foo.local"
        self.relations_for_id.return_value = [{}]
        self.config_get.return_value = {"services": ""}

        hooks.notify_relation("website", relation_ids=(None,))

        self.get_hostname.assert_called_once_with()
        self.relations_for_id.assert_called_once_with(None)
        self.relation_set.assert_called_once_with(
            relation_id=None, port="80", hostname="foo.local",
            all_services="")
        self.get_relation_ids.assert_not_called()

    def test_notify_website_relation_with_relations(self):
        self.get_relation_ids.return_value = ("website:1",
                                              "website:2")
        self.get_hostname.return_value = "foo.local"
        self.relations_for_id.return_value = [{}]
        self.config_get.return_value = {"services": ""}

        hooks.notify_relation("website")

        self.get_hostname.assert_called_once_with()
        self.get_relation_ids.assert_called_once_with("website")
        self.relations_for_id.assert_has_calls([
            call("website:1"),
            call("website:2"),
            ])

        self.relation_set.assert_has_calls([
            call(relation_id="website:1", port="80", hostname="foo.local",
                 all_services=""),
            call(relation_id="website:2", port="80", hostname="foo.local",
                 all_services=""),
            ])

    def test_notify_website_relation_with_different_sitenames(self):
        self.get_relation_ids.return_value = ("website:1",)
        self.get_hostname.return_value = "foo.local"
        self.relations_for_id.return_value = [{"service_name": "foo"},
                                              {"service_name": "bar"}]
        self.config_get.return_value = {"services": ""}

        hooks.notify_relation("website")

        self.get_hostname.assert_called_once_with()
        self.get_relation_ids.assert_called_once_with("website")
        self.relations_for_id.assert_has_calls([
            call("website:1"),
            ])

        self.relation_set.assert_has_calls([
            call.relation_set(
                relation_id="website:1", port="80", hostname="foo.local",
                all_services=""),
            ])
        self.log.assert_has_calls([
            call.log(
                "Remote units requested more than a single service name."
                "Falling back to default host/port."),
            call.log("No services configured, exiting."),
            ])

    def test_notify_website_relation_with_same_sitenames(self):
        self.get_relation_ids.return_value = ("website:1",)
        self.get_hostname.side_effect = ["foo.local", "bar.local"]
        self.relations_for_id.return_value = [{"service_name": "bar"},
                                              {"service_name": "bar"}]
        self.config_get.return_value = {"services": ""}
        self.get_config_service.return_value = {"service_host": "bar.local",
                                                "service_port": "4242"}

        hooks.notify_relation("website")

        self.get_hostname.assert_has_calls([
            call(),
            call("bar.local")])
        self.get_relation_ids.assert_called_once_with("website")
        self.relations_for_id.assert_has_calls([
            call("website:1"),
            ])

        self.relation_set.assert_has_calls([
            call.relation_set(
                relation_id="website:1", port="4242", hostname="bar.local",
                all_services=""),
            ])
        self.log.assert_has_calls([call("No services configured, exiting.")])
