#!/usr/bin/env bash
set -e
source ./devenv/scripts/command_invoke_with_output.sh

# Get cluster information for az aks command invoke
CLUSTER_RESOURCE_GROUP=$(cat devenv/state/cluster-info.json | jq '.ClusterResourceGroup' | tr -d '"')
CLUSTER_NAME=$(cat devenv/state/cluster-info.json | jq '.ClusterName' | tr -d '"')

if [ -z $CLUSTER_RESOURCE_GROUP  ]; then
  echo "CLUSTER_RESOURCE_GROUP is empty"
  exit 1
fi

if [ -z $CLUSTER_NAME  ]; then
  echo "CLUSTER_NAME is empty"
  exit 1
fi

TAG=$(cat devenv/state/operator-image-tag.txt)

if [ -z $TAG  ]; then
  echo "TAG is empty"
  exit 1
fi

# Push image
CMD="kubectl set image -n kube-system deployments/app-routing-operator operator=$TAG operator-setup=$TAG"

run_invoke $CLUSTER_NAME $CLUSTER_RESOURCE_GROUP "$CMD"
