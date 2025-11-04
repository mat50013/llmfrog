#!/usr/bin/env python3

"""
FrogLLM Comprehensive API Test Suite
Tests all new functionality: search, download, load/unload, and auto-download
"""

import requests
import json
import time
import sys
from typing import Dict, Any, List
from datetime import datetime

# Configuration
API_BASE = "http://localhost:5800/api"
V1_BASE = "http://localhost:5800/v1"
HF_TOKEN = "YOUR_HF_TOKEN_HERE"  # Replace with actual token
API_KEY = "YOUR_API_KEY_HERE"     # Replace with actual API key

# Color codes for terminal output
class Colors:
    HEADER = '\033[95m'
    BLUE = '\033[94m'
    CYAN = '\033[96m'
    GREEN = '\033[92m'
    YELLOW = '\033[93m'
    RED = '\033[91m'
    END = '\033[0m'
    BOLD = '\033[1m'

def print_test_header(test_name: str):
    """Print a formatted test header"""
    print(f"\n{Colors.YELLOW}{'='*60}{Colors.END}")
    print(f"{Colors.BOLD}{Colors.BLUE}TEST: {test_name}{Colors.END}")
    print(f"{Colors.YELLOW}{'='*60}{Colors.END}")

def print_result(success: bool, message: str):
    """Print test result with color coding"""
    if success:
        print(f"{Colors.GREEN}✓ {message}{Colors.END}")
    else:
        print(f"{Colors.RED}✗ {message}{Colors.END}")

def pretty_print_json(data: Any, max_length: int = None):
    """Pretty print JSON data"""
    json_str = json.dumps(data, indent=2)
    if max_length and len(json_str) > max_length:
        json_str = json_str[:max_length] + "..."
    print(json_str)

