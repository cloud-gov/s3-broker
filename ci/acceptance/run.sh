#!/bin/bash

set -eux

cf api "${CF_API_URL}"
(set +x; cf auth "$CF_USERNAME" "$CF_PASSWORD")

cf target -o "$CF_ORGANIZATION"

cf target -s "$CF_SPACE"

pushd broker-src/ci/acceptance
  cf push "$APP_NAME" -m "${MEMORY_LIMIT:-"128M"}" --no-start
popd

cf set-env "$APP_NAME" SERVICE_NAME "$SERVICE_NAME"
cf set-env "$APP_NAME" IS_PUBLIC "${IS_PUBLIC:-"false"}"
cf set-env "$APP_NAME" IS_DELETE "${IS_DELETE:-"false"}"
cf set-env "$APP_NAME" ENCRYPTION "${ENCRYPTION:-""}"
cf create-service "$SERVICE_NAME" "$PLAN_NAME" "$SERVICE_INSTANCE_NAME"

ADDITIONAL_INSTANCE_NAME="${ADDITIONAL_INSTANCE_NAME:-}"
if [[ -n "$ADDITIONAL_INSTANCE_NAME" ]]; then
  cf set-env "$APP_NAME" ADDITIONAL_INSTANCE_NAME "$ADDITIONAL_INSTANCE_NAME"
  cf create-service "$SERVICE_NAME" "$PLAN_NAME" "$ADDITIONAL_INSTANCE_NAME"
  cf bind-service "$APP_NAME" "$SERVICE_INSTANCE_NAME" -c '{"additional_instances": ["'"$ADDITIONAL_INSTANCE_NAME"'"]}'
else
  cf bind-service "$APP_NAME" "$SERVICE_INSTANCE_NAME"
fi

cf start "$APP_NAME"

url=$(cf app "$APP_NAME" | grep -e "urls: " -e "routes: " | awk '{print $2}')
status=$(curl -w "%{http_code}" "https://${url}")
if [ "${status}" != "200" ]; then
  echo "Unexpected status code ${status}"
  cf logs "$APP_NAME" --recent
  exit 1
fi

cf create-service-key "$SERVICE_INSTANCE_NAME" "${SERVICE_INSTANCE_NAME}-service-key"
cf service-keys "$SERVICE_INSTANCE_NAME"
cf delete-service-key -f "$SERVICE_INSTANCE_NAME" "${SERVICE_INSTANCE_NAME}-service-key"

cf delete -f "$APP_NAME"
cf delete-service -f "$SERVICE_INSTANCE_NAME"
if [[ -n "$ADDITIONAL_INSTANCE_NAME" ]]; then
  cf delete-service -f "$ADDITIONAL_INSTANCE_NAME"
fi
