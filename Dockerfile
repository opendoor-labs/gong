FROM resin/raspberrypi2-python:2.7

# Enable systemd init system
ENV INITSYSTEM on

# -- Start of resin-wifi-connect section -- #

# Use apt-get to install dependencies
RUN apt-get update && apt-get install -yq --no-install-recommends \
    dnsmasq \
    hostapd \
    iproute2 \
    iw \
    libdbus-1-dev \
    libexpat-dev \
    rfkill && \
    apt-get clean && rm -rf /var/lib/apt/lists/*

# Install node
ENV NODE_VERSION 6.9.1
RUN curl -SLO "http://nodejs.org/dist/v$NODE_VERSION/node-v$NODE_VERSION-linux-armv6l.tar.gz" && \
    echo "0b30184fe98bd22b859db7f4cbaa56ecc04f7f526313c8da42315d89fabe23b2  node-v6.9.1-linux-armv6l.tar.gz" | sha256sum -c - && \
    tar -xzf "node-v$NODE_VERSION-linux-armv6l.tar.gz" -C /usr/local --strip-components=1 && \
    rm "node-v$NODE_VERSION-linux-armv6l.tar.gz" && \
    npm config set unsafe-perm true -g --unsafe-perm && \
    rm -rf /tmp/*

# Install resin-wifi-connect
RUN git clone https://github.com/resin-io/resin-wifi-connect.git && \
    cd resin-wifi-connect && \
    JOBS=MAX npm install --unsafe-perm --production && \
    npm cache clean && \
    ./node_modules/.bin/bower --allow-root install && \
    ./node_modules/.bin/bower --allow-root cache clean && \
    ./node_modules/.bin/coffee -c ./src

# -- End of resin-wifi-connect section -- #

ENV GO_VERSION 1.6.4

RUN buildDeps='curl gcc g++ git' \
    && set -x \
    && apt-get update && apt-get install -y $buildDeps \
    && rm -rf /var/lib/apt/lists/* \
    && mkdir -p /usr/local/go \
    && curl -SLO "http://resin-packages.s3.amazonaws.com/golang/v$GO_VERSION/go$GO_VERSION.linux-armv7hf.tar.gz" \
    && echo "2e1041466b9bdffb1c07c691c2cfd6346ee3ce122f57fdf92710f24a400dc4e6  go1.6.4.linux-armv7hf.tar.gz" | sha256sum -c - \
    && tar -xzf "go$GO_VERSION.linux-armv7hf.tar.gz" -C /usr/local/go --strip-components=1 \
    && rm -f go$GO_VERSION.linux-armv7hf.tar.gz

ENV GOROOT /usr/local/go
ENV GOPATH /go
ENV PATH $GOPATH/bin:/usr/local/go/bin:$PATH

RUN mkdir -p "$GOPATH/src" "$GOPATH/bin" && chmod -R 777 "$GOPATH"
WORKDIR $GOPATH

COPY go-wrapper /usr/local/bin/

COPY . /go/src/github.com/opendoor-labs/gong
CMD ["bash", "/go/src/github.com/opendoor-labs/gong/start.sh"]
