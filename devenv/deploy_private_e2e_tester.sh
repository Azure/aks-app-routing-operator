CLUSTER_RESOURCE_GROUP=$(cat state/cluster-info.json | jq '.ClusterResourceGroup' | tr -d '"')
CLUSTER_NAME=$(cat state/cluster-info.json | jq '.ClusterName' | tr -d '"')

export IMAGE=$(cat state/e2e-image-tag.txt)
envsubst < e2e-tester.yaml > state/e2e-tester-formatted.yaml

az aks command invoke --resource-group $CLUSTER_RESOURCE_GROUP --name $CLUSTER_NAME --command "kubectl apply -f e2e-tester-formatted.yaml" --file state/e2e-tester-formatted.yaml

RESULT="null"
while [ "$RESULT" = "null" ]
do
  STATUS=$(az aks command invoke --resource-group $CLUSTER_RESOURCE_GROUP --name $CLUSTER_NAME --command 'kubectl get pods --selector=app=app-routing-operator-e2e --output="jsonpath={.items[0].status.containerStatuses[?(@.name==\"tester\")].state}" -n kube-system' -o json | jq ".logs" | tr -d '\\')
  STATUSNOQUOTES=${STATUS#"\""}
  STATUSNOQUOTES=${STATUSNOQUOTES%"\""}

  RESULT=$(echo $STATUSNOQUOTES | jq '.terminated')
  echo "test status is currently $STATUSNOQUOTES"

  sleep 5

done

echo "exited loop with status $STATUSNOQUOTES"Z

FINALSTATUS=$(echo $RESULT | jq ".exitCode")

echo "Test finished, echoing test pod logs..."
az aks command invoke --resource-group $CLUSTER_RESOURCE_GROUP --name $CLUSTER_NAME --command 'kubectl logs -l app=app-routing-operator-e2e -n kube-system'

if [ $FINALSTATUS != "0" ]
then
  echo "TEST FAILED"
fi



exit $FINALSTATUS