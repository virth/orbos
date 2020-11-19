#!/usr/bin/env bash
# getter for gopass and secret yaml creation
set -e
[[ `uname` = "Linux" ]] && ENCODE="base64 --wrap=0" || ENCODE="base64"

cat <<EOL
apiVersion: v1
data:
  username: $(echo -n "${1}" | $ENCODE)
  password: $(echo -n "${2}" | $ENCODE)
kind: Secret
metadata:
  name: grafana-cloud
  namespace: caos-system
type: Opaque
---
EOL
