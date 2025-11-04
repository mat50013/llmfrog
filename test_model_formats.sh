#!/bin/bash

# Test script to verify all model ID formats work correctly
# Ensure FrogLLM is running before executing this script

echo "========================================"
echo "Testing Model ID Format Support"
echo "========================================"

API_BASE="http://localhost:5800"

# Function to test a model ID
test_model() {
    local model_id="$1"
    local description="$2"

    echo -e "\n----------------------------------------"
    echo "Testing: $description"
    echo "Model ID: $model_id"
    echo "----------------------------------------"

    response=$(curl -s -X POST "$API_BASE/v1/chat/completions" \
        -H "Content-Type: application/json" \
        -d "{
            \"model\": \"$model_id\",
            \"messages\": [
                {
                    \"role\": \"user\",
                    \"content\": \"Say 'Hello' and nothing else.\"
                }
            ],
            \"temperature\": 0.1,
            \"max_tokens\": 10
        }" 2>&1)

    # Check if response contains an error
    if echo "$response" | grep -q '"error"'; then
        echo "Result: ❌ Failed"
        echo "$response" | python3 -m json.tool 2>/dev/null | grep -A2 '"error"' || echo "$response"
    else
        echo "Result: ✅ Success (or download initiated)"
        # Show first 200 chars of response
        echo "$response" | head -c 200
        echo "..."
    fi
}

# Test 1: Search for models first
echo -e "\n========================================"
echo "Step 1: Search for Available Models"
echo "========================================"

echo -e "\nSearching for Qwen models..."
search_response=$(curl -s "$API_BASE/api/v1/models/search?q=qwen+0.5b&limit=3")
echo "$search_response" | python3 -m json.tool | head -20

# Extract a model ID from search results (if available)
model_from_search=$(echo "$search_response" | python3 -c "
import json, sys
try:
    data = json.load(sys.stdin)
    if data.get('models') and len(data['models']) > 0:
        print(data['models'][0]['id'])
except:
    pass
" 2>/dev/null)

echo -e "\n========================================"
echo "Step 2: Test Different Model ID Formats"
echo "========================================"

# Test format 1: Traditional repo/model format
test_model "Qwen/Qwen2.5-0.5B-Instruct-GGUF" "Traditional repo format (will download all GGUF files)"

# Test format 2: repo:filename format (from search API)
if [ ! -z "$model_from_search" ]; then
    test_model "$model_from_search" "Model ID from search API (repo:filename format)"
else
    test_model "Qwen/Qwen2.5-0.5B-Instruct-GGUF:qwen2.5-0.5b-instruct-q4_k_m.gguf" "Manual repo:filename format"
fi

# Test format 3: repo:quantization format
test_model "Qwen/Qwen2.5-0.5B-Instruct-GGUF:q4_k_m" "Repo with quantization format"

# Test format 4: Invalid format (should fail gracefully)
test_model "invalid-model-id" "Invalid model ID (should fail)"

echo -e "\n========================================"
echo "Step 3: Check Loaded Models"
echo "========================================"

echo -e "\nGetting currently loaded models..."
curl -s "$API_BASE/api/v1/models/loaded" | python3 -m json.tool

echo -e "\n========================================"
echo "Testing Complete!"
echo "========================================"
echo ""
echo "Summary:"
echo "1. Search API returns models in 'repo:filename' format"
echo "2. Chat completions API accepts:"
echo "   - 'repo/model' format (downloads all GGUF files)"
echo "   - 'repo:filename.gguf' format (downloads specific file)"
echo "   - 'repo:quantization' format (downloads matching quantization)"
echo ""