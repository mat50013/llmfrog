# Using Custom llama-server Binary with FrogLLM

FrogLLM now supports replacing the llama-server binary path in your configuration and automatically rebuilding the application. This is useful when you have a custom-compiled llama-server or want to use a different version.

## Features

### 1. **--llama-server Flag** (New!)
Replace the llama-server binary path in existing config and rebuild automatically.

### 2. **--llama-server-path Flag**
Specify a custom llama-server path during initial setup/auto-configuration.

## Usage Examples

### Replace llama-server in Existing Config

```bash
# Replace with system-installed llama-server
./frogllm --llama-server /usr/local/bin/llama-server

# Replace with custom compiled version
./frogllm --llama-server ~/llama.cpp/build/bin/llama-server

# Replace with CUDA 12 specific build
./frogllm --llama-server ./binaries/llama-server-cuda12/llama-server

# Replace with ROCm build
./frogllm --llama-server /opt/rocm/bin/llama-server
```

### What Happens When You Use --llama-server

1. **Backup Created**: Your original config is backed up to `config.yaml.backup.TIMESTAMP`
2. **Path Replacement**: ALL occurrences of llama-server paths are replaced with your specified path
3. **Validation**: The tool checks that the binary exists at the specified path
4. **Automatic Rebuild**: FrogLLM is rebuilt with the new configuration
5. **Ready to Use**: The new binary uses your custom llama-server

### Initial Setup with Custom llama-server

When running auto-setup for the first time:

```bash
# Auto-setup with custom llama-server
./frogllm --models-folder /path/to/models --llama-server-path /usr/local/bin/llama-server
```

## Config File Changes

The tool updates llama-server references in:

1. **Macros Section**:
```yaml
macros:
  "llama-server-base": >
    /your/custom/path/llama-server  # <- Updated
    --host 127.0.0.1
    --port ${PORT}
```

2. **Model Commands**:
```yaml
models:
  "model-name":
    cmd: |
      /your/custom/path/llama-server  # <- Updated
      --model /path/to/model.gguf
```

## Use Cases

### Custom Optimizations
If you've compiled llama-server with specific optimizations:
```bash
# Compiled with AVX512 support
./frogllm --llama-server ~/builds/llama-server-avx512

# Compiled with custom BLAS library
./frogllm --llama-server ~/builds/llama-server-openblas
```

### Version Testing
Test different versions without changing your setup:
```bash
# Test latest version
./frogllm --llama-server ~/llama.cpp-latest/build/bin/llama-server

# Test stable version
./frogllm --llama-server ~/llama.cpp-stable/build/bin/llama-server
```

### Development/Debugging
When developing llama.cpp:
```bash
# Use debug build
./frogllm --llama-server ~/llama.cpp/build-debug/bin/llama-server

# Use release build
./frogllm --llama-server ~/llama.cpp/build-release/bin/llama-server
```

### Platform-Specific Binaries
```bash
# CUDA 11 systems
./frogllm --llama-server ./binaries/cuda11/llama-server

# CUDA 12 systems
./frogllm --llama-server ./binaries/cuda12/llama-server

# ROCm systems
./frogllm --llama-server ./binaries/rocm/llama-server

# Metal (macOS)
./frogllm --llama-server ./binaries/metal/llama-server
```

## Verification

After replacing, verify the change:

```bash
# Check config for new path
grep "llama-server" config.yaml

# Run FrogLLM and check logs
./frogllm

# The logs will show which llama-server binary is being used
```

## Rollback

If you need to revert:

```bash
# List backups
ls -la config.yaml.backup.*

# Restore a backup
cp config.yaml.backup.20250127-141523 config.yaml

# Rebuild with original config
python3 build.py
```

## Tips

1. **Always use absolute paths** when possible for consistency
2. **Check binary compatibility** - ensure the llama-server version supports all features used in your config
3. **Test after replacement** - run a simple model query to verify everything works
4. **Keep backups** - The tool auto-creates backups, but you can make additional copies

## Troubleshooting

### Binary Not Found
```
Error: llama-server binary not found at /path/to/llama-server
```
**Solution**: Verify the path exists and is executable:
```bash
ls -la /path/to/llama-server
chmod +x /path/to/llama-server  # Make executable if needed
```

### Incompatible Version
If models fail to load after replacement, the llama-server version might be incompatible.
**Solution**: Check llama-server version and features:
```bash
/path/to/llama-server --version
/path/to/llama-server --help
```

### Build Fails
If the rebuild fails after replacement:
**Solution**: Restore from backup and check build environment:
```bash
cp config.yaml.backup.TIMESTAMP config.yaml
go version  # Check Go is installed
python3 --version  # Check Python is installed
```

## Example Workflow

```bash
# 1. Check current setup
grep "llama-server" config.yaml

# 2. Build custom llama-server
cd ~/llama.cpp
mkdir build && cd build
cmake .. -DLLAMA_CUDA=ON
make -j8

# 3. Replace in FrogLLM
cd ~/FrogLLM
./frogllm --llama-server ~/llama.cpp/build/bin/llama-server

# 4. Verify
./frogllm --version
./frogllm  # Start server

# 5. Test with a query
curl http://localhost:5800/v1/models
```

This feature provides maximum flexibility for advanced users while maintaining simplicity for standard deployments.