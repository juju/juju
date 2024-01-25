FROM public.ecr.aws/ubuntu/ubuntu:22.04

RUN printf 'deb [arch=amd64] http://archive.ubuntu.com/ubuntu/ jammy main restricted\n\
deb [arch=amd64] http://archive.ubuntu.com/ubuntu/ jammy-updates main restricted\n\
deb [arch=amd64] http://archive.ubuntu.com/ubuntu/ jammy universe\n\
deb [arch=amd64] http://archive.ubuntu.com/ubuntu/ jammy-updates universe\n\
deb [arch=amd64] http://archive.ubuntu.com/ubuntu/ jammy multiverse\n\
deb [arch=amd64] http://archive.ubuntu.com/ubuntu/ jammy-updates multiverse\n\
deb [arch=amd64] http://archive.ubuntu.com/ubuntu/ jammy-backports main restricted universe multiverse\n\
deb [arch=amd64] http://security.ubuntu.com/ubuntu/ jammy-security main restricted\n\
deb [arch=amd64] http://security.ubuntu.com/ubuntu/ jammy-security universe\n\
deb [arch=amd64] http://security.ubuntu.com/ubuntu/ jammy-security multiverse\n\
deb [arch=arm64,s390x,ppc64el] http://ports.ubuntu.com/ubuntu-ports/ jammy main restricted\n\
deb [arch=arm64,s390x,ppc64el] http://ports.ubuntu.com/ubuntu-ports/ jammy-updates main restricted\n\
deb [arch=arm64,s390x,ppc64el] http://ports.ubuntu.com/ubuntu-ports/ jammy universe\n\
deb [arch=arm64,s390x,ppc64el] http://ports.ubuntu.com/ubuntu-ports/ jammy-updates universe\n\
deb [arch=arm64,s390x,ppc64el] http://ports.ubuntu.com/ubuntu-ports/ jammy multiverse\n\
deb [arch=arm64,s390x,ppc64el] http://ports.ubuntu.com/ubuntu-ports/ jammy-updates multiverse\n\
deb [arch=arm64,s390x,ppc64el] http://ports.ubuntu.com/ubuntu-ports/ jammy-backports main restricted universe multiverse\n\
deb [arch=arm64,s390x,ppc64el] http://ports.ubuntu.com/ubuntu-ports/ jammy-security main restricted\n\
deb [arch=arm64,s390x,ppc64el] http://ports.ubuntu.com/ubuntu-ports/ jammy-security universe\n\
deb [arch=arm64,s390x,ppc64el] http://ports.ubuntu.com/ubuntu-ports/ jammy-security multiverse\n' > /etc/apt/sources.list

RUN apt-get update && apt-get upgrade -y \
&& apt-get install -y software-properties-common curl wget \
&& dpkg --add-architecture arm64 \
&& dpkg --add-architecture s390x \
&& dpkg --add-architecture ppc64el \
&& add-apt-repository ppa:dqlite/dev -y \
&& printf 'deb [arch=amd64,arm64,s390x,ppc64el] https://ppa.launchpadcontent.net/dqlite/dev/ubuntu/ jammy main\n' > /etc/apt/sources.list.d/dqlite-ubuntu-dev-jammy.list \
&& apt-get update \
&& apt-get install -y build-essential \
&& apt-get install -y libdqlite-dev libraft-dev libdqlite0 \
&& apt-get install -y gcc-aarch64-linux-gnu libdqlite-dev:arm64 libraft-dev:arm64 libdqlite0:arm64 \
&& apt-get install -y gcc-s390x-linux-gnu libdqlite-dev:s390x libraft-dev:s390x libdqlite0:s390x \
&& apt-get install -y gcc-powerpc64le-linux-gnu libdqlite-dev:ppc64el libraft-dev:ppc64el libdqlite0:ppc64el

ADD install-go.sh .
RUN chmod +x install-go.sh && GOVERSION=1.21 ./install-go.sh && rm install-go.sh

# libraft3:arm64
# deb [arch=amd64,arm64,s390x,ppc64el] https://ppa.launchpadcontent.net/dqlite/dev/ubuntu/ jammy main