set -e
echo "beginning public cluster tests..."
make clean
make dev CLUSTER_TYPE=public
make push
make e2e