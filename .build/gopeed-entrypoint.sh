#!/bin/sh
set -eu

mkdir -p /app/storage /app/Downloads
# The runtime deliberately drops DAC_OVERRIDE. Recursive ownership changes
# would therefore fail on existing task workspaces owned by the unprivileged
# service account and cause a restart loop. The two mount roots are enough:
# Gopeed creates every child itself as `gopeed`, and its per-task temporary
# state is cleaned up with the task workspace.
chown gopeed:gopeed /app/storage /app/Downloads

# BusyBox's su is part of the Alpine base image, so the runtime image does
# not need to download an extra privilege-dropping package during its build.
# The image runs one fixed server binary; avoiding a shell-built argument
# string also prevents command override values from being interpreted here.
exec su gopeed -s /bin/sh -c 'exec ./gopeed'
