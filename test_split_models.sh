#!/bin/bash

# Test script to verify split model downloads work correctly

echo "========================================"
echo "Split Model Download Test"
echo "========================================"
echo ""

API_BASE="${FROG_URL:-http://localhost:5800}"

# First, search to see available models
echo "1. Searching for models to see individual IDs..."
echo "========================================"

curl -s "${API_BASE}/api/v1/models/search?q=llama+70b&limit=10" | python3 -c "
import json, sys

try:
    data = json.load(sys.stdin)
    models = data.get('models', [])

    print(f'Found {len(models)} models')
    print()

    for i, model in enumerate(models[:5], 1):
        print(f'{i}. ID: {model.get(\"id\")}')
        print(f'   Name: {model.get(\"name\")}')
        print(f'   Size: {model.get(\"size_gb\", 0):.2f} GB')
        print(f'   File: {model.get(\"file\", \"N/A\")}')
        print()
except Exception as e:
    print(f'Error: {e}')
"

echo ""
echo "2. Testing model download with specific quantization..."
echo "========================================"

test_model_download() {
    local model_id="$1"
    local description="$2"

    echo "Testing: $description"
    echo "Model ID: $model_id"
    echo ""

    # Make a request that should trigger download
    response=$(curl -s -X POST "${API_BASE}/v1/chat/completions" \
        -H "Content-Type: application/json" \
        -d "{
            \"model\": \"$model_id\",
            \"messages\": [{\"role\": \"user\", \"content\": \"Hello\"}],
            \"max_tokens\": 5
        }" 2>&1)

    if echo "$response" | grep -q '"error"'; then
        error=$(echo "$response" | python3 -c "import json,sys; print(json.load(sys.stdin).get('error','Unknown')[:100])" 2>/dev/null)
        echo "Response: $error"

        if echo "$error" | grep -q "downloading\|Download"; then
            echo "⏳ Model is downloading (this is expected for split models)..."
            return 0
        else
            echo "❌ Error: $error"
            return 1
        fi
    elif echo "$response" | grep -q '"choices"'; then
        echo "✅ Model already available/downloaded"
        return 0
    else
        echo "⚠️  Unexpected response"
        return 1
    fi
}

# Test cases
echo "Test Case 1: Specific file (should download only that file)"
echo "------------------------------------------------------------"
test_model_download \
    "Qwen/Qwen2.5-0.5B-Instruct-GGUF:qwen2.5-0.5b-instruct-q4_k_m.gguf" \
    "Single specific file"

echo ""
echo "Test Case 2: Quantization format (should download one file or all split parts)"
echo "-------------------------------------------------------------------------------"
test_model_download \
    "TheBloke/Llama-2-7B-Chat-GGUF:q4_k_m" \
    "Quantization-based selection"

echo ""
echo "3. Checking downloaded files..."
echo "========================================"

if [ -d models ]; then
    for dir in models/*/; do
        if [ -d "$dir" ]; then
            model_name=$(basename "$dir")
            file_count=$(find "$dir" -name "*.gguf" -type f 2>/dev/null | wc -l)

            if [ $file_count -gt 0 ]; then
                echo ""
                echo "$model_name:"
                echo "  Total files: $file_count"

                # Check if there are split files
                split_count=$(find "$dir" -name "*-[0-9][0-9][0-9][0-9][0-9]-of-[0-9][0-9][0-9][0-9][0-9]*" -type f 2>/dev/null | wc -l)
                if [ $split_count -gt 0 ]; then
                    echo "  Split model: YES ($split_count parts)"
                fi

                # List files with sizes
                find "$dir" -name "*.gguf" -type f 2>/dev/null | head -5 | while read -r file; do
                    size=$(ls -lh "$file" | awk '{print $5}')
                    echo "    - $(basename "$file") ($size)"
                done

                if [ $file_count -gt 5 ]; then
                    echo "    ... and $((file_count - 5)) more files"
                fi
            fi
        fi
    done
else
    echo "No models directory found"
fi

echo ""
echo "========================================"
echo "Test Summary"
echo "========================================"
echo ""
echo "Expected behavior:"
echo "1. Search returns individual IDs for each file (repo:filename)"
echo "2. For split models: ALL parts are downloaded"
echo "3. For regular models: Only ONE file is downloaded"
echo "4. Specific file requests: Exact file is downloaded"
echo "5. Quantization requests: First matching file (or all parts if split)"
echo ""
echo "You can use specific IDs from search results to download exact files:"
echo "  Example: model_id=\"repo/name:specific-file.gguf\""
echo ""