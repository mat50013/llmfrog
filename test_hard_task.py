#!/usr/bin/env python3

"""
FrogLLM Hard Task Test
Tests auto-download and execution of complex tasks with non-existent models
"""

import requests
import json
import time
import sys

API_BASE = "http://localhost:5800"

# Hard tasks for testing
HARD_TASKS = [
    {
        "name": "Complex Code Generation",
        "model": "Qwen/Qwen2.5-Coder-7B-Instruct-GGUF:qwen2.5-coder-7b-instruct-q4_k_m.gguf",
        "messages": [
            {
                "role": "system",
                "content": "You are an expert programmer. Write complete, production-ready code."
            },
            {
                "role": "user",
                "content": """Write a complete implementation of a distributed rate limiter in Go that:
1. Uses Redis for distributed state
2. Implements token bucket algorithm
3. Supports multiple rate limit tiers (free: 10/min, premium: 100/min, enterprise: unlimited)
4. Has middleware for Gin framework
5. Includes graceful degradation if Redis is unavailable
6. Has comprehensive error handling and logging
7. Includes unit tests

Make it production-ready with proper error handling, comments, and best practices."""
            }
        ],
        "temperature": 0.2,
        "max_tokens": 2000
    },
    {
        "name": "Advanced Mathematical Reasoning",
        "model": "TheBloke/Llama-2-13B-chat-GGUF:llama-2-13b-chat.Q4_K_M.gguf",
        "messages": [
            {
                "role": "system",
                "content": "You are a mathematics professor. Show all work and explain each step."
            },
            {
                "role": "user",
                "content": """Solve this optimization problem:

A company manufactures two products, A and B. The profit per unit is $40 for A and $30 for B.
The production constraints are:
- Product A requires 2 hours of labor and 3 units of raw material
- Product B requires 1 hour of labor and 4 units of raw material
- Available: 100 hours of labor and 180 units of raw material per week
- Market demand limits: maximum 40 units of A and 60 units of B per week

Using linear programming:
1. Formulate the complete LP problem
2. Find the optimal production quantities
3. Calculate the maximum profit
4. Perform sensitivity analysis on the labor constraint
5. Determine the shadow prices for each constraint

Show all calculations, draw the feasible region, and explain the economic interpretation."""
            }
        ],
        "temperature": 0.3,
        "max_tokens": 1500
    },
    {
        "name": "Complex Data Analysis",
        "model": "mistralai/Mixtral-8x7B-Instruct-v0.1-GGUF:mixtral-8x7b-instruct-v0.1.Q4_K_M.gguf",
        "messages": [
            {
                "role": "system",
                "content": "You are a data scientist. Provide detailed analysis with code."
            },
            {
                "role": "user",
                "content": """Design a complete machine learning pipeline for predicting customer churn:

Dataset characteristics:
- 1M customers, 50 features (demographic, behavioral, transactional)
- 15% class imbalance (15% churned)
- Missing values in 30% of features
- High multicollinearity between transaction features

Requirements:
1. Data preprocessing strategy (handle missing values, scaling, encoding)
2. Feature engineering (create 10 meaningful features)
3. Feature selection approach (reduce to top 20 features)
4. Model selection (compare 5 algorithms with pros/cons)
5. Handle class imbalance (3 techniques)
6. Hyperparameter tuning strategy
7. Model interpretability (SHAP/LIME)
8. Deployment architecture (real-time scoring API)
9. Monitoring strategy (data drift, model decay)
10. A/B testing framework for model updates

Provide Python code snippets for each step."""
            }
        ],
        "temperature": 0.4,
        "max_tokens": 2000
    },
    {
        "name": "Philosophical Reasoning",
        "model": "TheBloke/Nous-Hermes-2-Mixtral-8x7B-DPO-GGUF:nous-hermes-2-mixtral-8x7b-dpo.Q4_K_M.gguf",
        "messages": [
            {
                "role": "system",
                "content": "You are a philosophy professor. Provide deep, nuanced analysis."
            },
            {
                "role": "user",
                "content": """Analyze the following thought experiment:

'The Experience Machine': Imagine a machine that could give you any experiences you desire.
While plugged in, you would think and feel you were writing a great novel, making friends,
or reading interesting books. All the time you would be floating in a tank with electrodes
attached to your brain. Would you plug in?

In your analysis:
1. Examine this from utilitarian, deontological, and virtue ethics perspectives
2. Discuss the nature of reality, experience, and authenticity
3. Compare to modern issues: social media, VR, AI relationships
4. Address counterarguments to each position
5. Consider implications for the meaning of life and human flourishing
6. Relate to the philosophy of mind and consciousness
7. Discuss free will and determinism in this context

Provide a sophisticated philosophical analysis with references to relevant philosophers."""
            }
        ],
        "temperature": 0.7,
        "max_tokens": 1500
    },
    {
        "name": "Scientific Research Synthesis",
        "model": "NousResearch/Nous-Hermes-2-SOLAR-10.7B-GGUF:nous-hermes-2-solar-10.7b.Q4_K_M.gguf",
        "messages": [
            {
                "role": "system",
                "content": "You are a research scientist. Provide comprehensive scientific analysis."
            },
            {
                "role": "user",
                "content": """Synthesize current research on CRISPR-Cas9 gene editing for treating genetic diseases:

Cover these aspects:
1. Mechanism of action (detailed molecular biology)
2. Recent breakthroughs (2020-2024) in clinical trials
3. Comparison of delivery methods (AAV, LNP, ex vivo)
4. Off-target effects and mitigation strategies
5. Ethical considerations and regulatory landscape
6. Cost-benefit analysis for different genetic diseases
7. Technical challenges and current limitations
8. Future directions and emerging alternatives (prime editing, base editing)
9. Specific case studies: sickle cell, DMD, Leber's congenital amaurosis
10. Recommendations for research priorities

Include hypothetical experimental design for improving specificity."""
            }
        ],
        "temperature": 0.4,
        "max_tokens": 2000
    }
]

