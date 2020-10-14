ARG BASE_IMAGE
ARG BASE_GOLANG_IMAGE

# Build pebble.
FROM $BASE_GOLANG_IMAGE AS pebblebuild

ARG GOOS
ENV GOOS=${GOOS}
ARG GOARCH
ENV GOARCH=${GOARCH}

WORKDIR /tmp/pebble
ADD go.mod .
ADD go.sum .
RUN go build -o /usr/local/bin/pebble -ldflags "-s -w -extldflags '-static'" -v github.com/hpidcock/juju-fake-init

# Build operator image.
FROM $BASE_IMAGE

# Add the syslog user for audit logging.
RUN useradd --system -M syslog
RUN usermod -s /usr/sbin/nologin syslog

# Some Python dependencies.
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
    python3-yaml \
    python3-pip \
    python3-distutils \
    # for debug-hooks.
    tmux byobu \
    # below apt dependencies are required by controller pod.
    iproute2 \
    curl \
    # TODO(new-charms): remove when pebble/initer is complete.
    openssh-client \
    && pip3 install --upgrade pip setuptools \
    && rm -rf /var/lib/apt/lists/* \
    && rm -rf /root/.cache

# Install the standard charm dependencies.
ENV WHEELHOUSE=/tmp/wheelhouse
ENV PIP_WHEEL_DIR=/tmp/wheelhouse
ENV PIP_FIND_LINKS=/tmp/wheelhouse

COPY requirements.txt /tmp/wheelhouse/requirements.txt
RUN pip3 install -r /tmp/wheelhouse/requirements.txt

WORKDIR /var/lib/juju
COPY jujud /opt/
COPY jujuc /opt/
COPY k8sagent /opt/
COPY --from=pebblebuild /usr/local/bin/pebble /opt/pebble

ENTRYPOINT ["sh", "-c"]
