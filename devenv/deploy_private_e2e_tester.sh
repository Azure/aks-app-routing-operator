CLUSTER_RESOURCE_GROUP=$(cat state/cluster-info.json | jq '.ClusterResourceGroup' | tr -d '"')
CLUSTER_NAME=$(cat state/cluster-info.json | jq '.ClusterName' | tr -d '"')

export IMAGE=$(cat state/e2e-image-tag.txt)
envsubst < e2e-tester.yaml > state/e2e-tester-formatted.yaml

az aks command invoke --resource-group $CLUSTER_RESOURCE_GROUP --name $CLUSTER_NAME --command "kubectl apply -f e2e-tester-formatted.yaml" --file state/e2e-tester-formatted.yaml

EXIT_CODE=$(kubectl get pod busybox-term -ojson | jq .status.containerStatuses[].lastState.terminated.exitCode)

# TODO TOMORROW - FIGURE OUT HOW TO GET EXIT CODE AND SEE WHAT IT IS WHEN TERMINATED - GOOD PLACE TO START:
# az aks command invoke --resource-group app-routing-dev-e51abs1vw32ca --name cluster --command 'kubectl get pod app-routing-operator-6ff9968546-t7w57 -n kube-system --output="jsonpath={.status.containerStatuses[]}"'