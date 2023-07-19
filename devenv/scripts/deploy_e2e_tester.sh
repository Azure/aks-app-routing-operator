#!/usr/bin/env bash
set -e
source ./devenv/scripts/command_invoke_with_output.sh

CLUSTER_RESOURCE_GROUP=$(cat devenv/state/cluster-info.json | jq '.ClusterResourceGroup' | tr -d '"')
CLUSTER_NAME=$(cat devenv/state/cluster-info.json | jq '.ClusterName' | tr -d '"')

echo "adding image tags to kustomize and generating configmaps..."
cp devenv/kustomize/e2e/* devenv/state/kustomize/e2e
cp devenv/state/e2e-prom-client.txt devenv/state/kustomize/e2e

cd devenv/state/kustomize/e2e # change workingdir to kustomize/e2e
kustomize edit set image placeholderfortesterimage=`cat ../../e2e-image-tag.txt`

echo "deleting any existing e2e job..."
DELETE_APPLY_RESULT=$(run_invoke $CLUSTER_NAME $CLUSTER_RESOURCE_GROUP "kubectl delete jobs app-routing-operator-e2e -n kube-system --ignore-not-found && kubectl apply -k ." ".")

cd ../.. # go back to root as working dir

# wait until cluster has reached terminated status, keep checking until terminated result is not null
NUM_ACTIVE="1"
while [ "$NUM_ACTIVE" = "1" ]
do
  # use az cli to get status from pod JSON
  STATUS=$(run_invoke $CLUSTER_NAME $CLUSTER_RESOURCE_GROUP 'kubectl get jobs --selector=app=app-routing-operator-e2e --output="jsonpath={.items[0].status}" -n kube-system' | jq ".logs" | tr -d '\\' )

  # remove quotes at prefix and suffix from output to be able to use jquery
  STATUSNOQUOTES=${STATUS#"\""}
  STATUSNOQUOTES=${STATUSNOQUOTES%"\""}

  NUM_ACTIVE=$(echo $STATUSNOQUOTES | jq '.active')
  echo "test status is currently $STATUSNOQUOTES"

  sleep 5

done

echo "exited loop with status $STATUSNOQUOTES"

SUCCEEDED=$(echo $STATUSNOQUOTES | jq ".succeeded")

echo "Test finished, echoing test pod logs..."
POD_LOGS_RESULT=$(run_invoke $CLUSTER_NAME $CLUSTER_RESOURCE_GROUP 'kubectl logs jobs/app-routing-operator-e2e -n kube-system' | tr -d '\n\r' | jq ".logs")
echo $POD_LOGS_RESULT


if [[ $SUCCEEDED == "1" ]]
then
  echo "TEST PASSED"
  exit 0
fi

echo "TEST FAILED WITH STATUS $STATUSNOQUOTES"
exit 1
