#!/usr/bin/env python3

"""
Test script to verify search API and suggestedModelID work for downloading
"""

import requests
import json
import time
import sys
import os

# Get API base from environment or use default
API_BASE = os.getenv('FROG_URL', 'http://localhost:5800')

def test_search_and_download(search_query):
    """Test complete flow: search → get model ID → use for chat"""

    print(f"\n{'='*60}")
    print(f"Testing: {search_query}")
    print(f"{'='*60}")

    # Step 1: Search for models
    print(f"\n1. Searching for: {search_query}")
    search_url = f"{API_BASE}/api/v1/models/search"
    params = {
        'q': search_query,
        'limit': 5
    }

    try:
        response = requests.get(search_url, params=params, timeout=10)
        response.raise_for_status()
        data = response.json()
    except requests.RequestException as e:
        print(f"❌ Search failed: {e}")
        return False

    models = data.get('models', [])
    if not models:
        print(f"❌ No models found for query: {search_query}")
        return False

    print(f"✅ Found {len(models)} models")

    # Display found models
    for i, model in enumerate(models[:3], 1):
        print(f"\n  Model {i}:")
        print(f"    ID: {model.get('id', 'N/A')}")
        print(f"    Name: {model.get('name', 'N/A')}")
        print(f"    Quantization: {model.get('quantization', 'N/A')}")
        print(f"    Size: {model.get('size_gb', 0):.2f} GB")
        print(f"    Repo: {model.get('repo', 'N/A')}")
        print(f"    File: {model.get('file', 'N/A')}")

    # Step 2: Use the first model ID
    model_id = models[0].get('id')
    if not model_id:
        print("❌ No valid model ID in search result")
        return False

    print(f"\n2. Testing model ID from search: {model_id}")

    chat_url = f"{API_BASE}/v1/chat/completions"
    payload = {
        'model': model_id,
        'messages': [
            {'role': 'user', 'content': 'Say "Hello from FrogLLM" and nothing else.'}
        ],
        'temperature': 0.1,
        'max_tokens': 20
    }

    try:
        response = requests.post(chat_url, json=payload, timeout=30)
        data = response.json()
    except requests.RequestException as e:
        print(f"❌ Chat request failed: {e}")
        return False

    if 'error' in data:
        print(f"❌ Model failed: {data['error']}")
        return False

    if 'choices' in data:
        content = data['choices'][0]['message'].get('content', '')
        print(f"✅ Success! Model responded: {content[:100]}")
        return True

    print("⚠️  Unexpected response (model might be downloading)")
    print(f"Response: {json.dumps(data, indent=2)[:200]}")
    return None

def test_huggingface_search(model_id):
    """Test the HuggingFace search endpoint and suggestedModelID"""

    print(f"\n{'='*60}")
    print(f"Testing HuggingFace search for: {model_id}")
    print(f"{'='*60}")

    # Search for specific model
    search_url = f"{API_BASE}/api/models/search/huggingface"
    params = {'modelId': model_id}

    try:
        response = requests.get(search_url, params=params, timeout=10)
        response.raise_for_status()
        data = response.json()
    except requests.RequestException as e:
        print(f"❌ HF search failed: {e}")
        return False

    # Check for GGUF files
    gguf_files = data.get('ggufFiles', [])
    split_models = data.get('splitModels', [])

    print(f"Found: {len(gguf_files)} GGUF files, {len(split_models)} split models")

    # Test first suggestedModelID if available
    test_id = None

    if gguf_files:
        for i, file in enumerate(gguf_files[:3], 1):
            print(f"\n  File {i}:")
            print(f"    Filename: {file.get('filename', 'N/A')}")
            print(f"    Quantization: {file.get('quantization', 'N/A')}")
            print(f"    Size: {file.get('size', 0) / (1024**3):.2f} GB")
            if 'suggestedModelID' in file:
                print(f"    SuggestedID: {file['suggestedModelID']}")
                if not test_id:
                    test_id = file['suggestedModelID']

    if split_models:
        for i, split in enumerate(split_models[:2], 1):
            print(f"\n  Split Model {i}:")
            print(f"    Base: {split.get('baseName', 'N/A')}")
            print(f"    Parts: {len(split.get('parts', []))}")
            if 'suggestedModelID' in split:
                print(f"    SuggestedID: {split['suggestedModelID']}")
                if not test_id:
                    test_id = split['suggestedModelID']

    # Test the suggestedModelID
    if test_id:
        print(f"\n3. Testing suggestedModelID: {test_id}")

        chat_url = f"{API_BASE}/v1/chat/completions"
        payload = {
            'model': test_id,
            'messages': [
                {'role': 'user', 'content': 'Count to 3'}
            ],
            'temperature': 0.1,
            'max_tokens': 20
        }

        try:
            response = requests.post(chat_url, json=payload, timeout=30)
            data = response.json()

            if 'error' in data:
                print(f"❌ SuggestedModelID failed: {data['error'][:100]}")
                return False

            if 'choices' in data:
                print(f"✅ SuggestedModelID works!")
                return True

        except requests.RequestException as e:
            print(f"❌ Request failed: {e}")
            return False

    print("⚠️  No suggestedModelID found to test")
    return None

def main():
    """Main test function"""

    print("FrogLLM Search → Download Integration Test")
    print("=" * 60)
    print(f"API Base: {API_BASE}")
    print()

    # Check if server is running
    try:
        response = requests.get(f"{API_BASE}/api/system/specs", timeout=2)
        if response.status_code != 200:
            print("❌ FrogLLM server not responding properly")
            sys.exit(1)
    except requests.RequestException:
        print("❌ Cannot connect to FrogLLM")
        print(f"Make sure FrogLLM is running at {API_BASE}")
        sys.exit(1)

    print("✅ Server is running\n")

    # Test various search queries
    test_queries = [
        "qwen 0.5b instruct",
        "tinyllama 1.1b",
        "mistral 7b",
        "bartowski mistral",
        "llama 2 7b chat"
    ]

    results = []
    for query in test_queries:
        result = test_search_and_download(query)
        results.append((query, result))
        time.sleep(1)  # Be nice to the server

    # Test specific HuggingFace models
    hf_models = [
        "Qwen/Qwen2.5-0.5B-Instruct-GGUF",
        "bartowski/Mistral-22B-v0.1-GGUF",
        "TheBloke/Llama-2-7B-Chat-GGUF"
    ]

    for model in hf_models:
        result = test_huggingface_search(model)
        results.append((f"HF: {model}", result))
        time.sleep(1)

    # Print summary
    print("\n" + "=" * 60)
    print("Test Summary")
    print("=" * 60)

    for query, result in results:
        if result is True:
            status = "✅ PASSED"
        elif result is False:
            status = "❌ FAILED"
        else:
            status = "⚠️  UNKNOWN"
        print(f"{status}: {query}")

    # Count results
    passed = sum(1 for _, r in results if r is True)
    failed = sum(1 for _, r in results if r is False)
    unknown = sum(1 for _, r in results if r is None)

    print(f"\nTotal: {passed} passed, {failed} failed, {unknown} unknown")

    if failed > 0:
        print("\n⚠️  Some tests failed. Check:")
        print("1. Model repositories exist and have GGUF files")
        print("2. Network connectivity to HuggingFace")
        print("3. Server logs for detailed error messages")

    return 0 if failed == 0 else 1

if __name__ == "__main__":
    sys.exit(main())