#!/bin/bash

# Test script to verify single file download functionality

echo "========================================"
echo "Single File Download Test"
echo "========================================"
echo ""

API_BASE="${FROG_URL:-http://localhost:5800}"

# Test downloading a specific file from search result
test_single_download() {
    local model_id="$1"
    local test_name="$2"

    echo -e "\n========================================"
    echo "Test: $test_name"
    echo "Model ID: $model_id"
    echo "========================================"

    # Make the request that should trigger download
    echo "1. Requesting model (should trigger single file download):"
    response=$(curl -s -X POST "${API_BASE}/v1/chat/completions" \
        -H "Content-Type: application/json" \
        -d "{
            \"model\": \"$model_id\",
            \"messages\": [{\"role\": \"user\", \"content\": \"Hello\"}],
            \"max_tokens\": 10
        }" 2>&1)

    if echo "$response" | grep -q '"error"'; then
        error=$(echo "$response" | python3 -c "import json,sys; print(json.load(sys.stdin).get('error','Unknown')[:200])" 2>/dev/null)
        echo "Response: $error"

        if echo "$error" | grep -q "downloading"; then
            echo "⏳ Model is downloading..."
            echo "Waiting 30 seconds for download to complete..."
            sleep 30
        else
            echo "❌ Error occurred: $error"
            return 1
        fi
    elif echo "$response" | grep -q '"choices"'; then
        echo "✅ Model responded successfully (was already downloaded)"
        return 0
    fi

    # Second attempt after download
    echo -e "\n2. Second request (should use downloaded file):"
    response2=$(curl -s -X POST "${API_BASE}/v1/chat/completions" \
        -H "Content-Type: application/json" \
        -d "{
            \"model\": \"$model_id\",
            \"messages\": [{\"role\": \"user\", \"content\": \"Hi\"}],
            \"max_tokens\": 10
        }" 2>&1)

    if echo "$response2" | grep -q '"error"'; then
        error=$(echo "$response2" | python3 -c "import json,sys; print(json.load(sys.stdin).get('error','Unknown'))" 2>/dev/null)
        echo "❌ Still failing after download: $error"
        return 1
    elif echo "$response2" | grep -q '"choices"'; then
        echo "✅ Model works after download!"
        return 0
    fi
}

# Test 1: Specific file format (from search API)
echo "Testing specific file download..."
test_single_download \
    "Qwen/Qwen2.5-0.5B-Instruct-GGUF:qwen2.5-0.5b-instruct-q4_k_m.gguf" \
    "Specific file format"

# Test 2: Quantization format (should download first matching file)
echo -e "\nTesting quantization-based download..."
test_single_download \
    "TinyLlama/TinyLlama-1.1B-Chat-v1.0-GGUF:q4_k_m" \
    "Quantization format (first file only)"

# Check how many files were actually downloaded
echo -e "\n========================================"
echo "Checking downloaded files..."
echo "========================================"

if [ -d models ]; then
    echo "Downloaded models:"
    for dir in models/*/; do
        if [ -d "$dir" ]; then
            echo -e "\n$(basename "$dir"):"
            file_count=$(find "$dir" -name "*.gguf" -type f 2>/dev/null | wc -l)
            echo "  Files downloaded: $file_count"
            find "$dir" -name "*.gguf" -type f 2>/dev/null | while read -r file; do
                size=$(ls -lh "$file" | awk '{print $5}')
                echo "  - $(basename "$file") ($size)"
            done
        fi
    done
else
    echo "No models directory found"
fi

echo -e "\n========================================"
echo "Test Complete!"
echo "========================================"
echo ""
echo "Expected behavior:"
echo "1. Each model request downloads ONLY ONE file"
echo "2. Specific file requests get exact file"
echo "3. Quantization requests get first matching file"
echo "4. No duplicate downloads on subsequent requests"
echo ""