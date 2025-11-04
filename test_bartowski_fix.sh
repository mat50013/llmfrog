#!/bin/bash

# Quick test to verify the bartowski model duplicate alias issue is fixed

echo "========================================"
echo "🐸 Bartowski Model Fix Test"
echo "========================================"
echo "Testing that bartowski/Mistral-22B-v0.1-GGUF:q5_k"
echo "works without duplicate alias errors"
echo ""

API_BASE="${FROG_URL:-http://localhost:5800}"
MODEL_ID="bartowski/Mistral-22B-v0.1-GGUF:q5_k"

# Color codes
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo "Test 1: First request (may trigger download)"
echo "----------------------------------------"

response=$(curl -s -X POST "${API_BASE}/v1/chat/completions" \
    -H "Content-Type: application/json" \
    -d "{
        \"model\": \"$MODEL_ID\",
        \"messages\": [{\"role\": \"user\", \"content\": \"Count to 3\"}],
        \"max_tokens\": 20,
        \"temperature\": 0.1
    }" 2>&1)

if echo "$response" | grep -q "duplicate alias"; then
    echo -e "${RED}❌ FAILED: Duplicate alias error on first request${NC}"
    echo "Error details:"
    echo "$response" | python3 -c "import json,sys; print(json.load(sys.stdin).get('error','Unknown'))" 2>/dev/null || echo "$response"
    exit 1
elif echo "$response" | grep -q '"error"'; then
    error=$(echo "$response" | python3 -c "import json,sys; print(json.load(sys.stdin).get('error','Unknown')[:200])" 2>/dev/null)
    echo -e "${YELLOW}⏳ Model downloading or other error: $error${NC}"
    echo "Waiting 20 seconds for download..."
    sleep 20
elif echo "$response" | grep -q '"choices"'; then
    echo -e "${GREEN}✅ First request successful!${NC}"
fi

echo -e "\nTest 2: Second request (should use existing model)"
echo "----------------------------------------"

response=$(curl -s -X POST "${API_BASE}/v1/chat/completions" \
    -H "Content-Type: application/json" \
    -d "{
        \"model\": \"$MODEL_ID\",
        \"messages\": [{\"role\": \"user\", \"content\": \"Say hello\"}],
        \"max_tokens\": 10,
        \"temperature\": 0.1
    }" 2>&1)

if echo "$response" | grep -q "duplicate alias"; then
    echo -e "${RED}❌ FAILED: Duplicate alias error on second request${NC}"
    echo "Error details:"
    echo "$response" | python3 -c "import json,sys; print(json.load(sys.stdin).get('error','Unknown'))" 2>/dev/null || echo "$response"
    exit 1
elif echo "$response" | grep -q '"error"'; then
    error=$(echo "$response" | python3 -c "import json,sys; print(json.load(sys.stdin).get('error','Unknown'))" 2>/dev/null)
    echo -e "${RED}❌ Error: $error${NC}"
    exit 1
elif echo "$response" | grep -q '"choices"'; then
    echo -e "${GREEN}✅ Second request successful!${NC}"
    content=$(echo "$response" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d['choices'][0]['message']['content'][:50] if 'choices' in d else '')" 2>/dev/null)
    if [ ! -z "$content" ]; then
        echo "Model responded: $content"
    fi
fi

echo -e "\nTest 3: Third request with same ID"
echo "----------------------------------------"

response=$(curl -s -X POST "${API_BASE}/v1/chat/completions" \
    -H "Content-Type: application/json" \
    -d "{
        \"model\": \"$MODEL_ID\",
        \"messages\": [{\"role\": \"user\", \"content\": \"What is 2+2?\"}],
        \"max_tokens\": 10,
        \"temperature\": 0.1
    }" 2>&1)

if echo "$response" | grep -q "duplicate alias"; then
    echo -e "${RED}❌ FAILED: Duplicate alias error still occurring${NC}"
    exit 1
elif echo "$response" | grep -q '"choices"'; then
    echo -e "${GREEN}✅ Third request successful!${NC}"
fi

# Check if model is in config
echo -e "\nChecking config.yaml for model entry..."
echo "----------------------------------------"

if [ -f config.yaml ]; then
    if grep -q "bartowski" config.yaml; then
        echo -e "${GREEN}✅ Model found in config${NC}"
        echo "Aliases for this model:"
        grep -A 10 "bartowski" config.yaml | grep -A 5 "aliases:" | head -8
    else
        echo -e "${YELLOW}⚠️ Model not found in config (might be using different ID)${NC}"
    fi
fi

# Final result
echo -e "\n========================================"
echo "TEST RESULT"
echo "========================================"

if echo "$response" | grep -q '"choices"'; then
    echo -e "${GREEN}🎉 SUCCESS!${NC}"
    echo "The bartowski model works without duplicate alias errors!"
    echo "The fix is working correctly."
else
    echo -e "${RED}⚠️ PROBLEM DETECTED${NC}"
    echo "Please check the errors above."
fi

echo ""