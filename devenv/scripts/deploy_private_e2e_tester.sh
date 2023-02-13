CLUSTER_RESOURCE_GROUP=$(cat state/cluster-info.json | jq '.ClusterResourceGroup' | tr -d '"')
CLUSTER_NAME=$(cat state/cluster-info.json | jq '.ClusterName' | tr -d '"')

# get image tag for tester deployment
export IMAGE=$(cat state/e2e-image-tag.txt)
envsubst < e2e-tester.yaml > state/e2e-tester-formatted.yaml

echo "deleting any existing e2e job..."
az aks command invoke --resource-group $CLUSTER_RESOURCE_GROUP --name $CLUSTER_NAME --command "kubectl delete jobs app-routing-operator-e2e -n kube-system"

set -e
az aks command invoke --resource-group $CLUSTER_RESOURCE_GROUP --name $CLUSTER_NAME --command "kubectl apply -f e2e-tester-formatted.yaml" --file state/e2e-tester-formatted.yaml

# wait until cluster has reached terminated status, keep checking until terminated result is not null
RESULT="null"
while [ "$RESULT" = "null" ]
do
  # use az cli to get status from pod JSON
  STATUS=$(az aks command invoke --resource-group $CLUSTER_RESOURCE_GROUP --name $CLUSTER_NAME --command 'kubectl get pods --selector=app=app-routing-operator-e2e --output="jsonpath={.items[0].status.containerStatuses[?(@.name==\"tester\")].state}" -n kube-system' -o json | jq ".logs" | tr -d '\\')

  # remove quotes at prefix and suffix from output to be able to use jquery
  STATUSNOQUOTES=${STATUS#"\""}
  STATUSNOQUOTES=${STATUSNOQUOTES%"\""}

  RESULT=$(echo $STATUSNOQUOTES | jq '.terminated')
  echo "test status is currently $STATUSNOQUOTES"

  sleep 5

done

echo "exited loop with status $STATUSNOQUOTES"

FINALSTATUS=$(echo $RESULT | jq ".exitCode")

echo "Test finished, echoing test pod logs..."
az aks command invoke --resource-group $CLUSTER_RESOURCE_GROUP --name $CLUSTER_NAME --command 'kubectl logs -l app=app-routing-operator-e2e -n kube-system'

if [ $FINALSTATUS != "0" ]
then
  echo "TEST FAILED"
fi

exit $FINALSTATUS