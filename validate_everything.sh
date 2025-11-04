#!/bin/bash

# Final validation that everything works correctly
# This test ensures the complete flow works as expected

echo "========================================"
echo "🐸 FrogLLM Complete Validation Test"
echo "========================================"
echo ""
echo "This test validates:"
echo "1. Model downloads correctly"
echo "2. Config is updated with correct ID and aliases"
echo "3. Model is added to groups"
echo "4. Process groups are created"
echo "5. No duplicate alias errors"
echo "6. Model responds to requests"
echo ""

API_BASE="${FROG_URL:-http://localhost:5800}"

# Color codes
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

TOTAL_TESTS=0
PASSED_TESTS=0
FAILED_TESTS=0

# Function to run a test
run_test() {
    local test_name="$1"
    local command="$2"
    local expected_result="$3"

    echo -e "\n${BLUE}Test: $test_name${NC}"
    TOTAL_TESTS=$((TOTAL_TESTS + 1))

    if eval "$command"; then
        if [ "$expected_result" = "pass" ]; then
            echo -e "${GREEN}✅ PASSED${NC}"
            PASSED_TESTS=$((PASSED_TESTS + 1))
        else
            echo -e "${RED}❌ FAILED (expected to fail but passed)${NC}"
            FAILED_TESTS=$((FAILED_TESTS + 1))
        fi
    else
        if [ "$expected_result" = "fail" ]; then
            echo -e "${GREEN}✅ PASSED (correctly failed)${NC}"
            PASSED_TESTS=$((PASSED_TESTS + 1))
        else
            echo -e "${RED}❌ FAILED${NC}"
            FAILED_TESTS=$((FAILED_TESTS + 1))
        fi
    fi
}

# Test 1: Bartowski model with q5_k quantization
echo -e "${YELLOW}========================================${NC}"
echo -e "${YELLOW}Test Suite 1: Bartowski Model${NC}"
echo -e "${YELLOW}========================================${NC}"

MODEL_ID="bartowski/Mistral-22B-v0.1-GGUF:q5_k"
CONFIG_ID="bartowski-mistral-22b-v0.1-gguf-q5_k"

# First request
echo -e "\n${BLUE}Making first request to $MODEL_ID${NC}"
response=$(curl -s -X POST "${API_BASE}/v1/chat/completions" \
    -H "Content-Type: application/json" \
    -d "{
        \"model\": \"$MODEL_ID\",
        \"messages\": [{\"role\": \"user\", \"content\": \"Say hello\"}],
        \"max_tokens\": 5,
        \"temperature\": 0.1
    }" 2>&1)

# Check for errors
if echo "$response" | grep -q "duplicate alias"; then
    echo -e "${RED}❌ CRITICAL: Duplicate alias error!${NC}"
    FAILED_TESTS=$((FAILED_TESTS + 1))
elif echo "$response" | grep -q "could not find process group"; then
    echo -e "${RED}❌ CRITICAL: Process group error!${NC}"
    FAILED_TESTS=$((FAILED_TESTS + 1))
elif echo "$response" | grep -q '"error"'; then
    error=$(echo "$response" | python3 -c "import json,sys; print(json.load(sys.stdin).get('error','')[:100])" 2>/dev/null)
    if echo "$error" | grep -qi "downloading\|not found"; then
        echo -e "${YELLOW}Model downloading (expected on first run)...${NC}"
        echo "Waiting 30 seconds..."
        sleep 30
    else
        echo -e "${RED}Error: $error${NC}"
        FAILED_TESTS=$((FAILED_TESTS + 1))
    fi
else
    echo -e "${GREEN}✅ First request successful${NC}"
    PASSED_TESTS=$((PASSED_TESTS + 1))
fi

# Validation checks
echo -e "\n${YELLOW}Running validation checks...${NC}"

# Check 1: Model in config
run_test "Model exists in config.yaml" \
    "grep -q '$CONFIG_ID' config.yaml 2>/dev/null" \
    "pass"

# Check 2: Model has correct aliases
run_test "Model has alias for $MODEL_ID" \
    "grep -A 20 '$CONFIG_ID' config.yaml 2>/dev/null | grep -q '$MODEL_ID'" \
    "pass"

# Check 3: Model in group
run_test "Model is in all-models group" \
    "grep -A 20 'all-models:' config.yaml 2>/dev/null | grep -q '$CONFIG_ID'" \
    "pass"

