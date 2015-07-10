from __future__ import print_function

__metaclass__ = type

import json
import re
from base_asses import toUnitTest
from jujupy import yaml_loads

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
        # expected entity storage
        self._machines = dict()
        self._services = dict()
        self._units = dict()

        self.parse()

    def parse(self):
        return self._parse()

    @toUnitTest
    def store(self, tc, parsed):
        # Either there are less items than expected and therefore status
        # is returning a subset of what it should or there are more and
        # this means there are untested keys in status.
        tc.assertItemsEqual(parsed.keys(), self._expected,
                            "untested items or incomplete status output")

        # status of machines.
        for machine_id, machine in parsed.get("machines", {}).iteritems():
            tc.assertNotIn(machine_id, self._machines,
                           "Machine %s is repeated in yaml"
                           " status " % machine_id)
            self._machines[machine_id] = machine

        # status of services
        for service_name, service in parsed.get("services", {}).iteritems():
            tc.assertNotIn(service_name, self._services,
                           "Service %s is repeated in yaml "
                           "status " % service_name)
            self._services[service_name] = service

        # status of units
        for service_name, service in self._services.iteritems():
            for unit_name, unit in service.get("units", {}).iteritems():
                tc.assertNotIn(unit_name, self._units,
                               "Unit %s is repeated in yaml "
                               "status " % unit_name)
                self._units[unit_name] = unit

    @toUnitTest
    def assert_machines_len(self, tc, expected_len):
        """Assert that we got as many machines as we where expecting.

        :param expected_len: expected quantity of machines.
        :type expected_len: int
        """
        tc.assertEqual(len(self._machines), expected_len)

    @toUnitTest
    def assert_machines_ids(self, tc, expected_ids):
        """Assert that we got the machines we where expecting.

        :param expected_ids: expected ids of machines.
        :type expected_ids: tuple
        """
        tc.assertItemsEqual(self._machines, expected_ids)

    def _machine_key_get(self, tc, machine_id, key):
        tc.assertIn(machine_id, self._machines,
                    "Machine \"%s\" not present in machines" % machine_id)
        tc.assertIn(key, self._machines[machine_id],
                    "Key \"%s\" not present in Machine \"%s\"" %
                    (key, machine_id))
        return self._machines[machine_id][key]

    @toUnitTest
    def assert_machine_agent_state(self, tc, machine_id, state):
        value = self._machine_key_get(tc, machine_id, AGENT_STATE_KEY)
        tc.assertEqual(value, state)

    @toUnitTest
    def assert_machine_agent_version(self, tc, machine_id, version):
        value = self._machine_key_get(tc, machine_id, AGENT_VERSION_KEY)
        tc.assertEqual(value, version)

    @toUnitTest
    def assert_machine_dns_name(self, tc, machine_id, dns_name):
        value = self._machine_key_get(tc, machine_id, DNS_NAME_KEY)
        tc.assertEqual(value, dns_name)

    @toUnitTest
    def assert_machine_instance_id(self, tc, machine_id, instance_id):
        value = self._machine_key_get(tc, machine_id, INSTANCE_ID_KEY)
        tc.assertEqual(value, instance_id)

    @toUnitTest
    def assert_machine_series(self, tc, machine_id, series):
        value = self._machine_key_get(tc, machine_id, SERIES_KEY)
        tc.assertEqual(value, series)

    @toUnitTest
    def assert_machine_hardware(self, tc, machine_id, hardware):
        value = self._machine_key_get(tc, machine_id, HARDWARE_KEY)
        tc.assertEqual(value, hardware)

    @toUnitTest
    def assert_machine_member_status(self, tc, machine_id, member_status):
        value = self._machine_key_get(tc, machine_id,
                                      STATE_SERVER_MEMBER_STATUS_KEY)
        tc.assertEqual(value, member_status)

    def _service_key_get(self, tc, service_name, key):
        tc.assertIn(service_name, self._services,
                    "Service \"%s\" not present in services." % service_name)
        tc.assertIn(key, self._services[service_name],
                    "Key \"%s\" not present in Service \"%s\"" %
                    (key, service_name))
        return self._services[service_name][key]

    # Service status
    @toUnitTest
    def assert_service_charm(self, tc, service_name, charm):
        value = self._service_key_get(tc, service_name, CHARM_KEY)
        tc.assertEqual(value, charm)

    @toUnitTest
    def assert_service_exposed(self, tc, service_name, exposed):
        value = self._service_key_get(tc, service_name, EXPOSED_KEY)
        tc.assertEqual(value, exposed)

    @toUnitTest
    def assert_service_service_status(self, tc, service_name,
                                      status={"current": "", "message": ""}):
        value = self._service_key_get(tc, service_name, SERVICE_STATUS_KEY)
        tc.assertEqual(value["current"], status["current"])
        tc.assertEqual(value["message"], status["message"])

    def _unit_key_get(self, tc, unit_name, key):
        tc.assertIn(unit_name, self._units,
                    "Unit \"%s\" not present in units" % unit_name)
        tc.assertIn(key, self._units[unit_name],
                    "Key \"%s\" not present in Unit \"%s\"" %
                    (key, unit_name))
        return self._units[unit_name][key]

    # Units status
    @toUnitTest
    def assert_unit_workload_status(self, tc, unit_name,
                                    status={"current": "", "message": ""}):
        value = self._unit_key_get(tc, unit_name, WORKLOAD_STATUS_KEY)
        tc.assertEqual(value["current"], status["current"])
        tc.assertEqual(value["message"], status["message"])

    @toUnitTest
    def assert_unit_agent_status(self, tc, unit_name,
                                 status={"current": "", "message": ""}):
        value = self._unit_key_get(tc, unit_name, AGENT_STATUS_KEY)
        tc.assertEqual(value["current"], status["current"])
        # Message is optional for unit agents.
        tc.assertEqual(value.get("message", ""), status["message"])

    @toUnitTest
    def assert_unit_agent_state(self, tc, unit_name, state):
        value = self._unit_key_get(tc, unit_name, AGENT_STATE_KEY)
        tc.assertEqual(value, state)

    @toUnitTest
    def assert_unit_agent_version(self, tc, unit_name, version):
        value = self._unit_key_get(tc, unit_name, AGENT_VERSION_KEY)
        tc.assertEqual(value, version)

    @toUnitTest
    def assert_unit_machine(self, tc, unit_name, machine):
        value = self._unit_key_get(tc, unit_name, MACHINE_KEY)
        tc.assertEqual(value, machine)

    @toUnitTest
    def assert_unit_public_address(self, tc, unit_name, address):
        value = self._unit_key_get(tc, unit_name, PUBLIC_ADDRESS_KEY)
        tc.assertEqual(value, address)


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

    @toUnitTest
    def _normalize_machines(self, tc, header, items):
        nitems = items[:6]
        nitems.append(" ".join(items[6:]))
        tc.assertEqual(header, MACHINE_TAB_HEADERS,
                       "Unexpected headers for machine:\n"
                       "wanted: %s"
                       "got: %s" % (MACHINE_TAB_HEADERS, header))
        return nitems[0], dict(zip((AGENT_STATE_KEY, AGENT_VERSION_KEY,
                                    DNS_NAME_KEY, INSTANCE_ID_KEY,
                                    SERIES_KEY, HARDWARE_KEY),
                               nitems[1:]))

    @toUnitTest
    def _normalize_units(self, tc, header, items):
        eid, wlstate, astate, version, machine, paddress = items[:6]
        message = " ".join(items[6:])
        wlstatus = {"current": wlstate, "message": message,
                    "since": "bogus date"}
        astatus = {"current": astate, "message": "", "since": "bogus date"}
        tc.assertEqual(header, UNIT_TAB_HEADERS,
                       "Unexpected headers for unit.\n"
                       "wanted: %s"
                       "got: %s" % (UNIT_TAB_HEADERS, header))
        return eid, dict(zip((WORKLOAD_STATUS_KEY, AGENT_STATUS_KEY,
                              AGENT_VERSION_KEY, MACHINE_KEY,
                              PUBLIC_ADDRESS_KEY),
                             (wlstatus, astatus, version, machine, paddress)))

    @toUnitTest
    def _normalize_services(self, tc, header, items):
        name, status, exposed, charm = items
        tc.assertEqual(header, SERVICE_TAB_HEADERS,
                       "Unexpected headers for service.\n"
                       "wanted: %s"
                       "got: %s" % (SERVICE_TAB_HEADERS, header))

        return name, dict(zip((CHARM_KEY, EXPOSED_KEY, SERVICE_STATUS_KEY),
                              (charm, exposed == "true", {"current": status,
                               "message": ""})))

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
