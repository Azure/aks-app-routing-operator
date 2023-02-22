#!/usr/bin/env bash
run_invoke () {
  CLUSTER_NAME=$1
  CLUSTER_RESOURCE_GROUP=$2
  KUBECTL_CMD=$3
  FILEPATH=$4

  KUBECTL_CMD_RESULT=""
  if [ -z $FILEPATH  ]; then
    KUBECTL_CMD_RESULT=$(az aks command invoke -o json --resource-group $CLUSTER_RESOURCE_GROUP --name $CLUSTER_NAME --command "$KUBECTL_CMD")
  else
    KUBECTL_CMD_RESULT=$(az aks command invoke -o json --resource-group $CLUSTER_RESOURCE_GROUP --name $CLUSTER_NAME --command "$KUBECTL_CMD" --file $FILEPATH)
  fi

  FORMATTED_RESULT=$(echo $KUBECTL_CMD_RESULT | tr -d '\n\r')
  KUBECTL_EXIT_CODE=$(echo $FORMATTED_RESULT | jq '.exitCode')

  if [ $KUBECTL_EXIT_CODE != "0" ]
  then
    echo "Failed to run kubectl command $KUBECTL_CMD"
    echo $KUBECTL_CMD_RESULT
    exit $KUBECTL_EXIT_CODE
  fi

  echo $KUBECTL_CMD_RESULT

}