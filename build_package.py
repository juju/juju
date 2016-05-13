#!/usr/bin/python
"""Script for building source and binary debian packages."""

from __future__ import print_function

from argparse import ArgumentParser
from collections import namedtuple
import os
import re
import shutil
import subprocess
import sys

__metaclass__ = type


DEBS_NOT_FOUND = 3

# This constant defines the location of the base source package branch.
DEFAULT_SPB = 'lp:~juju-qa/juju-release-tools/packaging-juju-core-default'
# TODO Merge packaging-juju2-default when juju-ci-tools is ready.
DEFAULT_SPB2 = 'lp:~juju-qa/juju-release-tools/packaging-juju-core2-default'


# This constant defines the status of the series supported by CI and Releases.
SUPPORTED_RELEASES = """\
12.04 precise LTS
12.10 quantal HISTORIC
13.10 saucy HISTORIC
14.04 trusty LTS
14.10 utopic HISTORIC
15.04 vivid HISTORIC
15.10 wily SUPPORTED
16.04 xenial LTS
16.10 yakety DEVEL
"""


SourceFile = namedtuple('SourceFile', ['sha256', 'size', 'name', 'path'])


CREATE_LXC_TEMPLATE = """\
set -eu
sudo lxc-create -t ubuntu-cloud -n {container} -- -r {series} -a {arch}
sudo mkdir /var/lib/lxc/{container}/rootfs/workspace
echo "lxc.mount.entry = {build_dir} workspace none bind 0 0" |
    sudo tee -a /var/lib/lxc/{container}/config
"""


BUILD_DEB_TEMPLATE = """\
sudo lxc-attach -n {container} -- bash <<"EOT"
    set -eu
    echo "\nInstalling common build deps.\n"
    cd workspace
    # Wait for Cloud-init to complete to indicate the machine is in a ready
    # state with network to do work,
    while ! tail -1 /var/log/cloud-init-output.log | \
            egrep -q -i 'Cloud-init .* finished'; do
        echo "Waiting for Cloud-init to finish."
        sleep 5
    done
    set +e
    # The cloud-init breaks arm64, ppc64el. s390x /etc/apt/sources.list.
    if [[ $(dpkg --print-architecture) =~ ^(arm64|ppc64el|s390x)$ ]]; then
        sed -i \
            -e 's,archive.ubuntu.com/ubuntu,ports.ubuntu.com/ubuntu-ports,' \
            /etc/apt/sources.list
    fi
    # Adding the ppa directly to sources.list without the archive key
    # requires apt to be run with --force-yes
    echo "{ppa}" >> /etc/apt/sources.list
    export DEBIAN_FRONTEND=noninteractive
    apt-get update
    apt-get install -y --force-yes build-essential devscripts equivs
EOT
sudo lxc-attach -n {container} -- bash <<"EOT"
    set -eux
    echo "\nInstalling build deps from dsc.\n"
    cd workspace
    export DEBIAN_FRONTEND=noninteractive
    mk-build-deps -i --tool 'apt-get --yes --force-yes' *.dsc
EOT
sudo lxc-attach -n {container} -- bash <<"EOT"
    set -eux
    echo "\nBuilding the packages.\n"
    cd workspace
    rm *build-deps*.deb || true
    dpkg-source -x *.dsc
    cd $(basename *.orig.tar.gz .orig.tar.gz | tr _ -)
    dpkg-buildpackage -us -uc
EOT
"""


CREATE_SPB_TEMPLATE = """\
set -eux
bzr branch {branch} {spb}
cd {spb}
bzr import-upstream {version} {tarfile_path}
bzr merge . -r upstream-{version}
bzr commit -m "Merged upstream-{version}."
"""


BUILD_SOURCE_TEMPLATE = """\
set -eux
bzr branch {spb} {source}
cd {source}
dch --newversion {ubuntu_version} -D {series} --force-distribution "{message}"
debcommit
bzr bd -S -- -us -uc
"""

DEBSIGN_TEMPLATE = 'debsign -p {gpgcmd} *.changes'


UBUNTU_VERSION_TEMPLATE = '{version}-0ubuntu1~{release}.{upatch}~juju1'
DAILY_VERSION_TEMPLATE = '{version}-{date}+{build}+{revid}~{release}'


