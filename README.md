<div align="center">
  <h1>🐸 FrogLLM</h1>
  <h3>Leap into AI - The Amphibious LLM Server</h3>
  <img src="https://img.shields.io/badge/Powered%20by-Lily%20Pads-green?style=for-the-badge" alt="Powered by Lily Pads">
  <img src="https://img.shields.io/badge/GPU%20Pond-Ready-blue?style=for-the-badge" alt="GPU Pond Ready">
  <img src="https://img.shields.io/badge/Frog%20Approved-100%25-brightgreen?style=for-the-badge" alt="Frog Approved">
</div>

---

> 🐸 **"Ribbit! Welcome to the pond!"** - Where your AI models hop seamlessly between lily pads (GPUs) for maximum performance!

**FrogLLM** (formerly FrogLLM) is an intelligent auto-setup layer for llama.cpp that makes AI inference as smooth as a frog's leap. Just point it at your GGUF models, and watch it automatically configure everything - from GPU detection to optimal memory allocation. Built on the solid foundation of [llama-swap](https://github.com/mostlygeek/llama-swap), FrogLLM adds smart automation that makes deployment a breeze! 🌊

## 🏃 Quick Hop Start

```bash
# 🐸 One leap and you're ready!
curl -fsSL https://raw.githubusercontent.com/mat50013/llmfrog/main/scripts/install.sh | bash

# 🌊 Start the pond server
frogllm --models-folder /path/to/your/gguf/models --min-free-memory 15

# 🎉 Visit the lily pad: http://localhost:5800/ui/setup
```

## 🌟 What Makes FrogLLM Special?

### 🐸 Frog-Themed Features
- **🏞️ Frog Pond GPU Dashboard** - Watch your GPUs (lily pads) in real-time!
- **🌊 Smart Water Management** - Automatic VRAM (pond water) allocation
- **🐸 Frog-First UI** - Complete frog-themed interface with hop animations
- **💎 Crystal Clear Memory** - See exactly how much "pond water" is available

### 🚀 Powerful Features

#### 🎯 **Zero-Config Auto-Setup**
FrogLLM automatically detects your hardware and configures everything:
- 🔍 Finds all your GGUF models
- 🖥️ Detects GPUs (CUDA/ROCm/Vulkan/Metal)
- ⬇️ Downloads optimal llama.cpp binaries
- ⚙️ Generates production-ready configs
- 🐸 Starts serving immediately!

#### 💾 **Smart Memory Management**
Keep your pond healthy with intelligent memory management:
```bash
# Keep 20% of pond water free for other frogs
frogllm --min-free-memory 20
```
- Auto-unloads models when memory is low
- Prevents system crashes from memory exhaustion
- Monitors both GPU VRAM and system RAM

#### 🎯 **GPU-First Optimization**
FrogLLM always prioritizes GPU performance:
- Forces all layers to GPU with `-ngl 999`
- Uses all available GPUs automatically
- CUDA acceleration enabled by default
- Custom llama-server binary path support:
```bash
frogllm --llama-server-path /path/to/custom/llama-server
```

#### 📦 **Intelligent Model Downloads**
Never worry about split models again:
- **Automatic Split Model Detection** - Recognizes patterns like:
  - `model-00001-of-00005.gguf`
  - `model.Q4_K_M-00001-of-00003.gguf`
  - `model.gguf.part1of5`
- **Smart Grouping** - Groups split models for one-click download
- **Auto-Download on API Request** - Models download automatically when requested
- **HuggingFace Integration** - Direct download from HF with API key support
- **Command Line Token Support** - Set HF token directly via CLI:
```bash
frogllm --hf-token your_hf_token_here --models-folder /path/to/models
```

## 🐸 The Frog Pond Dashboard

Visit `http://localhost:5800/ui/gpu` to see your GPU pond in action:

```
🐸 Frog Pond GPU Status
🌊 Pond Backend: CUDA | Lily Pads: 8

🌊 Total Pond Water: 300.00 GB
💎 Crystal Clear: 250.00 GB
🐸 Frog Occupied: 50.00 GB

🐸 Lily Pad 0: NVIDIA RTX 6000
   🌊 Water Usage: ████████░░ 80%
   Temperature: 45°C | Power: 250W

🐸 Lily Pad 1: NVIDIA RTX 6000
   🌊 Water Usage: ██████░░░░ 60%
   Temperature: 42°C | Power: 200W
```

## 📦 Installation Methods

### 🐸 Native Installation (Recommended)

**Linux/macOS - Hop right in:**
```bash
curl -fsSL https://raw.githubusercontent.com/mat50013/llmfrog/main/scripts/install.sh | bash
```

**Windows - Leap from PowerShell:**
```powershell
irm https://raw.githubusercontent.com/mat50013/llmfrog/main/scripts/install.ps1 | iex
```

### 🐳 Docker Pond (GPU Support)

**NVIDIA GPUs (CUDA Pond):**
```bash
docker run -d --gpus all -p 5800:5800 \
  -v ./models:/models \
  frogllm:cuda --models-folder /models
```

**AMD GPUs (ROCm Pond):**
```bash
docker run -d --device=/dev/kfd --device=/dev/dri \
  -p 5800:5800 -v ./models:/models \
  frogllm:rocm --models-folder /models
```

### 🔨 Build Your Own Lily Pad

```bash
git clone https://github.com/mat50013/llmfrog.git
cd FrogLLM
python3 build.py  # Builds the complete pond ecosystem
```

## 🎮 Using FrogLLM

