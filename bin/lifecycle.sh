#!/bin/bash

./s3-broker --config=config-sample.json

####################################################################################################################################

# Catalog
curl -H 'Accept: application/json' -H 'Content-Type: application/json' -X GET "http://username:password@localhost:3000/v2/catalog"

####################################################################################################################################

# Provision ElastiCache
curl -H 'Accept: application/json' -H 'Content-Type: application/json' -X PUT "http://username:password@localhost:3000/v2/service_instances/testElastiCache" -d '{"service_id":"9F774667-3F01-416B-8377-0CEEC10B6D11","plan_id":"1B447D14-FB5D-463E-9F0F-E8427FA8B29B","organization_guid":"organization_id","space_guid":"space_id"}'

# Bind ElastiCache
curl -H 'Accept: application/json' -H 'Content-Type: application/json' -X PUT "http://username:password@localhost:3000/v2/service_instances/testElastiCache/service_bindings/ElastiCache-binding" -d '{"service_id":"9F774667-3F01-416B-8377-0CEEC10B6D11","plan_id":"1B447D14-FB5D-463E-9F0F-E8427FA8B29B"}'
curl -H 'Accept: application/json' -H 'Content-Type: application/json' -X PUT "http://username:password@localhost:3000/v2/service_instances/testElastiCache/service_bindings/ElastiCache-binding-2" -d '{"service_id":"9F774667-3F01-416B-8377-0CEEC10B6D11","plan_id":"1B447D14-FB5D-463E-9F0F-E8427FA8B29B"}'

# Unbind ElastiCache
curl -H 'Accept: application/json' -H 'Content-Type: application/json' -X DELETE "http://username:password@localhost:3000/v2/service_instances/testElastiCache/service_bindings/ElastiCache-binding-2?service_id=c9F774667-3F01-416B-8377-0CEEC10B6D11&plan_id=1B447D14-FB5D-463E-9F0F-E8427FA8B29B"
curl -H 'Accept: application/json' -H 'Content-Type: application/json' -X DELETE "http://username:password@localhost:3000/v2/service_instances/testElastiCache/service_bindings/ElastiCache-binding?service_id=c9F774667-3F01-416B-8377-0CEEC10B6D11&plan_id=1B447D14-FB5D-463E-9F0F-E8427FA8B29B"

# Update ElastiCache
curl -H 'Accept: application/json' -H 'Content-Type: application/json' -X PATCH "http://username:password@localhost:3000/v2/service_instances/testElastiCache" -d '{"service_id":"9F774667-3F01-416B-8377-0CEEC10B6D11","plan_id":"1B447D14-FB5D-463E-9F0F-E8427FA8B29B","previous_values":{"service_id":"9F774667-3F01-416B-8377-0CEEC10B6D11","plan_id":"1B447D14-FB5D-463E-9F0F-E8427FA8B29B","organization_guid":"organization_id","space_guid":"space_id"},"parameters":{"delay_seconds":"1"}}'

# Deprovision ElastiCache
curl -H 'Accept: application/json' -H 'Content-Type: application/json' -X DELETE "http://username:password@localhost:3000/v2/service_instances/testElastiCache?service_id=9F774667-3F01-416B-8377-0CEEC10B6D11&plan_id=1B447D14-FB5D-463E-9F0F-E8427FA8B29B"

####################################################################################################################################

# Provision Errors
curl -H 'Accept: application/json' -H 'Content-Type: application/json' -X PUT "http://username:password@localhost:3000/v2/service_instances/testElastiCache" -d '{"service_id":"9F774667-3F01-416B-8377-0CEEC10B6D11","plan_id":"1B447D14-FB5D-463E-9F0F-E8427FA8B29B","organization_guid":"organization_id","space_guid":"space_id","parameters":{"((("}}'
curl -H 'Accept: application/json' -H 'Content-Type: application/json' -X PUT "http://username:password@localhost:3000/v2/service_instances/testElastiCache" -d '{"service_id":"9F774667-3F01-416B-8377-0CEEC10B6D11","plan_id":"unknown","organization_guid":"organization_id","space_guid":"space_id"}'

# Update Errors
curl -H 'Accept: application/json' -H 'Content-Type: application/json' -X PATCH "http://username:password@localhost:3000/v2/service_instances/testElastiCache" -d '{"service_id":"9F774667-3F01-416B-8377-0CEEC10B6D11","plan_id":"1B447D14-FB5D-463E-9F0F-E8427FA8B29B","previous_values":{"service_id":"9F774667-3F01-416B-8377-0CEEC10B6D11","plan_id":"1B447D14-FB5D-463E-9F0F-E8427FA8B29B","organization_guid":"organization_id","space_guid":"space_id"},"parameters":{"((("}}'
curl -H 'Accept: application/json' -H 'Content-Type: application/json' -X PATCH "http://username:password@localhost:3000/v2/service_instances/testElastiCache" -d '{"service_id":"unknown","plan_id":"1B447D14-FB5D-463E-9F0F-E8427FA8B29B","previous_values":{"service_id":"9F774667-3F01-416B-8377-0CEEC10B6D11","plan_id":"1B447D14-FB5D-463E-9F0F-E8427FA8B29B","organization_guid":"organization_id","space_guid":"space_id"},"parameters":{"delay_seconds":"1"}}'
curl -H 'Accept: application/json' -H 'Content-Type: application/json' -X PATCH "http://username:password@localhost:3000/v2/service_instances/testElastiCache" -d '{"service_id":"9F774667-3F01-416B-8377-0CEEC10B6D11","plan_id":"unknown","previous_values":{"service_id":"9F774667-3F01-416B-8377-0CEEC10B6D11","plan_id":"1B447D14-FB5D-463E-9F0F-E8427FA8B29B","organization_guid":"organization_id","space_guid":"space_id"},"parameters":{"delay_seconds":"1"}}'

# Deprovision Errors
curl -H 'Accept: application/json' -H 'Content-Type: application/json' -X DELETE "http://username:password@localhost:3000/v2/service_instances/unknown?service_id=9F774667-3F01-416B-8377-0CEEC10B6D11&plan_id=1B447D14-FB5D-463E-9F0F-E8427FA8B29B"

# Bind Errors
curl -H 'Accept: application/json' -H 'Content-Type: application/json' -X PUT "http://username:password@localhost:3000/v2/service_instances/testElastiCache/service_bindings/ElastiCache-binding" -d '{"service_id":"unknown","plan_id":"1B447D14-FB5D-463E-9F0F-E8427FA8B29B"}'
curl -H 'Accept: application/json' -H 'Content-Type: application/json' -X PUT "http://username:password@localhost:3000/v2/service_instances/unknown/service_bindings/ElastiCache-binding" -d '{"service_id":"9F774667-3F01-416B-8377-0CEEC10B6D11","plan_id":"1B447D14-FB5D-463E-9F0F-E8427FA8B29B"}'

# Unbind Errors
curl -H 'Accept: application/json' -H 'Content-Type: application/json' -X DELETE "http://username:password@localhost:3000/v2/service_instances/testElastiCache/service_bindings/unknown?service_id=c9F774667-3F01-416B-8377-0CEEC10B6D11&plan_id=1B447D14-FB5D-463E-9F0F-E8427FA8B29B"

# Last Operation Errors
curl -H 'Accept: application/json' -H 'Content-Type: application/json' -X GET "http://username:password@localhost:3000/v2/service_instances/testElastiCache/last_operation"

####################################################################################################################################
