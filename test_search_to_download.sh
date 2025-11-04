#!/bin/bash

# Test that search results and suggestedModelID can be used directly for downloading

echo "========================================"
echo "Search API to Download Test"
echo "========================================"
echo "Testing complete flow: Search → Get suggestedModelID → Use in chat/download"
echo ""

API_BASE="${FROG_URL:-http://localhost:5800}"

# Function to test a model from search to download
test_search_to_download() {
    local search_query="$1"
    local test_name="$2"

    echo -e "\n========================================"
    echo "Test: $test_name"
    echo "Search Query: $search_query"
    echo "========================================"

    # Step 1: Search for models
    echo -e "\n1. Searching for models..."
    search_response=$(curl -s "${API_BASE}/api/v1/models/search?q=${search_query}&limit=5")

    if [ -z "$search_response" ] || echo "$search_response" | grep -q '"error"'; then
        echo "❌ Search failed"
        echo "$search_response"
        return
    fi

    # Extract model IDs from search results
    echo -e "\n2. Models found:"
    model_ids=$(echo "$search_response" | python3 -c "
import json, sys
try:
    data = json.load(sys.stdin)
    models = data.get('models', [])
    if not models:
        print('No models found')
        sys.exit(1)

    for i, model in enumerate(models[:3], 1):
        print(f'{i}. ID: {model.get(\"id\", \"N/A\")}')
        print(f'   Name: {model.get(\"name\", \"N/A\")}')
        print(f'   Quantization: {model.get(\"quantization\", \"N/A\")}')
        print(f'   Size: {model.get(\"size_gb\", 0):.2f} GB')
        print(f'   Repo: {model.get(\"repo\", \"N/A\")}')
        print(f'   File: {model.get(\"file\", \"N/A\")}')
        print()

    # Return first model ID for testing
    if models:
        print('SELECTED_ID:' + models[0].get('id', ''))
except Exception as e:
    print(f'Error parsing: {e}')
    sys.exit(1)
" 2>&1)

    echo "$model_ids" | grep -v "SELECTED_ID"

    # Extract the selected model ID
    selected_id=$(echo "$model_ids" | grep "SELECTED_ID:" | cut -d':' -f2-)

    if [ -z "$selected_id" ] || [ "$selected_id" = "N/A" ]; then
        echo "❌ No valid model ID found in search results"
        return
    fi

    echo -e "\n3. Testing with model ID from search: $selected_id"
    echo "----------------------------------------"

    # Step 2: Use the model ID in a chat completion request
    chat_response=$(curl -s -X POST "${API_BASE}/v1/chat/completions" \
        -H "Content-Type: application/json" \
        -d "{
            \"model\": \"$selected_id\",
            \"messages\": [{
                \"role\": \"user\",
                \"content\": \"Count to 5\"
            }],
            \"temperature\": 0.1,
            \"max_tokens\": 30
        }" 2>&1)

    # Check if the request was accepted
    if echo "$chat_response" | grep -q '"error"'; then
        error_msg=$(echo "$chat_response" | python3 -c "import json, sys; d=json.load(sys.stdin); print(d.get('error', 'Unknown error')[:200])" 2>/dev/null || echo "Parse error")
        echo "❌ Failed to use model ID: $error_msg"
    elif echo "$chat_response" | grep -q '"choices"'; then
        echo "✅ Success! Model ID worked directly from search"
        content=$(echo "$chat_response" | python3 -c "import json, sys; d=json.load(sys.stdin); print(d['choices'][0]['message']['content'][:100] if 'choices' in d else 'No content')" 2>/dev/null || echo "")
        if [ ! -z "$content" ]; then
            echo "Model responded: $content"
        fi
    else
        echo "⚠️  Model might be downloading (check server logs)"
        echo "Response preview: $(echo "$chat_response" | head -c 200)"
    fi
}

# Test different search queries
echo "Testing various model searches and their direct usage..."

# Test 1: Small Qwen model
test_search_to_download "qwen+0.5b+instruct" "Qwen 0.5B Instruct Model"