VERSION_PATTERN = re.compile('(\d+)\.(\d+)\.(\d+)')


Series = namedtuple('Series', ['version', 'name', 'status'])


class _JujuSeries:

    LIVING_STATUSES = ('DEVEL', 'SUPPORTED', 'LTS')

    def __init__(self):
        self.all = {}
        for line in SUPPORTED_RELEASES.splitlines():
            series = Series(*line.split())
            self.all[series.name] = series

    def get_devel_version(self):
        for series in self.all.values():
            if series.status == 'DEVEL':
                return series.version
        else:
            raise AssertionError(
                "SUPPORTED_RELEASES is missing the DEVEL series")

    def get_living_names(self):
        return sorted(s.name for s in self.all.values()
                      if s.status in self.LIVING_STATUSES)

    def get_name(self, version):
        for series in self.all.values():
            if series.version == version:
                return series.name
        else:
            raise KeyError("'%s' is not a known series" % version)

    def get_name_from_package_version(self, package_version):
        """Return the series name associated with the package version.

        The series is matched to the series version commonly embedded in
        backported package versions. Official juju package versions always
        contain the series version to indicate the tool-chain used to build.

        Ubuntu devel packages do not have series versions, they cannot be
        matched to a series. As Ubuntu packages are not built with ideal rules,
        they are not suitable for building agents.
        """
        for series in self.all.values():
            if series.version in package_version:
                return series.name
        return None

    def get_version(self, name):
        return self.all[name].version


juju_series = _JujuSeries()


def parse_dsc(dsc_path, verbose=False):
    """Return the source files need to build a binary package."""
    there = os.path.dirname(dsc_path)
    dsc_name = os.path.basename(dsc_path)
    dcs_source_file = SourceFile(None, None, dsc_name, dsc_path)
    files = [dcs_source_file]
    with open(dsc_path) as f:
        content = f.read()
    found = False
    for line in content.splitlines():
        if found and line.startswith(' '):
            data = line.split()
            data.append(os.path.join(there, data[2]))
            files.append(SourceFile(*data))
            if verbose:
                print("Found %s" % files[-1].name)
        elif found:
            # All files were found.
            break
        if not found and line.startswith('Checksums-Sha256:'):
            found = True
    return files


def setup_local(location, series, arch, source_files, verbose=False):
    """Create a directory to build binaries in.

    The directoy has the source files required to build binaries.
    """
    build_dir = os.path.abspath(
        os.path.join(location, 'juju-build-{}-{}'.format(series, arch)))
    if verbose:
        print('Creating %s' % build_dir)
    os.makedirs(build_dir)
    for sf in source_files:
        dest_path = os.path.join(build_dir, sf.name)
        if verbose:
            print('Copying %s to %s' % (sf.name, build_dir))
        shutil.copyfile(sf.path, dest_path)
    return build_dir


def setup_lxc(series, arch, build_dir, verbose=False):
    """Create an LXC container to build binaries.

    The local build_dir with the source files is bound to the container.
    """
    container = '{}-{}'.format(series, arch)
    lxc_script = CREATE_LXC_TEMPLATE.format(
        container=container, series=series, arch=arch, build_dir=build_dir)
    if verbose:
        print('Creating %s container' % container)
    output = subprocess.check_output([lxc_script], shell=True)
    if verbose:
        print(output)
    return container


def build_in_lxc(container, build_dir, ppa=None, verbose=False):
    """Build the binaries from the source files in the container."""
    returncode = 1
    if ppa:
        path = ppa.split(':')[1]
        series = container.split('-')[0]
        ppa = 'deb http://ppa.launchpad.net/{}/ubuntu {} main'.format(
            path, series)
    else:
        ppa = '# No PPA added.'
    # The work in the container runs as a different user. Care is needed to
    # ensure permissions and ownership are correct before and after the build.
    os.chmod(build_dir, 0o777)
    subprocess.check_call(['sudo', 'lxc-start', '-d', '-n', container])
    try:
        build_script = BUILD_DEB_TEMPLATE.format(container=container, ppa=ppa)
        returncode = subprocess.call([build_script], shell=True)
    finally:
        subprocess.check_call(['sudo', 'lxc-stop', '-n', container])
        user = os.environ.get('USER', 'jenkins')
        subprocess.check_call(['sudo', 'chown', '-R', user, build_dir])
        os.chmod(build_dir, 0o775)
    return returncode


