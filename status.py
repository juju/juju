from __future__ import print_function

import json
import re
from unittest import FunctionTestCase

from jujupy import yaml_loads


__metaclass__ = type


# Machine and unit, deprecated in unit
AGENT_STATE_KEY = "agent-state"
AGENT_VERSION_KEY = "agent-version"
DNS_NAME_KEY = "dns-name"
INSTANCE_ID_KEY = "instance-id"
HARDWARE_KEY = "hardware"
SERIES_KEY = "series"
STATE_SERVER_MEMBER_STATUS_KEY = "state-server-member-status"
# service
CHARM_KEY = "charm"
EXPOSED_KEY = "exposed"
SERVICE_STATUS_KEY = "service-status"
# unit
WORKLOAD_STATUS_KEY = "workload-status"
AGENT_STATUS_KEY = "agent-status"
MACHINE_KEY = "machine"
PUBLIC_ADDRESS_KEY = "public-address"

MACHINE_TAB_HEADERS = ['ID', 'STATE', 'VERSION', 'DNS', 'INS-ID', 'SERIES',
                       'HARDWARE']
UNIT_TAB_HEADERS = ['ID', 'WORKLOAD-STATE', 'AGENT-STATE', 'VERSION',
                    'MACHINE', 'PORTS', 'PUBLIC-ADDRESS', 'MESSAGE']
SERVICE_TAB_HEADERS = ['NAME', 'STATUS', 'EXPOSED', 'CHARM']


class StatusTester:
    def __init__(self, text="", status_format="yaml"):
        self._text = text
        self._format = status_format
        self.s = globals()["Status%sParser" % status_format.capitalize()](text)

    @classmethod
    def from_text(cls, text, status_format):
        return cls(text, status_format)

    def __unicode__(self):
        return self._text

    def __str__(self):
        return self._text


class ErrNoStatus(Exception):
    """An exception for missing juju status."""


class ErrMalformedStatus(Exception):
    """An exception for unexpected formats of status."""


class ErrUntestedStatusOutput(Exception):
    """An exception for results returned by status.

    Status that are known not to be covered by the tests should raise
    this exception. """


