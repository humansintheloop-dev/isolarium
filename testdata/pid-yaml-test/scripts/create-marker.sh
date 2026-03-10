#!/bin/bash
set -euo pipefail

sudo mkdir -p /opt/isolarium
echo "isolation-scripts-ok" | sudo tee /opt/isolarium/marker.txt > /dev/null