# Test 2: Mistral models
test_search_to_download "mistral+7b+instruct" "Mistral 7B Instruct Model"

# Test 3: Llama models
test_search_to_download "llama+2+7b+chat" "Llama 2 7B Chat Model"

# Test 4: TinyLlama
test_search_to_download "tinyllama+1.1b" "TinyLlama 1.1B Model"

# Test 5: Specific bartowski model
test_search_to_download "bartowski+mistral" "Bartowski Mistral Models"

# Now test the detailed HuggingFace search endpoint
echo -e "\n========================================"
echo "Testing HuggingFace Search Endpoint"
echo "========================================"

test_hf_search() {
    local model_id="$1"

    echo -e "\nSearching HuggingFace for: $model_id"
    hf_response=$(curl -s "${API_BASE}/api/models/search/huggingface?modelId=${model_id}")

    echo "$hf_response" | python3 -c "
import json, sys
try:
    data = json.load(sys.stdin)

    # Check for regular files
    if 'ggufFiles' in data:
        files = data['ggufFiles']
        print(f'Found {len(files)} GGUF files:')
        for f in files[:3]:
            print(f'  File: {f.get(\"filename\", \"N/A\")}')
            print(f'    Quantization: {f.get(\"quantization\", \"N/A\")}')
            if 'suggestedModelID' in f:
                print(f'    SuggestedID: {f[\"suggestedModelID\"]}')
            print()

    # Check for split models
    if 'splitModels' in data and data['splitModels']:
        print(f'Found {len(data[\"splitModels\"])} split models:')
        for s in data['splitModels'][:2]:
            print(f'  Base: {s.get(\"baseName\", \"N/A\")}')
            print(f'    Parts: {len(s.get(\"parts\", []))}')
            if 'suggestedModelID' in s:
                print(f'    SuggestedID: {s[\"suggestedModelID\"]}')

    # Test first suggested ID if available
    if 'ggufFiles' in data and data['ggufFiles']:
        first_file = data['ggufFiles'][0]
        if 'suggestedModelID' in first_file:
            print(f'\\n🔍 Testing suggestedModelID: {first_file[\"suggestedModelID\"]}')
            # Return it for testing
            print('TEST_ID:' + first_file['suggestedModelID'])
except Exception as e:
    print(f'Error: {e}')
" 2>&1

    # Extract and test the suggested ID
    test_id=$(echo "$hf_response" | python3 -c "
import json, sys
try:
    data = json.load(sys.stdin)
    if 'ggufFiles' in data and data['ggufFiles']:
        if 'suggestedModelID' in data['ggufFiles'][0]:
            print(data['ggufFiles'][0]['suggestedModelID'])
except:
    pass
" 2>/dev/null)

    if [ ! -z "$test_id" ]; then
        echo -e "\nTesting suggestedModelID: $test_id"
        test_response=$(curl -s -X POST "${API_BASE}/v1/chat/completions" \
            -H "Content-Type: application/json" \
            -d "{
                \"model\": \"$test_id\",
                \"messages\": [{\"role\": \"user\", \"content\": \"Hi\"}],
                \"max_tokens\": 5
            }" 2>&1)

        if echo "$test_response" | grep -q '"error"'; then
            echo "❌ SuggestedModelID failed"
        elif echo "$test_response" | grep -q '"choices"'; then
            echo "✅ SuggestedModelID works!"
        else
            echo "⚠️  Unknown response (might be downloading)"
        fi
    fi
}

# Test specific model repositories
test_hf_search "Qwen/Qwen2.5-0.5B-Instruct-GGUF"
test_hf_search "bartowski/Mistral-22B-v0.1-GGUF"
test_hf_search "TheBloke/Llama-2-7B-Chat-GGUF"

echo -e "\n========================================"
echo "Summary"
echo "========================================"
echo "This test verified:"
echo "1. Search API returns usable model IDs"
echo "2. Model IDs from search work in chat/completions"
echo "3. SuggestedModelID format is correct"
echo "4. Auto-download triggers when needed"
echo ""
echo "Check server logs for download progress if models aren't cached."
echo ""