# QEMU Setup

Install the following:

```console
sudo apt-get update
sudo apt-get install qemu qemu-kvm
```

## QEMU Images

The following instructions show how to setup a set of images to work with spread
locally.

```console
sudo apt install autopkgtest genisoimage
mkdir -p ~/.spread/qemu
cd ~/.spread/qemu
```

### 16.04 (xenial)

The 16.04 (xenial) setup requires a few more things in the base image to be
sorted and ensure everything is running smoothly.

```console
autopkgtest-buildvm-ubuntu-cloud -r xenial --post-command='sudo apt remove --purge -y lxd lxd-client liblxc1 lxcfs && sudo apt-get update && sudo apt-get install -y build-essential snapd ca-certificates bzip2 distro-info-data git zip && sudo snap install lxd && sudo snap install jq'
```

### 18.04 (bionic)

18.04 (bionic) setup is a bit more straight forward, but helps if we have some
default things installed to make things easier for setting up the tests.

```console
autopkgtest-buildvm-ubuntu-cloud -r bionic --post-command='sudo apt-get update && sudo apt-get install -y build-essential snapd ca-certificates bzip2 distro-info-data git zip && sudo snap install lxd && sudo snap install jq'
```

Note: you may need to add these repos to your source list

```console
add-apt-repository 'deb http://archive.ubuntu.com/ubuntu bionic main universe'
add-apt-repository 'deb http://archive.ubuntu.com/ubuntu bionic-security main universe'
add-apt-repository 'deb http://archive.ubuntu.com/ubuntu bionic-updates main universe'
```
