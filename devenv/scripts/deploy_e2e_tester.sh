#!/usr/bin/env sh
set -e
source ./devenv/scripts/command_invoke_with_output.sh

CLUSTER_RESOURCE_GROUP=$(cat devenv/state/cluster-info.json | jq '.ClusterResourceGroup' | tr -d '"')
CLUSTER_NAME=$(cat devenv/state/cluster-info.json | jq '.ClusterName' | tr -d '"')

echo "adding image tag to kustomize and generating configmap..."
cp devenv/kustomize/* devenv/state/kustomize

cd devenv/state/kustomize # change workingdir to kustomize
kustomize edit set image placeholderfortesterimage=`cat ../e2e-image-tag.txt`

echo "deleting any existing e2e job..."
DELETE_APPLY_RESULT=$(run_invoke $CLUSTER_NAME $CLUSTER_RESOURCE_GROUP "kubectl delete jobs app-routing-operator-e2e -n kube-system --ignore-not-found && kubectl apply -k ." ".")

cd ../.. # go back to root as working dir

# wait until cluster has reached terminated status, keep checking until terminated result is not null
RESULT="null"
while [ "$RESULT" = "null" ]
do
  # use az cli to get status from pod JSON
  STATUS=$(run_invoke $CLUSTER_NAME $CLUSTER_RESOURCE_GROUP 'kubectl get pods --selector=app=app-routing-operator-e2e --output="jsonpath={.items[0].status.containerStatuses[?(@.name==\"tester\")].state}" -n kube-system' | jq ".logs" | tr -d '\\' )

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
POD_LOGS_RESULT=$(run_invoke $CLUSTER_NAME $CLUSTER_RESOURCE_GROUP 'kubectl logs -l app=app-routing-operator-e2e -n kube-system' | tr -d '\n\r' | jq ".logs")
echo $POD_LOGS_RESULT


if [ $FINALSTATUS != "0" ]
then
  echo "TEST FAILED"
else
  echo "TEST PASSED"
fi

exit $FINALSTATUS