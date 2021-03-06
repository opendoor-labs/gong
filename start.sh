#!/bin/bash

modprobe i2c-dev

# Start resin-wifi-connect
export DEVICE_TYPE=raspberrypi2
export DBUS_SYSTEM_BUS_ADDRESS=unix:path=/host_run/dbus/system_bus_socket
sleep 1
node /resin-wifi-connect/src/app.js --clear=false

# At this point the WiFi connection has been configured and the device has
# internet - unless the configured WiFi connection is no longer available.

# Start the main application
cd /go/src/github.com/opendoor-labs/gong
go-wrapper install
go-wrapper run
