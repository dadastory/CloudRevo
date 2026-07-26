#!/bin/sh
set -eu

exec docker compose --profile test run --build --rm --use-aliases gopeed-contract
