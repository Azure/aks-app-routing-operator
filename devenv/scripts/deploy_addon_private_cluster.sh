#!/usr/bin/env sh
set -e

source ./devenv/scripts/command_invoke_with_output.sh

# Load variables from tfstate
export CLUSTER_CLIENT_ID=$(cat devenv/state/deployment-auth-info.json | jq '.ClusterClientId' | tr -d '"')
export ARM_CLIENT_TENANT_ID=$(cat devenv/state/deployment-auth-info.json | jq '.ArmTenantId' | tr -d '"')
export RG_LOCATION=$(cat devenv/state/deployment-auth-info.json | jq '.ResourceGroupLocation' | tr -d '"')
export DNS_ZONE_RG=$(cat devenv/state/deployment-auth-info.json | jq '.DnsResourceGroup' | tr -d '"')
export DNS_ZONE_SUBSCRIPTION=$(cat devenv/state/deployment-auth-info.json | jq '.DnsZoneSubscription' | tr -d '"')
export DNS_ZONE_DOMAIN=$(cat devenv/state/deployment-auth-info.json | jq '.DnsZoneDomain' | tr -d '"')

# move into state before envsubst
mkdir -p devenv/state/kustomize/operator-private-deployment
cp devenv/kustomize/operator-private-deployment/* devenv/state/kustomize/operator-private-deployment

# Put into filename that starts with terraform
envsubst < devenv/kustomize/operator-private-deployment/kustomization.yaml > devenv/state/kustomize/operator-private-deployment/kustomization.yaml

# Get cluster information for az aks command invoke
CLUSTER_RESOURCE_GROUP=$(cat devenv/state/cluster-info.json | jq '.ClusterResourceGroup' | tr -d '"')
CLUSTER_NAME=$(cat devenv/state/cluster-info.json | jq '.ClusterName' | tr -d '"')

echo "deploying to ${CLUSTER_NAME} in resource group ${CLUSTER_RESOURCE_GROUP}"
cd devenv/state/kustomize/operator-private-deployment

APPLY_RESULT=$(run_invoke $CLUSTER_NAME $CLUSTER_RESOURCE_GROUP "kubectl apply -k ." ".")
