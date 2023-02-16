#!/usr/bin/env sh
# Get cluster information for az aks command invoke
CLUSTER_RESOURCE_GROUP=$(cat state/cluster-info.json | jq '.ClusterResourceGroup' | tr -d '"')
CLUSTER_NAME=$(cat state/cluster-info.json | jq '.ClusterName' | tr -d '"')

if [ -z $CLUSTER_RESOURCE_GROUP  ]; then
  echo "CLUSTER_RESOURCE_GROUP is empty"
  exit 1
fi

if [ -z $CLUSTER_NAME  ]; then
  echo "CLUSTER_NAME is empty"
  exit 1
fi

TAG=$(cat state/operator-image-tag.txt)

if [ -z $TAG  ]; then
  echo "TAG is empty"
  exit 1
fi

# Push image
CMD="kubectl set image -n kube-system deployments/app-routing-operator operator=$TAG"

az aks command invoke --resource-group $CLUSTER_RESOURCE_GROUP --name $CLUSTER_NAME --command "$CMD"