# Check 4: Second request works
echo -e "\n${BLUE}Making second request to verify model works${NC}"
response=$(curl -s -X POST "${API_BASE}/v1/chat/completions" \
    -H "Content-Type: application/json" \
    -d "{
        \"model\": \"$MODEL_ID\",
        \"messages\": [{\"role\": \"user\", \"content\": \"Count to 3\"}],
        \"max_tokens\": 20,
        \"temperature\": 0.1
    }" 2>&1)

run_test "Second request succeeds" \
    "echo '$response' | grep -q '\"choices\"'" \
    "pass"

# Check 5: No duplicate alias error on third request
echo -e "\n${BLUE}Making third request to ensure no duplicate issues${NC}"
response=$(curl -s -X POST "${API_BASE}/v1/chat/completions" \
    -H "Content-Type: application/json" \
    -d "{
        \"model\": \"$MODEL_ID\",
        \"messages\": [{\"role\": \"user\", \"content\": \"What is 2+2?\"}],
        \"max_tokens\": 10,
        \"temperature\": 0.1
    }" 2>&1)

run_test "No duplicate alias error" \
    "! echo '$response' | grep -q 'duplicate alias'" \
    "pass"

# Test 2: Different quantization formats
echo -e "\n${YELLOW}========================================${NC}"
echo -e "${YELLOW}Test Suite 2: Different Formats${NC}"
echo -e "${YELLOW}========================================${NC}"

# Test with base repo format
echo -e "\n${BLUE}Testing base repo format${NC}"
response=$(curl -s -X POST "${API_BASE}/v1/chat/completions" \
    -H "Content-Type: application/json" \
    -d "{
        \"model\": \"bartowski/Mistral-22B-v0.1-GGUF\",
        \"messages\": [{\"role\": \"user\", \"content\": \"Hi\"}],
        \"max_tokens\": 5
    }" 2>&1)

run_test "Base repo format works" \
    "echo '$response' | grep -q '\"choices\"'" \
    "pass"

# Test with config ID directly
echo -e "\n${BLUE}Testing config ID directly${NC}"
response=$(curl -s -X POST "${API_BASE}/v1/chat/completions" \
    -H "Content-Type: application/json" \
    -d "{
        \"model\": \"$CONFIG_ID\",
        \"messages\": [{\"role\": \"user\", \"content\": \"Hi\"}],
        \"max_tokens\": 5
    }" 2>&1)

run_test "Config ID works directly" \
    "echo '$response' | grep -q '\"choices\"'" \
    "pass"

# Test 3: File system checks
echo -e "\n${YELLOW}========================================${NC}"
echo -e "${YELLOW}Test Suite 3: File System${NC}"
echo -e "${YELLOW}========================================${NC}"

# Check downloaded files
run_test "Model file exists in models directory" \
    "find models -name '*.gguf' -type f 2>/dev/null | grep -q 'Mistral-22B'" \
    "pass"

# Final Summary
echo -e "\n${YELLOW}========================================"
echo "📊 VALIDATION SUMMARY"
echo "========================================${NC}"

echo -e "Total Tests: ${BLUE}$TOTAL_TESTS${NC}"
echo -e "Passed: ${GREEN}$PASSED_TESTS${NC}"
echo -e "Failed: ${RED}$FAILED_TESTS${NC}"

if [ $FAILED_TESTS -eq 0 ]; then
    echo -e "\n${GREEN}🎉 ALL VALIDATION TESTS PASSED!${NC}"
    echo ""
    echo "The system is working correctly:"
    echo "✅ Models download properly"
    echo "✅ Config is updated with correct IDs and aliases"
    echo "✅ Models are added to groups"
    echo "✅ Process groups are created"
    echo "✅ No duplicate alias errors"
    echo "✅ All ID formats work (repo:quant, repo, config ID)"
    echo ""
    echo "The FrogLLM model management system is fully functional!"
else
    echo -e "\n${RED}⚠️ SOME TESTS FAILED${NC}"
    echo "Please review the failures above."
    echo ""
    echo "Common issues to check:"
    echo "1. Is FrogLLM running? (./frogllm)"
    echo "2. Is the API accessible at $API_BASE?"
    echo "3. Check server logs for detailed errors"
    echo "4. Ensure models directory is writable"
fi

echo ""