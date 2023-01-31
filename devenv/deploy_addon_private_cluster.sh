# Load variables from tfstate
export CLUSTER_CLIENT_ID=$(cat terraform.tfstate | jq '.resources[] | select((.name=="clusteridentity") and .type=="azurerm_user_assigned_identity") | .instances[] | .attributes.client_id')
export ARM_CLIENT_TENANT_ID=$(cat terraform.tfstate | jq '.resources[] | select((.name=="current") and .type=="azurerm_client_config") | .instances[] | .attributes.tenant_id')
export RG_LOCATION=$(cat terraform.tfstate | jq '.resources[] | select((.type=="azurerm_resource_group") and .name=="rg") | .instances[] | .attributes.location')
export DNS_ZONE_RG=$(cat terraform.tfstate | jq '.resources[] | select((.type=="azurerm_private_dns_zone") and .name=="dnszone") | .instances[] | .attributes.resource_group_name')
export DNS_ZONE_SUBSCRIPTION=$(cat terraform.tfstate | jq '.resources[] | select((.type=="azurerm_subscription") and .name=="current") | .instances[] | .attributes.subscription_id')
export DNS_ZONE_DOMAIN=$(cat terraform.tfstate | jq '.resources[] | select((.type=="azurerm_private_dns_zone") and .name=="dnszone") | .instances[] | .attributes.name')

# Put into filename that starts with terraform
envsubst < private-cluster.yaml > state/terraform-formatted.yaml

# Get cluster information for az aks command invoke
CLUSTER_RESOURCE_GROUP=$(cat state/cluster-info.json | jq '.ClusterResourceGroup' | tr -d '"')
CLUSTER_NAME=$(cat state/cluster-info.json | jq '.ClusterName' | tr -d '"')

echo "deploying to ${CLUSTER_NAME} in resource group ${CLUSTER_RESOURCE_GROUP}"

az aks command invoke --resource-group $CLUSTER_RESOURCE_GROUP --name $CLUSTER_NAME --command "kubectl apply -f terraform-formatted.yaml" --file state/terraform-formatted.yaml