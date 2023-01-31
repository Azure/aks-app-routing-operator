# Get cluster information for az aks command invoke
CLUSTER_RESOURCE_GROUP=$(cat state/cluster-info.json | jq '.ClusterResourceGroup' | tr -d '"')
CLUSTER_NAME=$(cat state/cluster-info.json | jq '.ClusterName' | tr -d '"')

TAG=$(cat state/operator-image-tag.txt)
# Push image
CMD="kubectl set image -n kube-system deployments/app-routing-operator operator=$TAG"

az aks command invoke --resource-group $CLUSTER_RESOURCE_GROUP --name $CLUSTER_NAME --command "$CMD"