set -e
echo "beginning private cluster tests..."
make dev CLUSTER_TYPE=private
make push
make e2e

