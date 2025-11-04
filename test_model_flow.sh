#!/bin/bash

# Test the complete flow for the bartowski model

echo "========================================"
echo "🐸 Complete Model Flow Test"
echo "========================================"
echo ""

API_BASE="${FROG_URL:-http://localhost:5800}"
MODEL_ID="bartowski/Mistral-22B-v0.1-GGUF:q5_k"
CONFIG_ID="bartowski-mistral-22b-v0.1-gguf-q5_k"

# Color codes
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

echo -e "${BLUE}Testing model: $MODEL_ID${NC}"
echo -e "${BLUE}Expected config ID: $CONFIG_ID${NC}"
echo ""

# Step 1: Clear any existing model from config
echo -e "${YELLOW}Step 1: Checking initial state${NC}"
echo "----------------------------------------"
if grep -q "$CONFIG_ID" config.yaml 2>/dev/null; then
    echo -e "${YELLOW}Model already in config, showing current entry:${NC}"
    grep -A 20 "$CONFIG_ID" config.yaml
else
    echo "Model not in config (clean state)"
fi

# Step 2: Make request to trigger download
echo -e "\n${YELLOW}Step 2: Making request (may trigger download)${NC}"
echo "----------------------------------------"
response=$(curl -s -X POST "${API_BASE}/v1/chat/completions" \
    -H "Content-Type: application/json" \
    -d "{
        \"model\": \"$MODEL_ID\",
        \"messages\": [{\"role\": \"user\", \"content\": \"Say hello\"}],
        \"max_tokens\": 5,
        \"temperature\": 0.1
    }" 2>&1)

if echo "$response" | grep -q '"error"'; then
    error=$(echo "$response" | python3 -c "import json,sys; print(json.load(sys.stdin).get('error','Unknown')[:200])" 2>/dev/null || echo "Parse error")

    if echo "$error" | grep -q "duplicate alias"; then
        echo -e "${RED}❌ Duplicate alias error!${NC}"
        echo "Error: $error"
        exit 1
    elif echo "$error" | grep -q "could not find process group"; then
        echo -e "${RED}❌ Process group error!${NC}"
        echo "Error: $error"
        exit 1
    elif echo "$error" | grep -qi "downloading\|not found locally"; then
        echo -e "${YELLOW}⏳ Model is downloading...${NC}"
        echo "Waiting 30 seconds for download to complete..."
        sleep 30
    else
        echo -e "${RED}❌ Unexpected error: $error${NC}"
    fi
elif echo "$response" | grep -q '"choices"'; then
    echo -e "${GREEN}✅ Model responded immediately${NC}"
fi

# Step 3: Check config.yaml
echo -e "\n${YELLOW}Step 3: Checking config.yaml${NC}"
echo "----------------------------------------"

if grep -q "$CONFIG_ID" config.yaml 2>/dev/null; then
    echo -e "${GREEN}✅ Model found in config with ID: $CONFIG_ID${NC}"

    # Check aliases
    echo -e "\n${BLUE}Checking aliases:${NC}"
    grep -A 20 "$CONFIG_ID" config.yaml | grep -A 5 "aliases:"

    # Check if in group
    echo -e "\n${BLUE}Checking group membership:${NC}"
    if grep -A 10 "all-models:" config.yaml | grep -q "$CONFIG_ID"; then
        echo -e "${GREEN}✅ Model is in all-models group${NC}"
    else
        echo -e "${RED}❌ Model NOT in all-models group${NC}"
    fi

    # Check model path
    echo -e "\n${BLUE}Checking model path:${NC}"
    grep -A 10 "$CONFIG_ID" config.yaml | grep "model" | head -1

else
    echo -e "${RED}❌ Model NOT found in config${NC}"
    echo "Looking for any bartowski entries:"
    grep -i "bartowski" config.yaml || echo "No bartowski entries found"
fi

# Step 4: Make second request (should work without download)
echo -e "\n${YELLOW}Step 4: Second request (should use existing model)${NC}"
echo "----------------------------------------"
response=$(curl -s -X POST "${API_BASE}/v1/chat/completions" \
    -H "Content-Type: application/json" \
    -d "{
        \"model\": \"$MODEL_ID\",
        \"messages\": [{\"role\": \"user\", \"content\": \"Count to 3\"}],
        \"max_tokens\": 20,
        \"temperature\": 0.1
    }" 2>&1)

