#!/usr/bin/env bash
set -Eeuo pipefail
cmd1 || :
cmd2 && cmd3 || :
