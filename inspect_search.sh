#!/bin/bash

# Script to inspect exactly what the search API returns

echo "========================================"
echo "Search API Response Inspector"
echo "========================================"
echo ""

API_BASE="${FROG_URL:-http://localhost:5800}"

inspect_search() {
    local query="$1"

    echo "Query: $query"
    echo "----------------------------------------"

    # Make the search request and pretty print
    curl -s "${API_BASE}/api/v1/models/search?q=${query}&limit=3" | python3 -c "
import json, sys

try:
    data = json.load(sys.stdin)

    print(f\"Total models found: {data.get('total', 0)}\")
    print()

    models = data.get('models', [])
    for i, model in enumerate(models, 1):
        print(f'Model {i}:')
        print(f'  id: {model.get(\"id\")}')
        print(f'  name: {model.get(\"name\")}')
        print(f'  quantization: {model.get(\"quantization\")}')
        print(f'  size_gb: {model.get(\"size_gb\", 0):.2f}')
        print(f'  repo: {model.get(\"repo\")}')
        print(f'  file: {model.get(\"file\")}')
        print(f'  requires_auth: {model.get(\"requires_auth\")}')
        print()

        # Show how to use this model
        model_id = model.get('id')
        if model_id:
            print(f'  To use this model:')
            print(f'  curl -X POST {API_BASE}/v1/chat/completions \\\\')
            print(f'    -H \"Content-Type: application/json\" \\\\')
            print(f'    -d \\'{{')
            print(f'      \"model\": \"{model_id}\",')
            print(f'      \"messages\": [{{\"role\": \"user\", \"content\": \"Hello\"}}],')
            print(f'      \"max_tokens\": 50')
            print(f'    }}\\''')
            print()

except Exception as e:
    print(f'Error parsing response: {e}')
    print('Raw response:')
    sys.stdin.seek(0)
    print(sys.stdin.read())
"

    echo ""
}

# Test different searches
echo "1. Searching for Qwen models:"
echo "========================================"
inspect_search "qwen"

echo -e "\n2. Searching for Bartowski models:"
echo "========================================"
inspect_search "bartowski+mistral"

echo -e "\n3. Searching for TinyLlama:"
echo "========================================"
inspect_search "tinyllama"

echo -e "\n4. Testing HuggingFace endpoint for bartowski/Mistral-22B-v0.1-GGUF:"
echo "========================================"
curl -s "${API_BASE}/api/models/search/huggingface?modelId=bartowski/Mistral-22B-v0.1-GGUF" | python3 -c "
import json, sys

try:
    data = json.load(sys.stdin)

    if 'ggufFiles' in data:
        files = data['ggufFiles']
        print(f'GGUF Files: {len(files)}')
        for f in files[:5]:
            print(f'  - {f.get(\"filename\")}')
            print(f'    Quantization: {f.get(\"quantization\")}')
            if 'suggestedModelID' in f:
                print(f'    SuggestedModelID: {f[\"suggestedModelID\"]}')
            print()

    if 'splitModels' in data:
        splits = data['splitModels']
        print(f'Split Models: {len(splits)}')
        for s in splits[:3]:
            print(f'  - {s.get(\"baseName\")}')
            if 'suggestedModelID' in s:
                print(f'    SuggestedModelID: {s[\"suggestedModelID\"]}')

except Exception as e:
    print(f'Error: {e}')
"

echo -e "\n========================================"
echo "Key Points:"
echo "========================================"
echo "1. The 'id' field from search results should work directly in chat/completions"
echo "2. Format is usually 'repo:filename' or 'repo:quantization'"
echo "3. These IDs trigger auto-download if the model isn't local"
echo "4. Check server logs if downloads fail"
echo ""