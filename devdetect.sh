#!/bin/sh
set -e

DEV_ID=`lsusb | grep 'Holtek Semiconductor, Inc' | awk '{print $6}'`
if [ -z "$DEV_ID" ]; then
	echo "ERROR: device id not found" >&2
	exit 1
fi

DEV_NAME=`ls -pilaF /sys/class/hidraw/ | grep -i $DEV_ID | awk '{print $10}'`
if [ -z "$DEV_NAME" ]; then
	echo "ERROR: device name not found" >&2
	exit 2
fi

ls /dev/${DEV_NAME}
