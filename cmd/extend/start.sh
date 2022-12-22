#!/bin/bash
set -eu
set -o pipefail
DATE=$(date "+%Y-%m-%d %H:%M:%S")

# 20 13 * * 5 /usr/bin/nohup /bin/bash /ipfsdata/extend_sectors.sh >> /ipfsdata/extend.log 2>&1 &

echo "$DATE start......."
/usr/local/bin/lotus-extend -address=127.0.0.1:2234 -token=<token> -debug=false -s=true -r=10 -e=2023-10-01

