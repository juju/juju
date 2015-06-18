from __future__ import print_function

__metaclass__ = type

import json
import re
from jujupy import yaml_loads
from base_asses import assertion_test

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

NO_PRESENT_STR = "%s key is not present in %s status"


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
    """An exception for missing juju status"""


class ErrMalformedStatus(Exception):
    """An exception for unexpected formats of status"""


class ErrUntestedStatusOutput(Exception):
    """An exception for results returned by status that are known not to be
    covered by the tests"""


def untested(obtained, expected):
    """Fail according to the issue tahta caused the expected vs obtained
    missmatch

    :param obtained: obtained status entities.
    :type obtained: list
    :param expected: expected status entities.
    :type expected: list
    :returns: exeption that caused the missmatch.
    :rtype: Exception
    """
    obtained = set(obtained)
    expected = set(expected)

    obtained_diff = obtained.difference(expected)
    expected_diff = expected.difference(obtained)

    if obtained_diff:
        return ErrUntestedStatusOutput("status returned elements that"
                                       " are not covered by this test: %s"
                                       % ", ".join(obtained_diff))
    else:
        return ErrMalformedStatus("status returned less elements than expected"
                                  "by this test, the missing are: %s"
                                  % ", ".join(expected_diff))


class BaseStatusParser:

    _expected = set(["environment", "services", "machines"])

    def __init__(self):
        super(BaseStatusParser, self).__init__()
        # expected entity storage
        self._machines = dict()
        self._services = dict()
        self._units = dict()

        self.parse()

    def parse(self):
        return self._parse()

    def store(self, parsed):
        if set(parsed.keys()) != self._expected:
            raise untested(parsed.keys(), self._expected)

        # status of machines.
        for machine_id, machine in parsed.get("machines", {}).iteritems():
            if machine_id in self._machines:
                raise ErrMalformedStatus("Machine %s is repeated in yaml"
                                         " status " % machine_id)
            self._machines[machine_id] = machine

        # status of services
        for service_name, service in parsed.get("services", {}).iteritems():
            if service_name in self._services:
                raise ErrMalformedStatus("Service %s is repeated in yaml "
                                         "status " % service_name)
            self._services[service_name] = service

        # status of units
        for service_name, service in self._services.iteritems():
            for unit_name, unit in service.get("units", {}).iteritems():
                if unit_name in self._units:
                    raise ErrMalformedStatus("Unit %s is repeated in yaml "
                                             "status " % unit_name)
                self._units[unit_name] = unit

    @assertion_test
    def assert_machines_len(self, expected_len):
        """Assert that we got as many machines as we where expecting

        :param expected_len: expected quantity of machines.
        :type expected_len: int
        :returns: true if the lenght is the expected, false if not.
        :rtype: bool
        """
        return len(self._machines) == expected_len

    @assertion_test
    def assert_machines_ids(self, expected_ids):
        """Assert that we got the machines we where expecting

        :param expected_ids: expected ids of machines.
        :type expected_ids: tuple
        :returns: true if the elements match, false if not.
        :rtype: bool
        """
        return set(self._machines) == set(expected_ids)

    def _machine_key_get(self, machine_id, key):

        if (machine_id in self._machines) and \
           (key in self._machines[machine_id]):
            return True, self._machines[machine_id][key]
        return False, None

    @assertion_test
    def assert_machine_agent_state(self, machine_id, state):
        present, value = self._machine_key_get(machine_id, AGENT_STATE_KEY)
        if present:
            return value == state
        return NO_PRESENT_STR % (AGENT_STATE_KEY, machine_id)

    @assertion_test
    def assert_machine_agent_version(self, machine_id, version):
        present, value = self._machine_key_get(machine_id, AGENT_VERSION_KEY)
        if present:
            return value == version
        return NO_PRESENT_STR % (AGENT_VERSION_KEY, machine_id)

    @assertion_test
    def assert_machine_dns_name(self, machine_id, dns_name):
        present, value = self._machine_key_get(machine_id, DNS_NAME_KEY)
        if present:
            return value == dns_name
        return NO_PRESENT_STR % (DNS_NAME_KEY, machine_id)

    @assertion_test
    def assert_machine_instance_id(self, machine_id, instance_id):
        present, value = self._machine_key_get(machine_id, INSTANCE_ID_KEY)
        if present:
            return value == instance_id
        return NO_PRESENT_STR % (INSTANCE_ID_KEY, machine_id)

    @assertion_test
    def assert_machine_series(self, machine_id, series):
        present, value = self._machine_key_get(machine_id, SERIES_KEY)
        if present:
            return value == series
        return NO_PRESENT_STR % (SERIES_KEY, machine_id)

    @assertion_test
    def assert_machine_hardware(self, machine_id, hardware):
        present, value = self._machine_key_get(machine_id, HARDWARE_KEY)
        if present:
            return value == hardware
        return NO_PRESENT_STR % (HARDWARE_KEY, machine_id)

    @assertion_test
    def assert_machine_member_status(self, machine_id, member_status):
        present, value = self._machine_key_get(machine_id,
                                               STATE_SERVER_MEMBER_STATUS_KEY)
        if present:
            return value == member_status
        return NO_PRESENT_STR % (STATE_SERVER_MEMBER_STATUS_KEY, machine_id)

    def _service_key_get(self, service_name, key):
        if (service_name in self._services) and \
           (key in self._services[service_name]):
            return True, self._services[service_name][key]
        return False, None

    # Service status
    @assertion_test
    def assert_service_charm(self, service_name, charm):
        present, value = self._service_key_get(service_name, CHARM_KEY)
        if present:
            return value == charm
        return NO_PRESENT_STR % (CHARM_KEY, service_name)

    @assertion_test
    def assert_service_exposed(self, service_name, exposed):
        present, value = self._service_key_get(service_name, EXPOSED_KEY)
        if present:
            return value == exposed
        return NO_PRESENT_STR % (EXPOSED_KEY, service_name)

    @assertion_test
    def assert_service_service_status(self, service_name,
                                      status={"current": "", "message": ""}):
        present, value = self._service_key_get(service_name,
                                               SERVICE_STATUS_KEY)
        if present:
            current = value["current"] == status["current"]
            message = value["message"] == status["message"]
            return current and message
        return NO_PRESENT_STR % (SERVICE_STATUS_KEY, service_name)

    def _unit_key_get(self, unit_name, key):
        if (unit_name in self._units) and \
           (key in self._units[unit_name]):
            return True, self._units[unit_name][key]
        return False, None

    # Units status
    @assertion_test
    def assert_unit_workload_status(self, unit_name,
                                    status={"current": "", "message": ""}):
        present, value = self._unit_key_get(unit_name, WORKLOAD_STATUS_KEY)
        if present:
            current = value["current"] == status["current"]
            message = value["message"] == status["message"]
            return current and message
        return NO_PRESENT_STR % (WORKLOAD_STATUS_KEY, unit_name)

    @assertion_test
    def assert_unit_agent_status(self, unit_name,
                                 status={"current": "", "message": ""}):
        present, value = self._unit_key_get(unit_name, AGENT_STATUS_KEY)
        if present:
            current = value["current"] == status["current"]
            # Message is optional for unit agents.
            message = value.get("message", "") == status["message"]
            return current and message
        return NO_PRESENT_STR % (AGENT_STATUS_KEY, unit_name)

    @assertion_test
    def assert_unit_agent_state(self, unit_name, state):
        present, value = self._unit_key_get(unit_name, AGENT_STATE_KEY)
        if present:
            return value == state
        return NO_PRESENT_STR % (AGENT_STATE_KEY, unit_name)

    @assertion_test
    def assert_unit_agent_version(self, unit_name, version):
        present, value = self._unit_key_get(unit_name, AGENT_VERSION_KEY)
        if present:
            return value == version
        return NO_PRESENT_STR % (AGENT_VERSION_KEY, unit_name)

    @assertion_test
    def assert_unit_machine(self, unit_name, machine):
        present, value = self._unit_key_get(unit_name, MACHINE_KEY)
        if present:
            return value == machine
        return NO_PRESENT_STR % (MACHINE_KEY, unit_name)

    @assertion_test
    def assert_unit_public_address(self, unit_name, address):
        present, value = self._unit_key_get(unit_name, PUBLIC_ADDRESS_KEY)
        if present:
            return value == address
        return NO_PRESENT_STR % (PUBLIC_ADDRESS_KEY, unit_name)


