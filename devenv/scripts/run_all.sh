	echo "running all"
	make private-cluster-public-dns
	make deploy-operator
	make push
	make e2e

	make private-cluster-private-dns
	make deploy-operator
	make push
	make e2e

	echo "beginning public cluster tests..."
	make clean
	make public-cluster-public-dns
	make push
	make deploy-operator
	make e2e

	make public-cluster-private-dns
	make deploy-operator
	make e2e