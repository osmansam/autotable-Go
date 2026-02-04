#!/bin/bash

# Multi-File Excel Upload Test Script
# This script demonstrates how to use the multi-file Excel upload endpoint

# Configuration
TENANT_SLUG="myTenant"
PROJECT_SLUG="myProject"
BASE_URL="http://localhost:4000/api/v1"
TOKEN="YOUR_AUTH_TOKEN_HERE"

# Color codes for output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

echo -e "${BLUE}======================================${NC}"
echo -e "${BLUE}Multi-File Excel Upload Test${NC}"
echo -e "${BLUE}======================================${NC}"

# Test 1: Upload multiple files
echo -e "\n${YELLOW}Test 1: Upload multiple Excel files with relationships${NC}"
echo -e "${GREEN}Uploading users.xlsx, orders.xlsx, and products.xlsx...${NC}"

RESPONSE=$(curl -s -X POST "${BASE_URL}/${TENANT_SLUG}/${PROJECT_SLUG}/excel/upload-multiple" \
  -H "Authorization: Bearer ${TOKEN}" \
  -F "files=@test_data/users.xlsx" \
  -F "files=@test_data/orders.xlsx" \
  -F "files=@test_data/products.xlsx")

echo -e "${GREEN}Response:${NC}"
echo "$RESPONSE" | jq '.'

# Extract container IDs
USERS_CONTAINER=$(echo "$RESPONSE" | jq -r '.data.containers[] | select(.schemaName=="users") | .containerId')
ORDERS_CONTAINER=$(echo "$RESPONSE" | jq -r '.data.containers[] | select(.schemaName=="orders") | .containerId')
PRODUCTS_CONTAINER=$(echo "$RESPONSE" | jq -r '.data.containers[] | select(.schemaName=="products") | .containerId')

echo -e "\n${GREEN}Created Containers:${NC}"
echo -e "Users Container ID: ${BLUE}${USERS_CONTAINER}${NC}"
echo -e "Orders Container ID: ${BLUE}${ORDERS_CONTAINER}${NC}"
echo -e "Products Container ID: ${BLUE}${PRODUCTS_CONTAINER}${NC}"

# Display detected relationships
echo -e "\n${GREEN}Detected Relationships:${NC}"
echo "$RESPONSE" | jq -r '.data.relationships[]' | while read -r rel; do
  echo -e "${YELLOW}$rel${NC}"
done

# Test 2: Verify the created schemas
echo -e "\n${YELLOW}Test 2: Verify created schemas${NC}"

if [ ! -z "$ORDERS_CONTAINER" ]; then
  echo -e "${GREEN}Fetching orders container schema...${NC}"
  curl -s -X GET "${BASE_URL}/${TENANT_SLUG}/${PROJECT_SLUG}/container/${ORDERS_CONTAINER}" \
    -H "Authorization: Bearer ${TOKEN}" | jq '.data.fields[] | select(.type=="reference")'
fi

# Test 3: Query the data
echo -e "\n${YELLOW}Test 3: Query uploaded data${NC}"

if [ ! -z "$USERS_CONTAINER" ]; then
  echo -e "${GREEN}Fetching users data...${NC}"
  curl -s -X GET "${BASE_URL}/${TENANT_SLUG}/${PROJECT_SLUG}/dynamic/users" \
    -H "Authorization: Bearer ${TOKEN}" | jq '.data | length' | xargs -I {} echo "Total users: {}"
fi

if [ ! -z "$ORDERS_CONTAINER" ]; then
  echo -e "${GREEN}Fetching orders data...${NC}"
  curl -s -X GET "${BASE_URL}/${TENANT_SLUG}/${PROJECT_SLUG}/dynamic/orders" \
    -H "Authorization: Bearer ${TOKEN}" | jq '.data | length' | xargs -I {} echo "Total orders: {}"
fi

echo -e "\n${BLUE}======================================${NC}"
echo -e "${GREEN}Test completed!${NC}"
echo -e "${BLUE}======================================${NC}"

# Instructions
echo -e "\n${YELLOW}Next Steps:${NC}"
echo "1. Review the detected relationships in the response"
echo "2. Check that reference fields have 'type: reference' and 'objectSchemaName' set"
echo "3. Query the data using the dynamic endpoints"
echo "4. Update reference fields with actual MongoDB ObjectIDs if needed"