### 🐸 Basic Pond Setup
```bash
# Auto-setup with smart memory management
frogllm --models-folder ~/models --min-free-memory 15

# Custom GPU binary (for maximum hop speed)
frogllm --llama-server-path /opt/llama-cuda/llama-server

# Manual pond configuration
frogllm -ram 64 -vram 300 -backend cuda
```

### 🌊 API Usage

**Chat with your frogs:**
```bash
curl http://localhost:5800/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "llama-3-70b",
    "messages": [{"role": "user", "content": "Why do frogs love GPUs?"}]
  }'
```

**Auto-download models (they hop right in!):**
```bash
curl http://localhost:5800/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "HF-Token: your-token" \
  -d '{
    "model": "meta-llama/Llama-3-8B-Instruct",
    "messages": [{"role": "user", "content": "Ribbit!"}]
  }'
# FrogLLM will download the model automatically!
```

### 🖥️ Web Interface - The Lily Pad Control Center

- **🏞️ GPU Pond Monitor**: `http://localhost:5800/ui/gpu` - Watch your lily pads in real-time
- **🐸 Setup Wizard**: `http://localhost:5800/ui/setup` - Configure your pond
- **📦 Model Pond**: `http://localhost:5800/ui/models` - Manage your AI frogs
- **⬇️ Frog Downloader**: `http://localhost:5800/ui/downloader` - Get new models with smart grouping
- **⚙️ Pond Settings**: `http://localhost:5800/ui/config` - Fine-tune the ecosystem

## 🎯 Advanced Frog Features

### 🔄 Smart Model Grouping
FrogLLM automatically detects and groups split models:
- Downloads all parts with one click
- Preserves folder structure from HuggingFace
- Handles complex split patterns intelligently
- Shows combined size for split models

### 💾 Memory Pond Management
```yaml
groups:
  "frog-pond":
    minFreeMemoryPercent: 15.0  # Keep pond healthy
    autoUnload: true  # Frogs hop out when space is needed
    members: ["llama-70b", "qwen-72b", "mistral-7b"]
```

### 🚀 GPU Optimization
FrogLLM ensures maximum GPU utilization:
- All layers forced to GPU (`-ngl 999`)
- Multi-GPU support with `CUDA_VISIBLE_DEVICES`
- Automatic CUDA/ROCm/Vulkan detection
- Custom binary paths for specialized setups

## 🛠️ Configuration

FrogLLM generates smart configs automatically:

```yaml
# Auto-generated by FrogLLM 🐸
models:
  "llama-3-70b":
    cmd: |
      binaries/llama-server/llama-server
      --model models/llama-3-70b-q4.gguf
      --host 127.0.0.1 --port ${PORT}
      --flash-attn auto -ngl 999  # All layers on lily pads!
    proxy: "http://127.0.0.1:${PORT}"

groups:
  "frog-pond":
    minFreeMemoryPercent: 15.0
    autoUnload: true
    members: ["llama-3-70b", "qwen-72b"]
```

## 📚 API Endpoints

### 🐸 Core Frog Services
- `GET /v1/models` - List all swimming models with detailed information:
  - Model size (GB), quantization type, context length
  - Loading status (`loaded`, `unloaded`, `loading`)
  - File existence and path information
- `POST /v1/chat/completions` - Chat with your AI frogs
- `POST /v1/embeddings` - Get frog embeddings

### 🏞️ Pond Management
- `GET /api/gpu/stats` - Real-time lily pad statistics
- `GET /api/system/detection` - Detect pond hardware
- `POST /api/config/regenerate` - Rebuild the pond
- `GET /api/events` - Live pond events (SSE)

### 📦 Model Pond
- `POST /api/models/download` - Add new frogs to the pond
- `GET /api/models/downloads` - Check download progress
- `POST /api/models/load/{model}` - Load specific model (auto-download if needed)
- `POST /api/models/unload/{model}` - Unload individual model from lily pad
- `POST /api/models/unload` - Unload all models to free up the pond
- `POST /api/config/append-model` - Register new models

## 🙏 Credits & Acknowledgments

**FrogLLM hops on the shoulders of giants:**

- **[@mostlygeek](https://github.com/mostlygeek)** - Creator of [llama-swap](https://github.com/mostlygeek/llama-swap), the foundation that makes our pond possible! 🎉
- **[llama.cpp team](https://github.com/ggerganov/llama.cpp)** - The powerful inference engine
- **[Georgi Gerganov](https://github.com/ggerganov)** - Creator of llama.cpp

This project extends llama-swap with intelligent automation, maintaining 100% compatibility while adding the magic of automatic setup and frog-themed goodness! 🐸

## 🤝 Contributing

Help us make the pond better! We welcome all contributions:

1. Fork the pond 🍴
2. Create your feature branch (`git checkout -b feature/amazing-frog`)
3. Commit your changes (`git commit -m '🐸 Add amazing frog feature'`)
4. Push to the branch (`git push origin feature/amazing-frog`)
5. Open a Pull Request 🎉

## 📄 License

MIT License - Same as llama-swap. Free as a frog! See [LICENSE](LICENSE) for details.

## 🔗 Pond Links

- [🐸 FrogLLM Issues](https://github.com/prave/FrogLLM/issues)
- [🏗️ Original llama-swap](https://github.com/mostlygeek/llama-swap)
- [⚙️ llama.cpp](https://github.com/ggerganov/llama.cpp)
- [📚 Documentation Wiki](https://github.com/prave/FrogLLM/wiki)

---

<div align="center">

### 🐸 **Ribbit! Happy hopping!** 🐸

**Built with 💚 by the frog community, for the frog community**

*Leap into the future of AI inference*

</div>