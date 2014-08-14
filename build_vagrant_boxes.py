#!/usr/bin/env python
from jenkins import Jenkins
import logging
import os
import re
import shutil
import subprocess


"""Build Juju-Vagrant boxes for Juju packages build by publish-revision.

Required environment variables:

    WORKSPACE - path to Jenkins' workspace directory
    JENKINS_KVM - path to the local copy of
        lp:~ubuntu-on-ec2/vmbuilder/jenkins_kvm (main build scripts)
"""

JENKINS_URL = 'http://juju-ci.vapour.ws:8080'
PUBLISH_REVISION_JOB = 'publish-revision'
SERIES = (('trusty', '14.04'), ('precise', '12.04'))
ARCH = ('amd64', 'i386')
JENKINS_KVM = 'JENKINS_KVM'
WORKSPACE = 'WORKSPACE'

def package_regexes():
    for series in SERIES:
        for arch in ARCH:
            series_number = series[1].replace('.', r'\.')
            regex_core = re.compile(
                r'^juju-core_.*%s.*%s\.deb$' % (series_number, arch))
            yield series[0], arch, regex_core

        regex_local = re.compile(
            r'^juju-local_.*%s.*all\.deb$' % series_number)
        yield series[0], 'all', regex_local


def get_debian_packages(jenkins):
    job_info = jenkins.get_job_info(PUBLISH_REVISION_JOB)
    build_number = job_info['lastSuccessfulBuild']['number']
    build_info = jenkins.get_build_info(PUBLISH_REVISION_JOB, build_number)

    result = {}
    try:
        for artifact in build_info['artifacts']:
            filename = artifact['fileName']
            for series, arch, matcher in package_regexes():
                if matcher.search(filename) is not None:
                    package_url = '%s/artifact/%s' % (
                        build_info['url'], filename)
                    local_path = os.path.join(os.getenv(WORKSPACE), filename)
                    logging.info(
                        'copying %s from build %s' % (filename, build_number))
                    result.setdefault(series, {})[arch] = local_path
                    command = 'wget -q -O %s %s' % (
                        local_path, package_url)
                    subprocess.check_call(command.split(' '))
                    break
    except Exception:
        for arch in result.values():
            for file_path in arch.values():
                if os.path.exists(file_path):
                    os.unlink(file_path)
        raise
    return result


def remove_leftover_virtualbox(series, arch):
    """The build script sometimes does not remove a VirtualBox instance.

    If such an instance is not deleted, the next build for the same
    series/architecture will fail.
    """
    left_over_name = 'ubuntu-cloudimg-%s-juju-vagrant-%s' % (series, arch)
    instances = subprocess.check_output(['vboxmanage', 'list', 'vms'])
    for line in instances.split('\n'):
        name, uid = line.split(' ')
        name = name.strip('"')
        if name == left_over_name:
            subprocess.check_call([
                'vboxmanage', 'unregistervm', name, '--delete'])
            break


def build_vagrant_box(series, arch, juju_core_package, juju_local_package):
    env = {
        'JUJU_CORE_PKG': juju_core_package,
        'JUJU_LOCAL_PKG': juju_local_package,
        'BUILD_FOR': '%s:%s' % (series, arch),
    }
    builder_path = os.path.join(
        os.getenv(JENKINS_KVM), 'build-juju-local.sh')
    logging.info('Building Vagrant box for %s %s' % (series, arch))
    try:
        subprocess.check_call(builder_path, env=env)
        boxname = '%s-server-cloudimg-%s-juju-vagrant-disk1.box' % (
            series, arch)
        build_dir = '%s-%s' % (series, arch)
        os.rename(
            os.path.join(os.getenv(JENKINS_KVM), build_dir, boxname),
            os.path.join(os.getenv(WORKSPACE), boxname))
    finally:
        remove_leftover_virtualbox(series, arch)
        shutil.rmtree(os.path.join(os.getenv(JENKINS_KVM), build_dir))
        os.unlink(os.path.join(
            os.getenv(JENKINS_KVM), '%s-builder-%s.img' % (series, arch)))


def build_vagrant_boxes():
    logging.basicConfig(
        level=logging.INFO, format='%(asctime)s %(levelname)s %(message)s',
        datefmt='%Y-%m-%d %H:%M:%S')
    jenkins = Jenkins(JENKINS_URL)
    package_info = get_debian_packages(jenkins)
    try:
        for series in package_info:
            if 'all' not in package_info[series]:
                log.error(
                    'juju-local package is missing for series %s' % series)
                continue
            local_package_name = package_info[series]['all']
            for arch, package_name in package_info[series].items():
                if arch == 'all':
                    continue
                build_vagrant_box(
                    series, arch, package_name, local_package_name)
    finally:
        for arch_info in package_info.values():
            for filename in arch_info.values():
                os.unlink(filename)


if __name__ == '__main__':
    build_vagrant_boxes()
