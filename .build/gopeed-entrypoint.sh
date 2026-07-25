#!/bin/sh
set -eu

mkdir -p /app/storage /app/Downloads
chown -R gopeed:gopeed /app/storage /app/Downloads

# BusyBox's su is part of the Alpine base image, so the runtime image does
# not need to download an extra privilege-dropping package during its build.
# The image runs one fixed server binary; avoiding a shell-built argument
# string also prevents command override values from being interpreted here.
exec su gopeed -s /bin/sh -c 'exec ./gopeed'