class BaseStatusParser:

    _expected = set(["environment", "services", "machines"])

    def __init__(self):
        self.tc = FunctionTestCase("", description=self.__class__.__name__)
        # expected entity storage
        self._machines = dict()
        self._services = dict()
        self._units = dict()

        self.parse()

    def parse(self):
        return self._parse()

    def store(self, parsed):
        # Either there are less items than expected and therefore status
        # is returning a subset of what it should or there are more and
        # this means there are untested keys in status.
        self.tc.assertItemsEqual(parsed.keys(), self._expected,
                                 "untested items or incomplete status output")

        # status of machines.
        for machine_id, machine in parsed.get("machines", {}).iteritems():
            self.tc.assertNotIn(machine_id, self._machines,
                                "Machine %s is repeated in yaml"
                                " status " % machine_id)
            self._machines[machine_id] = machine

        # status of services
        for service_name, service in parsed.get("services", {}).iteritems():
            self.tc.assertNotIn(service_name, self._services,
                                "Service %s is repeated in yaml "
                                "status " % service_name)
            self._services[service_name] = service

        # status of units
        for service_name, service in self._services.iteritems():
            for unit_name, unit in service.get("units", {}).iteritems():
                self.tc.assertNotIn(unit_name, self._units,
                                    "Unit %s is repeated in yaml "
                                    "status " % unit_name)
                self._units[unit_name] = unit

    def assert_machines_len(self, expected_len):
        """Assert that we got as many machines as we where expecting.

        :param expected_len: expected quantity of machines.
        :type expected_len: int
        """
        self.tc.assertEqual(len(self._machines), expected_len)

    def assert_machines_ids(self, expected_ids):
        """Assert that we got the machines we where expecting.

        :param expected_ids: expected ids of machines.
        :type expected_ids: tuple
        """
        self.tc.assertItemsEqual(self._machines, expected_ids)

    def _machine_key_get(self, machine_id, key):
        self.tc.assertIn(machine_id, self._machines,
                         "Machine \"%s\" not present in machines" % machine_id)
        self.tc.assertIn(key, self._machines[machine_id],
                         "Key \"%s\" not present in Machine \"%s\"" %
                         (key, machine_id))
        return self._machines[machine_id][key]

    def assert_machine_agent_state(self, machine_id, state):
        value = self._machine_key_get(machine_id, AGENT_STATE_KEY)
        self.tc.assertEqual(value, state)

    def assert_machine_agent_version(self, machine_id, version):
        value = self._machine_key_get(machine_id, AGENT_VERSION_KEY)
        self.tc.assertEqual(value, version)

    def assert_machine_dns_name(self, machine_id, dns_name):
        value = self._machine_key_get(machine_id, DNS_NAME_KEY)
        self.tc.assertEqual(value, dns_name)

    def assert_machine_instance_id(self, machine_id, instance_id):
        value = self._machine_key_get(machine_id, INSTANCE_ID_KEY)
        self.tc.assertEqual(value, instance_id)

    def assert_machine_series(self, machine_id, series):
        value = self._machine_key_get(machine_id, SERIES_KEY)
        self.tc.assertEqual(value, series)

    def assert_machine_hardware(self, machine_id, hardware):
        value = self._machine_key_get(machine_id, HARDWARE_KEY)
        self.tc.assertEqual(value, hardware)

    def assert_machine_member_status(self, machine_id, member_status):
        value = self._machine_key_get(machine_id,
                                      STATE_SERVER_MEMBER_STATUS_KEY)
        self.tc.assertEqual(value, member_status)

    def _service_key_get(self, service_name, key):
        self.tc.assertIn(service_name, self._services,
                         "Service \"%s\" not present in services." %
                         service_name)
        self.tc.assertIn(key, self._services[service_name],
                         "Key \"%s\" not present in Service \"%s\"" %
                         (key, service_name))
        return self._services[service_name][key]

    # Service status
    def assert_service_charm(self, service_name, charm):
        value = self._service_key_get(service_name, CHARM_KEY)
        self.tc.assertEqual(value, charm)

    def assert_service_exposed(self, service_name, exposed):
        value = self._service_key_get(service_name, EXPOSED_KEY)
        self.tc.assertEqual(value, exposed)

    def assert_service_service_status(self, service_name,
                                      status={"current": "", "message": ""}):
        value = self._service_key_get(service_name, SERVICE_STATUS_KEY)
        self.tc.assertEqual(value["current"], status["current"])
        self.tc.assertEqual(value["message"], status["message"])

    def _unit_key_get(self, unit_name, key):
        self.tc.assertIn(unit_name, self._units,
                         "Unit \"%s\" not present in units" % unit_name)
        self.tc.assertIn(key, self._units[unit_name],
                         "Key \"%s\" not present in Unit \"%s\"" %
                         (key, unit_name))
        return self._units[unit_name][key]

    # Units status
    def assert_unit_workload_status(self, unit_name,
                                    status={"current": "", "message": ""}):
        value = self._unit_key_get(unit_name, WORKLOAD_STATUS_KEY)
        self.tc.assertEqual(value["current"], status["current"])
        self.tc.assertEqual(value["message"], status["message"])

    def assert_unit_agent_status(self, unit_name,
                                 status={"current": "", "message": ""}):
        value = self._unit_key_get(unit_name, AGENT_STATUS_KEY)
        self.tc.assertEqual(value["current"], status["current"])
        # Message is optional for unit agents.
        self.tc.assertEqual(value.get("message", ""), status["message"])

    def assert_unit_agent_state(self, unit_name, state):
        value = self._unit_key_get(unit_name, AGENT_STATE_KEY)
        self.tc.assertEqual(value, state)

    def assert_unit_agent_version(self, unit_name, version):
        value = self._unit_key_get(unit_name, AGENT_VERSION_KEY)
        self.tc.assertEqual(value, version)

    def assert_unit_machine(self, unit_name, machine):
        value = self._unit_key_get(unit_name, MACHINE_KEY)
        self.tc.assertEqual(value, machine)

    def assert_unit_public_address(self, unit_name, address):
        value = self._unit_key_get(unit_name, PUBLIC_ADDRESS_KEY)
        self.tc.assertEqual(value, address)


class StatusYamlParser(BaseStatusParser):
    """StatusYamlParser handles parsing of status output in yaml format.

    To be used by status tester.
    """

    def __init__(self, yaml=""):
        self._yaml = yaml
        if yaml == "":
            raise ErrNoStatus("Yaml status was empty")
        super(StatusYamlParser, self).__init__()

    def _parse(self):
        parsed = yaml_loads(self._yaml)
        self.store(parsed)