def test_hard_task(task_config):
    """Test a single hard task with auto-download"""
    print(f"\n{'='*70}")
    print(f"Testing: {task_config['name']}")
    print(f"Model: {task_config['model']}")
    print(f"{'='*70}")

    start_time = time.time()

    # Make the request
    response = requests.post(
        f"{API_BASE}/v1/chat/completions",
        json={
            "model": task_config["model"],
            "messages": task_config["messages"],
            "temperature": task_config["temperature"],
            "max_tokens": task_config["max_tokens"],
            "stream": False
        },
        timeout=300  # 5 minute timeout for download + processing
    )

    elapsed_time = time.time() - start_time

    print(f"\nResponse Status: {response.status_code}")
    print(f"Time Elapsed: {elapsed_time:.2f} seconds")

    if response.status_code == 200:
        data = response.json()

        # Extract response
        if 'choices' in data and len(data['choices']) > 0:
            content = data['choices'][0]['message']['content']
            print(f"\n--- Model Response (first 1500 chars) ---")
            print(content[:1500])
            if len(content) > 1500:
                print("\n... (truncated)")

            # Print statistics
            print(f"\n--- Statistics ---")
            usage = data.get('usage', {})
            print(f"Prompt Tokens: {usage.get('prompt_tokens', 'N/A')}")
            print(f"Completion Tokens: {usage.get('completion_tokens', 'N/A')}")
            print(f"Total Tokens: {usage.get('total_tokens', 'N/A')}")

            if 'model' in data:
                print(f"Model Used: {data['model']}")

            return True
        else:
            print("No response content in successful response")
            return False
    else:
        print(f"\n--- Error Response ---")
        try:
            error_data = response.json()
            print(json.dumps(error_data, indent=2))
        except:
            print(response.text[:500])
        return False