class FrogLLMTestSuite:
    def __init__(self):
        self.session = requests.Session()
        self.test_results = []

    def run_test(self, test_func):
        """Run a test function and track results"""
        try:
            test_func()
            self.test_results.append((test_func.__name__, True, None))
        except Exception as e:
            self.test_results.append((test_func.__name__, False, str(e)))
            print(f"{Colors.RED}Error: {e}{Colors.END}")

    # Test 1: Model Search
    def test_model_search(self):
        print_test_header("Model Search API")

        # Search for Qwen models
        print("\n1. Searching for Qwen 7B models...")
        response = self.session.get(
            f"{API_BASE}/v1/models/search",
            params={"q": "qwen 7b", "limit": 3}
        )
        if response.status_code == 200:
            data = response.json()
            print_result(True, f"Found {len(data.get('models', []))} models")
            pretty_print_json(data, 500)
        else:
            print_result(False, f"Search failed: {response.status_code}")

        # Search with HF token for gated models
        print("\n2. Searching for Llama models (including gated)...")
        headers = {"HF-Token": HF_TOKEN} if HF_TOKEN != "YOUR_HF_TOKEN_HERE" else {}
        response = self.session.get(
            f"{API_BASE}/v1/models/search",
            params={"q": "llama 3", "include_restricted": "true", "limit": 3},
            headers=headers
        )
        if response.status_code == 200:
            data = response.json()
            print_result(True, f"Found {len(data.get('models', []))} models")
            pretty_print_json(data, 500)
        else:
            print_result(False, f"Search failed: {response.status_code}")

    # Test 2: Model Download
    def test_model_download(self):
        print_test_header("Model Download API")

        print("Downloading small test model: Qwen2.5-0.5B-Instruct...")
        response = self.session.post(
            f"{API_BASE}/models/download",
            json={
                "repo": "Qwen/Qwen2.5-0.5B-Instruct-GGUF",
                "filename": "qwen2.5-0.5b-instruct-q4_k_m.gguf",
                "destination": "/downloads"
            }
        )

        if response.status_code == 200:
            data = response.json()
            download_id = data.get("downloadId")
            print_result(True, f"Download initiated: {download_id}")

            # Check download status
            time.sleep(2)
            status_response = self.session.get(f"{API_BASE}/models/downloads/{download_id}")
            if status_response.status_code == 200:
                status_data = status_response.json()
                print(f"Download status: {status_data.get('status')}")
                print(f"Progress: {status_data.get('progress', 0):.1f}%")
        else:
            print_result(False, f"Download failed: {response.status_code}")

    # Test 3: Load/Unload Models
    def test_model_management(self):
        print_test_header("Model Load/Unload with LRU")

        # Get currently loaded models
        print("\n1. Getting currently loaded models...")
        response = self.session.get(f"{API_BASE}/v1/models/loaded")
        if response.status_code == 200:
            data = response.json()
            print_result(True, f"Loaded models: {data.get('total', 0)}")
            pretty_print_json(data.get('models', []), 500)

        # Try to load a model
        print("\n2. Loading a model with auto-unload...")
        response = self.session.post(
            f"{API_BASE}/v1/models/load",
            json={
                "model_id": "test-model",
                "auto_unload": True
            }
        )
        print(f"Load response: {response.status_code}")
        if response.status_code in [200, 404]:
            pretty_print_json(response.json(), 300)

    # Test 4: Chat Completion with Non-Existent Model
    def test_chat_with_auto_download(self):
        print_test_header("Chat Completion with Auto-Download")

        print("Testing complex reasoning with non-existent model...")
        print("Model: Qwen/Qwen2.5-1.5B-Instruct-GGUF:qwen2.5-1.5b-instruct-q4_k_m.gguf")

        headers = {"Authorization": f"Bearer {API_KEY}"} if API_KEY != "YOUR_API_KEY_HERE" else {}

        # Complex reasoning problem
        response = self.session.post(
            f"{V1_BASE}/chat/completions",
            headers=headers,
            json={
                "model": "Qwen/Qwen2.5-1.5B-Instruct-GGUF:qwen2.5-1.5b-instruct-q4_k_m.gguf",
                "messages": [
                    {
                        "role": "system",
                        "content": "You are a mathematics tutor. Explain your reasoning step by step."
                    },
                    {
                        "role": "user",
                        "content": (
                            "A train leaves Station A at 9:00 AM traveling at 60 mph. "
                            "Another train leaves Station B at 10:00 AM traveling at 80 mph "
                            "toward Station A. If the stations are 280 miles apart, "
                            "at what time do they meet? Show your work."
                        )
                    }
                ],
                "temperature": 0.3,
                "max_tokens": 500,
                "stream": False
            },
            timeout=60  # Longer timeout for download
        )

        if response.status_code == 200:
            data = response.json()
            print_result(True, "Chat completion successful")
            if 'choices' in data:
                print("\nModel response:")
                print(data['choices'][0]['message']['content'][:1000])
        else:
            print_result(False, f"Chat failed: {response.status_code}")
            print(response.text[:500])

    # Test 5: Stress Test with Multiple Models
    def test_lru_eviction(self):
        print_test_header("LRU Eviction Stress Test")

        models_to_test = [
            "Qwen/Qwen2.5-0.5B-Instruct-GGUF:qwen2.5-0.5b-instruct-q4_k_m.gguf",
            "TinyLlama/TinyLlama-1.1B-Chat-v1.0-GGUF:tinyllama-1.1b-chat-v1.0.Q4_K_M.gguf",
            "mistralai/Mistral-7B-Instruct-v0.2-GGUF:mistral-7b-instruct-v0.2.Q4_K_M.gguf"
        ]

        print("Sending requests to multiple models to test LRU...")
        headers = {"Authorization": f"Bearer {API_KEY}"} if API_KEY != "YOUR_API_KEY_HERE" else {}

        for i, model in enumerate(models_to_test):
            print(f"\n{i+1}. Testing model: {model.split(':')[0]}...")
            response = self.session.post(
                f"{V1_BASE}/chat/completions",
                headers=headers,
                json={
                    "model": model,
                    "messages": [
                        {"role": "user", "content": f"Say 'Model {i+1} is working!' and nothing else."}
                    ],
                    "temperature": 0.1,
                    "max_tokens": 20,
                    "stream": False
                },
                timeout=30
            )

            if response.status_code == 200:
                print_result(True, f"Model {i+1} responded")
            else:
                print_result(False, f"Model {i+1} failed: {response.status_code}")

            time.sleep(1)  # Give time between requests

    # Test 6: Activity Statistics
    def test_activity_stats(self):
        print_test_header("Activity Statistics")

        response = self.session.get(f"{API_BASE}/activity/stats")
        if response.status_code == 200:
            data = response.json()
            print_result(True, "Retrieved activity statistics")
            pretty_print_json(data, 1000)
        else:
            print_result(False, f"Failed to get stats: {response.status_code}")

    # Test 7: Multi-GPU Display
    def test_multi_gpu(self):
        print_test_header("Multi-GPU Detection and Display")

        # System specs
        print("\n1. System Specs (should show all GPUs)...")
        response = self.session.get(f"{API_BASE}/system/specs")
        if response.status_code == 200:
            data = response.json()
            gpu_count = len(data.get('gpus', []))
            total_vram = data.get('totalVRAM', 0)
            print_result(True, f"Detected {gpu_count} GPUs with {total_vram}GB total VRAM")
            for gpu in data.get('gpus', []):
                print(f"  - GPU {gpu.get('index', 'N/A')}: {gpu.get('name', 'Unknown')} - {gpu.get('memory', 0)}GB")

        # GPU stats
        print("\n2. GPU Statistics...")
        response = self.session.get(f"{API_BASE}/gpu/stats")
        if response.status_code == 200:
            data = response.json()
            for gpu in data:
                usage = (gpu.get('memoryUsed', 0) / gpu.get('memoryTotal', 1)) * 100
                print(f"  - GPU {gpu.get('index', 'N/A')}: {usage:.1f}% memory used")

    # Test 8: Edge Cases
    def test_edge_cases(self):
        print_test_header("Edge Cases and Error Handling")

        # Invalid model ID
        print("\n1. Testing invalid model load...")
        response = self.session.post(
            f"{API_BASE}/v1/models/load",
            json={
                "model_id": "!!!invalid-model-id-with-special-chars!!!",
                "auto_unload": False
            }
        )
        if response.status_code >= 400:
            print_result(True, f"Properly rejected invalid model: {response.status_code}")
        else:
            print_result(False, "Should have rejected invalid model")

        # Empty search query
        print("\n2. Testing empty search query...")
        response = self.session.get(f"{API_BASE}/v1/models/search", params={"q": ""})
        if response.status_code >= 400:
            print_result(True, "Properly rejected empty search")
        else:
            print_result(False, "Should have rejected empty search")

        # Very long model name
        print("\n3. Testing extremely long model name...")
        long_model = "a" * 1000 + ":model.gguf"
        response = self.session.post(
            f"{V1_BASE}/chat/completions",
            json={
                "model": long_model,
                "messages": [{"role": "user", "content": "test"}],
                "max_tokens": 1
            },
            timeout=5
        )
        if response.status_code >= 400:
            print_result(True, "Properly handled long model name")
        else:
            print_result(False, "Should have rejected extremely long model name")

    def run_all_tests(self):
        """Run all tests in sequence"""
        print(f"{Colors.BOLD}{Colors.HEADER}FrogLLM Comprehensive Test Suite{Colors.END}")
        print(f"API Base: {API_BASE}")
        print(f"Started at: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}")

        tests = [
            self.test_model_search,
            self.test_model_download,
            self.test_model_management,
            self.test_chat_with_auto_download,
            self.test_lru_eviction,
            self.test_activity_stats,
            self.test_multi_gpu,
            self.test_edge_cases
        ]

        for test in tests:
            self.run_test(test)
            time.sleep(1)  # Brief pause between tests

        # Print summary
        print(f"\n{Colors.BOLD}{Colors.CYAN}{'='*60}{Colors.END}")
        print(f"{Colors.BOLD}Test Summary{Colors.END}")
        print(f"{Colors.CYAN}{'='*60}{Colors.END}")

        passed = sum(1 for _, success, _ in self.test_results if success)
        failed = len(self.test_results) - passed

        for test_name, success, error in self.test_results:
            if success:
                print(f"{Colors.GREEN}✓ {test_name}{Colors.END}")
            else:
                print(f"{Colors.RED}✗ {test_name}: {error}{Colors.END}")

        print(f"\n{Colors.BOLD}Results: {Colors.GREEN}{passed} passed{Colors.END}, {Colors.RED}{failed} failed{Colors.END}")
        print(f"Completed at: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}")

        return failed == 0

if __name__ == "__main__":
    # Check if FrogLLM is running
    try:
        response = requests.get(f"{API_BASE}/system/specs", timeout=2)
        if response.status_code != 200:
            print(f"{Colors.RED}Error: FrogLLM API is not responding properly{Colors.END}")
            sys.exit(1)
    except requests.exceptions.RequestException:
        print(f"{Colors.RED}Error: Cannot connect to FrogLLM at localhost:5800{Colors.END}")
        print(f"{Colors.YELLOW}Please ensure FrogLLM is running: ./frogllm{Colors.END}")
        sys.exit(1)

    # Run tests
    suite = FrogLLMTestSuite()
    success = suite.run_all_tests()

    sys.exit(0 if success else 1)