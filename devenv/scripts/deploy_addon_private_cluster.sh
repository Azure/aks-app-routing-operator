# Load variables from tfstate
export CLUSTER_CLIENT_ID=$(cat state/deployment-auth-info.json | jq '.ClusterClientId' | tr -d '"')
export ARM_CLIENT_TENANT_ID=$(cat state/deployment-auth-info.json | jq '.ArmTenantId' | tr -d '"')
export RG_LOCATION=$(cat state/deployment-auth-info.json | jq '.ResourceGroupLocation' | tr -d '"')
export DNS_ZONE_RG=$(cat state/deployment-auth-info.json | jq '.DnsResourceGroup' | tr -d '"')
export DNS_ZONE_SUBSCRIPTION=$(cat state/deployment-auth-info.json | jq '.DnsZoneSubscription' | tr -d '"')
export DNS_ZONE_DOMAIN=$(cat state/deployment-auth-info.json | jq '.DnsZoneDomain' | tr -d '"')

# Put into filename that starts with terraform
envsubst < private-cluster.yaml > state/terraform-formatted.yaml

# Get cluster information for az aks command invoke
CLUSTER_RESOURCE_GROUP=$(cat state/cluster-info.json | jq '.ClusterResourceGroup' | tr -d '"')
CLUSTER_NAME=$(cat state/cluster-info.json | jq '.ClusterName' | tr -d '"')

echo "deploying to ${CLUSTER_NAME} in resource group ${CLUSTER_RESOURCE_GROUP}"

az aks command invoke --resource-group $CLUSTER_RESOURCE_GROUP --name $CLUSTER_NAME --command "kubectl apply -f terraform-formatted.yaml" --file state/terraform-formatted.yaml