def teardown_lxc(container, verbose=False):
    """Destroy the lxc container."""
    if verbose:
        print('Deleting the lxc container %s' % container)
    subprocess.check_call(['sudo', 'lxc-destroy', '-n', container])


def move_debs(build_dir, location, verbose=False):
    """Move the debs from the build_dir to the location dir."""
    found = False
    files = [f for f in os.listdir(build_dir) if f.endswith('.deb')]
    for file_name in files:
        file_path = os.path.join(build_dir, file_name)
        dest_path = os.path.join(location, file_name)
        if verbose:
            print("Found %s" % file_name)
        shutil.move(file_path, dest_path)
        found = True
    return found


def build_binary(dsc_path, location, series, arch, ppa=None, verbose=False):
    """Build binary debs from a dsc file."""
    # If location is remote, setup remote location and run.
    source_files = parse_dsc(dsc_path, verbose=verbose)
    build_dir = setup_local(
        location, series, arch, source_files, verbose=verbose)
    container = setup_lxc(series, arch, build_dir, verbose=verbose)
    try:
        build_in_lxc(container, build_dir, ppa=ppa, verbose=verbose)
    finally:
        teardown_lxc(container, verbose=False)
    found = move_debs(build_dir, location, verbose=verbose)
    if not found:
        return DEBS_NOT_FOUND
    return 0


def create_source_package_branch(build_dir, version, tarfile, branch):
    """Create a new source package branch with the imported release tarfile.

    The new source package can be pushed to a permanent location if it will
    be used as the base for future packages.

    :param build_dir: The root directory to create the new branch in.
    :param version: The upstream version, which is not always the version
        in the tarfile name.
    :param tarfile: The path to the tarfile to import.
    :param branch: The base source package branch to fork and import into.
    :return: The path to the source package branch.
    """
    spb = os.path.join(build_dir, 'spb')
    tarfile_path = os.path.join(build_dir, tarfile)
    script = CREATE_SPB_TEMPLATE.format(
        branch=branch, spb=spb, tarfile_path=tarfile_path, version=version)
    subprocess.check_call([script], shell=True, cwd=build_dir)
    return spb


def make_ubuntu_version(series, version, upatch=1,
                        date=None, build=None, revid=None):
    """Return an Ubuntu package version.

    :param series: The series codename.
    :param version: The upstream version.
    :param upatch: The package patch number for cases where a packaging rules
        are updated and the package rebuilt.
    :param date: The date of the build.
    :param build: The build number in CI.
    :param revid: The revid hash of the source.
    :return: An Ubuntu version string.
    """
    release = juju_series.get_version(series)
    # if daily params are set, we make daily build
    if all([date, build, revid]):
        return DAILY_VERSION_TEMPLATE.format(
            version=version, release=release, upatch=upatch,
            date=date, build=build, revid=revid)
    else:
        return UBUNTU_VERSION_TEMPLATE.format(
            version=version, release=release, upatch=upatch)


def make_changelog_message(version, bugs=None):
    """Return a changelog message for the version.

    :param version: The upstream version.
    :param bugs: A list of Lp bug numbers or None. They will be formatted
        for the changelog.
    :return: a changelog message.
    """
    match = VERSION_PATTERN.match(version)
    if match is None:
        message = 'New upstream devel release.'
    elif match.group(3) == '0':
        message = 'New upstream stable release.'
    else:
        message = 'New upstream stable point release.'
    if bugs:
        fixes = ', '.join(['LP #%s' % b for b in bugs])
        message = '%s (%s)' % (message, fixes)
    return message


def make_deb_shell_env(debemail, debfullname):
    """Return a replacement environ suitable for DEB building.

    :param debemail: The email address to attribute the changelog entry to.
    :param debfullname: The name to attribute the changelog entry to.
    :return: A modified copy of os.environ that can be passed to subprocesses.
    """
    env = dict(os.environ)
    env['DEBEMAIL'] = debemail
    env['DEBFULLNAME'] = debfullname
    return env


