#!/bin/bash
# test_auth.sh

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m' # No Color

BASE_URL="http://localhost:8080/api/v1"
EMAIL="test@example.com"
PASSWORD="Test123!"
APP_ID="godot-app"

echo -e "${GREEN}Testing ProjectorBackend Auth API${NC}"
echo "=================================="

# Step 1: Register
echo -e "\n${GREEN}1. Registering user...${NC}"
REGISTER_RESPONSE=$(curl -s -X POST "$BASE_URL/auth/register" \
  -H "Content-Type: application/json" \
  -d "{
    \"email\": \"$EMAIL\",
    \"password\": \"$PASSWORD\",
    \"app_id\": \"$APP_ID\"
  }")

echo "Response: $REGISTER_RESPONSE"

# Step 2: Get verification code from logs (in development)
echo -e "\n${GREEN}2. Please check the logs for verification code${NC}"
echo "Run: docker-compose -f docker-compose.dev.yml logs identity-provider | grep 'Verification code'"
echo -e "\nEnter verification code: "
read CODE

# Step 3: Verify code
echo -e "\n${GREEN}3. Verifying code...${NC}"
VERIFY_RESPONSE=$(curl -s -X POST "$BASE_URL/auth/verify" \
  -H "Content-Type: application/json" \
  -d "{
    \"email\": \"$EMAIL\",
    \"code\": \"$CODE\",
    \"app_id\": \"$APP_ID\"
  }")

echo "Response: $VERIFY_RESPONSE"

# Extract token
TOKEN=$(echo $VERIFY_RESPONSE | grep -o '"token":"[^"]*' | cut -d'"' -f4)

if [ ! -z "$TOKEN" ]; then
    echo -e "\n${GREEN}✓ Token received successfully${NC}"
    echo "Token: $TOKEN"
else
    echo -e "\n${RED}✗ Failed to get token${NC}"
    exit 1
fi

# Step 4: Login with existing user
echo -e "\n${GREEN}4. Testing login...${NC}"
LOGIN_RESPONSE=$(curl -s -X POST "$BASE_URL/auth/login" \
  -H "Content-Type: application/json" \
  -d "{
    \"email\": \"$EMAIL\",
    \"password\": \"$PASSWORD\",
    \"app_id\": \"$APP_ID\"
  }")

echo "Response: $LOGIN_RESPONSE"

# Check if login successful
if echo "$LOGIN_RESPONSE" | grep -q "token"; then
    echo -e "\n${GREEN}✓ Login successful${NC}"
else
    echo -e "\n${RED}✗ Login failed${NC}"
fi