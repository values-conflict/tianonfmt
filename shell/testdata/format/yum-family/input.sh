#!/usr/bin/env bash
set -Eeuo pipefail

# all four names — aliases normalise to the same rules
yum install -y curl
dnf install -y curl
microdnf install -y curl
tinydnf install -y curl

# long form --assumeyes → -y (tidy only)
yum install --assumeyes curl

# -q is non-canonical → --quiet (tidy only)
yum install -q -y curl