def sign_source_package(source_dir, gpgcmd, debemail, debfullname):
    """Sign a source package.

    The debemail and debfullname must match the identity used in the changelog
    and the changes file.

    :param source_dir: The source package directory.
    :param gpgcmd: The path to a gpg signing command to sign with.
    :param debemail: The email address to attribute the changelog entry to.
    :param debfullname: The name to attribute the changelog entry to.
    """
    env = make_deb_shell_env(debemail, debfullname)
    script = DEBSIGN_TEMPLATE.format(gpgcmd=gpgcmd)
    subprocess.check_call([script], shell=True, cwd=source_dir, env=env)


def create_source_package(source_dir, spb, series, version,
                          upatch='1', bugs=None, gpgcmd=None, debemail=None,
                          debfullname=None, verbose=False,
                          date=None, build=None, revid=None):
    """Create a series source package from a source package branch.

    The new source package can be used to create series source packages.

    :param source_dir: The source package directory.
    :param spb: The path (or location) of the source package branch to create
        the new source package with.
    :param series: The series codename.
    :param version: The upstream version.
    :param upatch: The package patch number for cases where a packaging rules
        are updated and the package rebuilt.
    :param bugs: A list of Lp bug numbers for the changelog or None.
    :param gpgcmd: The path to a gpg signing command to sign with.
        Source packages will be signed when gpgcmd is not None.
    :param debemail: The email address to attribute the changelog entry to.
    :param debfullname: The name to attribute the changelog entry to.
    :param verbose: Increase the information about the work performed.
    :param date: The date of the build.
    :param build: The build number in CI.
    :param revid: The revid hash of the source.
    """

    ubuntu_version = make_ubuntu_version(series, version, upatch,
                                         date, build, revid)
    message = make_changelog_message(version, bugs=bugs)
    source = os.path.join(source_dir, 'source')
    env = make_deb_shell_env(debemail, debfullname)
    script = BUILD_SOURCE_TEMPLATE.format(
        spb=spb, source=source, series=series, ubuntu_version=ubuntu_version,
        message=message)
    subprocess.check_call([script], shell=True, cwd=source_dir, env=env)
    if gpgcmd:
        sign_source_package(source_dir, gpgcmd, debemail, debfullname)


def build_source(tarfile_path, location, series, bugs,
                 debemail=None, debfullname=None, gpgcmd=None,
                 branch=None, upatch=1, verbose=False,
                 date=None, build=None, revid=None):
    """Build one or more series source packages from a new release tarfile.

    The packages are unsigned by default, but providing the path to a gpgcmd,
    the dsc file will be signed.

    :param tarfile_path: The path to the upstream tarfile. to import.
    :param location: The path to the directory to build packages in.
    :param series: The series codename or list of series codenames.
    :param bugs: A list of Lp bug numbers the release fixes.
    :param gpgcmd: The path to a gpg signing command to sign with.
        Source packages will be signed when gpgcmd is not None.
    :param debemail: The email address to attribute the changelog entry to.
    :param debfullname: The name to attribute the changelog entry to.
    :param branch: The path (or location) of the source package branch to
        create the new source package with.
    :param upatch: The package patch number for cases where a packaging rules
        are updated and the package rebuilt.
    :param verbose: Increase the verbostiy of output.
    :param date: The date of the build.
    :param build: The build number in CI.
    :param revid: The revid hash of the source.
    :return: the exit code (which is 0 or else an exception was raised).
    """
    if not isinstance(series, list):
        series = [series]
    tarfile_name = os.path.basename(tarfile_path)
    version = tarfile_name.split('_')[-1].replace('.tar.gz', '')
    if all([date, build, revid]):
        daily_version = '{}~{}~{}~{}'.format(version, date, build, revid)
        daily_tarfile_name = tarfile_name.replace(version, daily_version)
        tarfile_dir = os.path.dirname(tarfile_path)
        daily_tarfile_path = os.path.join(tarfile_dir, daily_tarfile_name)
        os.rename(tarfile_name, daily_tarfile_name)
        tarfile_path = daily_tarfile_path
        tarfile_name = daily_tarfile_name
        version = daily_version

    files = [SourceFile(None, None, tarfile_name, tarfile_path)]
    spb_dir = setup_local(
        location, 'any', 'all', files, verbose=verbose)
    spb = create_source_package_branch(spb_dir, version, tarfile_name, branch)
    for a_series in series:
        build_dir = setup_local(location, a_series, 'all', [], verbose=verbose)
        create_source_package(
            build_dir, spb, a_series, version,
            upatch=upatch, bugs=bugs, gpgcmd=gpgcmd,
            debemail=debemail, debfullname=debfullname, verbose=verbose,
            date=date, build=build, revid=revid)
    return 0


