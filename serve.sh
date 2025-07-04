#!/bin/bash
# Copyright 2025 Marc-Antoine Ruel. All rights reserved.
# Use of this source code is governed under the Apache License, Version 2.0
# that can be found in the LICENSE file.

set -eu
cd "$(dirname "$0")"

# Handle Ctrl+C (SIGINT) gracefully
trap "echo -e '\nExiting...'; exit 0" INT

go install .

while true; do
  devpostdash "$@"
done