def test_streaming_hard_task():
    """Test streaming with a hard task and non-existent model"""
    print(f"\n{'='*70}")
    print("Testing: Streaming Response with Complex Task")
    print(f"{'='*70}")

    model = "TinyLlama/TinyLlama-1.1B-Chat-v1.0-GGUF:tinyllama-1.1b-chat-v1.0.Q4_K_M.gguf"

    response = requests.post(
        f"{API_BASE}/v1/chat/completions",
        json={
            "model": model,
            "messages": [
                {
                    "role": "system",
                    "content": "You are a creative writer."
                },
                {
                    "role": "user",
                    "content": "Write a complex nested story: A story about someone telling a story about someone writing a story. Each level should have different themes, time periods, and narrative styles. Make it mind-bending but coherent."
                }
            ],
            "temperature": 0.8,
            "max_tokens": 500,
            "stream": True
        },
        stream=True,
        timeout=120
    )

    print(f"Response Status: {response.status_code}")

    if response.status_code == 200:
        print("\n--- Streaming Response ---")
        full_response = ""
        chunk_count = 0

        for line in response.iter_lines():
            if line:
                chunk_count += 1
                line_str = line.decode('utf-8')
                if line_str.startswith("data: "):
                    data_str = line_str[6:]
                    if data_str == "[DONE]":
                        break
                    try:
                        data = json.loads(data_str)
                        if 'choices' in data and len(data['choices']) > 0:
                            delta = data['choices'][0].get('delta', {})
                            if 'content' in delta:
                                content = delta['content']
                                full_response += content
                                print(content, end='', flush=True)
                    except json.JSONDecodeError:
                        pass

        print(f"\n\n--- Streaming Statistics ---")
        print(f"Total Chunks: {chunk_count}")
        print(f"Response Length: {len(full_response)} characters")
        return True
    else:
        print("Streaming failed")
        return False

def run_all_hard_tests():
    """Run all hard task tests"""
    print("FrogLLM Hard Task Test Suite")
    print("Testing auto-download and complex task execution")
    print(f"API Base: {API_BASE}")
    print(f"Time: {time.strftime('%Y-%m-%d %H:%M:%S')}")

    # Check if server is running
    try:
        response = requests.get(f"{API_BASE}/api/system/specs", timeout=5)
        if response.status_code == 200:
            data = response.json()
            print(f"\nSystem: {len(data.get('gpus', []))} GPUs, {data.get('totalVRAM', 0)}GB VRAM")
        else:
            print("Warning: Could not get system specs")
    except requests.exceptions.RequestException as e:
        print(f"Error: Cannot connect to FrogLLM at {API_BASE}")
        print("Please ensure FrogLLM is running: ./frogllm")
        sys.exit(1)

    results = []

    # Test streaming first (usually faster with small model)
    print("\n" + "="*70)
    print("STREAMING TEST")
    print("="*70)
    streaming_result = test_streaming_hard_task()
    results.append(("Streaming Hard Task", streaming_result))

    # Test each hard task
    for i, task in enumerate(HARD_TASKS, 1):
        print(f"\n" + "="*70)
        print(f"HARD TASK {i}/{len(HARD_TASKS)}")
        print("="*70)

        result = test_hard_task(task)
        results.append((task['name'], result))

        # Brief pause between tests
        if i < len(HARD_TASKS):
            print("\nWaiting 5 seconds before next test...")
            time.sleep(5)

    # Print summary
    print("\n" + "="*70)
    print("TEST SUMMARY")
    print("="*70)

    for test_name, success in results:
        status = "✓ PASSED" if success else "✗ FAILED"
        print(f"{status}: {test_name}")

    passed = sum(1 for _, success in results if success)
    total = len(results)
    print(f"\nTotal: {passed}/{total} tests passed")

    return passed == total

if __name__ == "__main__":
    success = run_all_hard_tests()
    sys.exit(0 if success else 1)