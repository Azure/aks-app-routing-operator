	set -e
	echo "beginning private cluster tests..."
	make dev CLUSTER_TYPE=private DNS_ZONE_TYPE=public
	make push
	make e2e

  make dev CLUSTER_TYPE=private DNS_ZONE_TYPE=private
	make push
	make e2e