class StatusYamlParser(BaseStatusParser):
    """StatusYamlParser handles parsing of status output in yaml format to be
    used by status tester.
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
    """StatusJSONParser handles parsing of status output in JSON format to be
    used by status tester.
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
    """StatusTabularParser handles parsing of status output in Tabular format
    to be used by status tester.
    """

    def __init__(self, tabular_text=""):
        self._tabular = tabular_text
        if tabular_text == "":
            raise ErrNoStatus("tabular status was empty")
        super(StatusTabularParser, self).__init__()

    def _normalize_machines(self, header, items):
        nitems = items[:6]
        nitems.append(" ".join(items[6:]))
        if header != MACHINE_TAB_HEADERS:
            raise ErrMalformedStatus("Unexpected headers for machine:\n"
                                     "wanted: %s"
                                     "got: %s" % (MACHINE_TAB_HEADERS, header))
        return nitems[0], dict(zip((AGENT_STATE_KEY, AGENT_VERSION_KEY,
                                    DNS_NAME_KEY, INSTANCE_ID_KEY,
                                    SERIES_KEY, HARDWARE_KEY),
                               nitems[1:]))

    def _normalize_units(self, header, items):
        eid, wlstate, astate, version, machine, paddress = items[:6]
        message = " ".join(items[6:])
        wlstatus = {"current": wlstate, "message": message,
                    "since": "bogus date"}
        astatus = {"current": astate, "message": "", "since": "bogus date"}
        if header != UNIT_TAB_HEADERS:
            raise ErrMalformedStatus("Unexpected headers for unit.\n"
                                     "wanted: %s"
                                     "got: %s" % (UNIT_TAB_HEADERS, header))
        return eid, dict(zip((WORKLOAD_STATUS_KEY, AGENT_STATUS_KEY,
                              AGENT_VERSION_KEY, MACHINE_KEY,
                              PUBLIC_ADDRESS_KEY),
                             (wlstatus, astatus, version, machine, paddress)))

    def _normalize_services(self, header, items):
        name, status, exposed, charm = items
        if header != SERVICE_TAB_HEADERS:
            raise ErrMalformedStatus("Unexpected headers for service.\n"
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