def print_series_info(package_version=None):
    exitcode = 1
    if package_version:
        version = juju_series.get_name_from_package_version(package_version)
        if version:
            print(version)
            return 0
    return exitcode


def main(argv):
    """Execute the commands from the command line."""
    exitcode = 0
    args = get_args(argv)
    if args.command == 'source':
        exitcode = build_source(
            args.tar_file, args.location, args.series, args.bugs,
            debemail=args.debemail, debfullname=args.debfullname,
            gpgcmd=args.gpgcmd, branch=args.branch, upatch=args.upatch,
            verbose=args.verbose,
            date=args.date, build=args.build, revid=args.revid)
    elif args.command == 'binary':
        exitcode = build_binary(
            args.dsc, args.location, args.series, args.arch,
            ppa=args.ppa, verbose=args.verbose)
    elif args.command == 'print':
        exitcode = print_series_info(
            package_version=args.series_name_from_package_version)
    return exitcode


def get_args(argv=None):
    """Return the arguments for this program."""
    parser = ArgumentParser("Build debian packages.")
    parser.add_argument(
        "-v", "--verbose", action="store_true", default=False,
        help="Increase the verbosity of the output")
    subparsers = parser.add_subparsers(help='sub-command help', dest="command")
    src_parser = subparsers.add_parser('source', help='Build source packages')
    src_parser.add_argument(
        '--debemail', default=os.environ.get("DEBEMAIL"),
        help="Your email address; Environment: DEBEMAIL.")
    src_parser.add_argument(
        '--debfullname', default=os.environ.get("DEBFULLNAME"),
        help="Your full name; Environment: DEBFULLNAME.")
    src_parser.add_argument(
        '--gpgcmd', default=None,
        help="Path to a gpg signing command to make signed packages.")
    src_parser.add_argument(
        '--branch', help="The base/previous source package branch.")
    src_parser.add_argument(
        '--upatch', default='1', help="The Ubuntu patch number.")
    src_parser.add_argument('tar_file', help="The release tar file.")
    src_parser.add_argument("location", help="The location to build in.")
    src_parser.add_argument(
        'series', help="The destination Ubuntu release or LIVING for all.")
    src_parser.add_argument(
        '--date', default=None, help="A datestamp to apply to the build")
    src_parser.add_argument(
        '--build', default=None, help="The build number from CI")
    src_parser.add_argument(
        '--revid', default=None, help="The short hash for revid")
    src_parser.add_argument(
        'bugs', nargs='*', help="Bugs this version will fix in the release.")
    bin_parser = subparsers.add_parser('binary', help='Build a binary package')
    bin_parser.add_argument(
        '--ppa', default=None, help="The PPA that provides package deps.")
    bin_parser.add_argument("dsc", help="The dsc file to build")
    bin_parser.add_argument("location", help="The location to build in.")
    bin_parser.add_argument("series", help="The series to build in.")
    bin_parser.add_argument("arch", help="The dpkg architecture to build in.")
    print_parser = subparsers.add_parser('print', help='Print series info')
    print_parser.add_argument(
        '--series-name-from-package-version',
        help="Print the series name associated with the package version.")
    args = parser.parse_args(argv[1:])
    if getattr(args, 'series', None) and args.series == 'LIVING':
        args.series = juju_series.get_living_names()
    if args.command == 'source' and args.branch is None:
        tarfile_name = os.path.basename(args.tar_file)
        version = tarfile_name.split('_')[-1].replace('.tar.gz', '')
        if version.startswith('2.'):
            args.branch = DEFAULT_SPB2
        else:
            args.branch = DEFAULT_SPB
    return args


if __name__ == '__main__':
    sys.exit(main(sys.argv))
