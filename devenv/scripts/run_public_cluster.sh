	set -e

	echo "beginning public cluster tests..."
	make clean
	make dev CLUSTER_TYPE=public DNS_ZONE_TYPE=public
	make push
	make e2e

	make dev CLUSTER_TYPE=public DNS_ZONE_TYPE=private
	make push
	make e2e