class StatusJsonParser(BaseStatusParser):
    """StatusJSONParser handles parsing of status output in JSON format.

    To be used by status tester.
    """

    def __init__(self, json_text=""):
        self._json = json_text
        if json_text == "":
            raise ErrNoStatus("JSON status was empty")
        super(StatusJsonParser, self).__init__()

    def _parse(self):
        parsed = json.loads(self._json)
        self.store(parsed)


class StatusTabularParser(BaseStatusParser):
    """StatusTabularParser handles parsing of status output in Tabular format.

    To be used by status tester.
    """

    def __init__(self, tabular_text=""):
        self._tabular = tabular_text
        if tabular_text == "":
            raise ErrNoStatus("tabular status was empty")
        super(StatusTabularParser, self).__init__()

    def _normalize_machines(self, header, items):
        nitems = items[:6]
        nitems.append(" ".join(items[6:]))
        self.tc.assertEqual(header, MACHINE_TAB_HEADERS,
                            "Unexpected headers for machine:\n"
                            "wanted: %s"
                            "got: %s" % (MACHINE_TAB_HEADERS, header))
        normalized = dict(zip((AGENT_STATE_KEY, AGENT_VERSION_KEY,
                               DNS_NAME_KEY, INSTANCE_ID_KEY,
                               SERIES_KEY, HARDWARE_KEY),
                              nitems[1:]))
        return nitems[0], normalized

    def _normalize_units(self, header, items):
        eid, wlstate, astate, version, machine, paddress = items[:6]
        message = " ".join(items[6:])
        wlstatus = {"current": wlstate, "message": message,
                    "since": "bogus date"}
        astatus = {"current": astate, "message": "", "since": "bogus date"}
        self.tc.assertEqual(header, UNIT_TAB_HEADERS,
                            "Unexpected headers for unit.\n"
                            "wanted: %s"
                            "got: %s" % (UNIT_TAB_HEADERS, header))
        normalized = dict(zip((WORKLOAD_STATUS_KEY, AGENT_STATUS_KEY,
                              AGENT_VERSION_KEY, MACHINE_KEY,
                              PUBLIC_ADDRESS_KEY),
                              (wlstatus, astatus, version, machine, paddress)))

        return eid, normalized

    def _normalize_services(self, header, items):
        name, status, exposed, charm = items
        self.tc.assertEqual(header, SERVICE_TAB_HEADERS,
                            "Unexpected headers for service.\n"
                            "wanted: %s"
                            "got: %s" % (SERVICE_TAB_HEADERS, header))
        normalized = dict(zip((CHARM_KEY, EXPOSED_KEY, SERVICE_STATUS_KEY),
                              (charm, exposed == "true", {"current": status,
                               "message": ""})))
        return name, normalized

    def _parse(self):
        section = re.compile("^\[(\w*)\]")
        base = {"environment": "not provided"}
        current_parent = ""
        current_headers = []
        prev_was_section = False
        for line in self._tabular.splitlines():
            # parse section
            is_section = section.findall(line)
            if len(is_section) == 1:
                current_parent = is_section[0].lower()
                if current_parent != "units":
                    base[current_parent] = {}
                prev_was_section = True
                continue
            # parse headers
            if prev_was_section:
                prev_was_section = False
                current_headers = line.split()
                continue

            # parse content
            if current_parent == "" or current_headers == []:
                raise ErrMalformedStatus("Tabular status is malformed")
            items = line.split()

            # separation line
            if len(items) == 0:
                continue

            normalize = None
            if current_parent == "services":
                normalize = self._normalize_services
            elif current_parent == "units":
                normalize = self._normalize_units
            elif current_parent == "machines":
                normalize = self._normalize_machines

            if not normalize:
                raise ErrUntestedStatusOutput("%s is not an expected tabular"
                                              " status section" %
                                              current_parent)
            k, v = normalize(current_headers, items)
            if current_parent == "units":
                base.setdefault("services", dict())
                service = k.split("/")[0]
                base["services"][service].setdefault("units", dict())
                base["services"][service]["units"][k] = v
            else:
                base[current_parent][k] = v
        self.store(base)
