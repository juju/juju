FROM ubuntu:22.04
ARG TARGETOS
ARG TARGETARCH
ARG BUILDOS

# Add the syslog user for audit logging.
RUN useradd --system -M -s /usr/sbin/nologin syslog

# Create a user under which jujud can run
RUN useradd --system -M -s /usr/sbin/nologin jujud && \
    mkdir -p /var/lib/juju && \
    chown -R jujud:jujud /var/lib/juju

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
    && pip3 install --upgrade pip setuptools \
    && rm -rf /var/lib/apt/lists/* \
    && rm -rf /root/.cache

# Install the standard charm dependencies.
ENV WHEELHOUSE=/tmp/wheelhouse
ENV PIP_WHEEL_DIR=/tmp/wheelhouse
ENV PIP_FIND_LINKS=/tmp/wheelhouse

RUN mkdir -p /tmp/wheelhouse
COPY docker-staging/requirements.txt /tmp/wheelhouse/requirements.txt
RUN pip3 install -r /tmp/wheelhouse/requirements.txt

COPY ${TARGETOS}_${TARGETARCH}/bin/jujud /opt/
COPY ${TARGETOS}_${TARGETARCH}/bin/jujuc /opt/
COPY ${TARGETOS}_${TARGETARCH}/bin/containeragent /opt/
COPY ${TARGETOS}_${TARGETARCH}/bin/pebble /opt/

USER jujud
WORKDIR /var/lib/juju

ENTRYPOINT ["sh", "-c"]
