# Yggstack AAR Build Modes

This document explains the dual-build system for the Yggstack Android AAR library.

## Build Modes

### Debug Build (Local Development)
- **When**: Building locally on developer machine
- **Detection**: `CI` and `GITHUB_ACTIONS` environment variables are not set
- **LDFLAGS**: `-checklinkname=0`
- **Symbols**: ✅ **Included** (full debug symbols, DWARF info)
- **Size**: ~37M
- **Use case**: Local development, debugging native crashes with full stack traces

```bash
# Build debug version (default for local builds)
./build-android.sh
```

### Release Build (Production/CI)
- **When**: Building in CI/CD pipelines (GitHub Actions)
- **Detection**: `CI=true` or `GITHUB_ACTIONS=true`
- **LDFLAGS**: `-s -w -checklinkname=0`
  - `-s`: Strip symbol table
  - `-w`: Strip DWARF debug info
- **Symbols**: ❌ **Stripped**
- **Size**: ~20M (46% smaller!)
- **Use case**: Production releases, optimized for size and performance

```bash
# Build release version (simulating CI)
CI=true ./build-android.sh
```

## Size Comparison

| Component | Debug | Release | Savings |
|-----------|-------|---------|---------|
| AAR Library | 37M | 20M | **-46%** |
| APK (arm64) | 24M | 18M | **-25%** |
| APK (universal) | 77M | 56M | **-27%** |

## Trade-offs

### Debug Build
✅ Full native crash debugging with function names and line numbers  
✅ Detailed stack traces with gdb/lldb  
❌ Larger file size  
❌ Higher memory usage  

### Release Build
✅ 46% smaller AAR/APK size  
✅ Reduced memory footprint  
✅ Faster library loading  
❌ Native crashes show addresses only (no function names)  
❌ Limited debugging capability for Go code  

## Automatic Behavior

The build script automatically detects the environment:

- **GitHub Actions**: Automatically builds release version
- **Local development**: Automatically builds debug version
- **Manual override**: Set `CI=true` to force release build locally

## Testing Release Build Locally

To test the production release build before pushing:

```bash
cd lib/yggstack
CI=true ./build-android.sh
```

This ensures the same optimized binary that will be used in production.
