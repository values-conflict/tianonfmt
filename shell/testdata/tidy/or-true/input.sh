#!/usr/bin/env bash
set -Eeuo pipefail
cmd1 || true
cmd2 && cmd3 || true
