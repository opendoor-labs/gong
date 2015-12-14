FROM resin/raspberrypi2-golang:1.5-slim

COPY . /go/src/github.com/opendoor-labs/gong
CMD modprobe i2c-dev && \
    cd /go/src/github.com/opendoor-labs/gong && \
    go-wrapper install && \
    go-wrapper run