if echo "$response" | grep -q '"error"'; then
    error=$(echo "$response" | python3 -c "import json,sys; print(json.load(sys.stdin).get('error','Unknown'))" 2>/dev/null)
    echo -e "${RED}❌ Error on second request: $error${NC}"

    # Debug: Check what models are available
    echo -e "\n${YELLOW}Available models:${NC}"
    curl -s "${API_BASE}/v1/models" | python3 -m json.tool | grep '"id"' | head -10

elif echo "$response" | grep -q '"choices"'; then
    echo -e "${GREEN}✅ Second request successful!${NC}"
    content=$(echo "$response" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d['choices'][0]['message']['content'][:50] if 'choices' in d else '')" 2>/dev/null)
    if [ ! -z "$content" ]; then
        echo "Model responded: $content"
    fi
fi

# Step 5: Test with different formats
echo -e "\n${YELLOW}Step 5: Testing different ID formats${NC}"
echo "----------------------------------------"

# Test with base model ID
echo -e "${BLUE}Testing: bartowski/Mistral-22B-v0.1-GGUF${NC}"
response=$(curl -s -X POST "${API_BASE}/v1/chat/completions" \
    -H "Content-Type: application/json" \
    -d "{
        \"model\": \"bartowski/Mistral-22B-v0.1-GGUF\",
        \"messages\": [{\"role\": \"user\", \"content\": \"Hi\"}],
        \"max_tokens\": 5,
        \"temperature\": 0.1
    }" 2>&1)

if echo "$response" | grep -q '"choices"'; then
    echo -e "${GREEN}✅ Base model ID works${NC}"
elif echo "$response" | grep -q '"error"'; then
    echo -e "${RED}❌ Base model ID failed${NC}"
fi

# Test with config ID directly
echo -e "\n${BLUE}Testing: $CONFIG_ID${NC}"
response=$(curl -s -X POST "${API_BASE}/v1/chat/completions" \
    -H "Content-Type: application/json" \
    -d "{
        \"model\": \"$CONFIG_ID\",
        \"messages\": [{\"role\": \"user\", \"content\": \"Hi\"}],
        \"max_tokens\": 5,
        \"temperature\": 0.1
    }" 2>&1)

if echo "$response" | grep -q '"choices"'; then
    echo -e "${GREEN}✅ Config ID works directly${NC}"
elif echo "$response" | grep -q '"error"'; then
    echo -e "${RED}❌ Config ID failed${NC}"
fi

# Final Summary
echo -e "\n${YELLOW}========================================"
echo "FINAL VERIFICATION"
echo "========================================${NC}"

CHECKS_PASSED=0
CHECKS_FAILED=0

# Check 1: Model in config
if grep -q "$CONFIG_ID" config.yaml 2>/dev/null; then
    echo -e "${GREEN}✅ Model in config${NC}"
    CHECKS_PASSED=$((CHECKS_PASSED + 1))
else
    echo -e "${RED}❌ Model NOT in config${NC}"
    CHECKS_FAILED=$((CHECKS_FAILED + 1))
fi

# Check 2: Model has aliases
if grep -A 20 "$CONFIG_ID" config.yaml 2>/dev/null | grep -q "aliases:"; then
    echo -e "${GREEN}✅ Model has aliases${NC}"
    CHECKS_PASSED=$((CHECKS_PASSED + 1))
else
    echo -e "${RED}❌ Model has NO aliases${NC}"
    CHECKS_FAILED=$((CHECKS_FAILED + 1))
fi

# Check 3: Model in group
if grep -A 10 "all-models:" config.yaml 2>/dev/null | grep -q "$CONFIG_ID"; then
    echo -e "${GREEN}✅ Model in group${NC}"
    CHECKS_PASSED=$((CHECKS_PASSED + 1))
else
    echo -e "${RED}❌ Model NOT in group${NC}"
    CHECKS_FAILED=$((CHECKS_FAILED + 1))
fi

# Check 4: Model works
if echo "$response" | grep -q '"choices"'; then
    echo -e "${GREEN}✅ Model responds to requests${NC}"
    CHECKS_PASSED=$((CHECKS_PASSED + 1))
else
    echo -e "${RED}❌ Model does NOT respond${NC}"
    CHECKS_FAILED=$((CHECKS_FAILED + 1))
fi

echo -e "\n${BLUE}Checks Passed: $CHECKS_PASSED/4${NC}"
echo -e "${BLUE}Checks Failed: $CHECKS_FAILED/4${NC}"

if [ $CHECKS_FAILED -eq 0 ]; then
    echo -e "\n${GREEN}🎉 ALL CHECKS PASSED! The model flow works correctly!${NC}"
else
    echo -e "\n${RED}⚠️ Some checks failed. Please review the output above.${NC}"
fi

echo ""