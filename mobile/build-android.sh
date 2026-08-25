#!/bin/bash

# Build script for Android AAR library
# This script builds yggstack mobile bindings for Android using gomobile

set -e

# Resolve module root (one level up from this script's directory)
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"
cd "$ROOT_DIR"

# Configuration
PACKAGE_NAME="link.yggdrasil.yggstack"
MIN_SDK=21
OUTPUT_DIR="android-build"
AAR_NAME="yggstack"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}Building Yggstack Android Library${NC}"
echo -e "${GREEN}========================================${NC}"

# Check if gomobile is installed
if ! command -v gomobile &> /dev/null; then
    echo -e "${RED}Error: gomobile is not installed${NC}"
    echo "Installing gomobile..."
    go install golang.org/x/mobile/cmd/gomobile@v0.0.0-20260203041319-574ceaa2f723
    go install golang.org/x/mobile/cmd/gobind@v0.0.0-20260203041319-574ceaa2f723
fi

# Prepare the gomobile toolchain directory.
# Do not use `gomobile init`: it unconditionally installs gobind@latest, which
# now requires Go >= 1.26. `gomobile bind` only needs this marker directory to
# exist plus gobind on PATH (same as what init provides).
echo -e "${YELLOW}Preparing gomobile toolchain...${NC}"
mkdir -p "$(go env GOPATH)/pkg/gomobile"

# Check if Android SDK/NDK is configured
if [ -z "$ANDROID_HOME" ] && [ -z "$ANDROID_SDK_ROOT" ]; then
    echo -e "${YELLOW}Warning: ANDROID_HOME or ANDROID_SDK_ROOT is not set${NC}"
    echo "Trying to detect Android SDK..."
    
    # Common Android SDK locations
    POSSIBLE_SDK_PATHS=(
        "$HOME/Library/Android/sdk"
        "$HOME/Android/Sdk"
        "/usr/local/android-sdk"
    )
    
    for path in "${POSSIBLE_SDK_PATHS[@]}"; do
        if [ -d "$path" ]; then
            export ANDROID_HOME="$path"
            echo -e "${GREEN}Found Android SDK at: $ANDROID_HOME${NC}"
            break
        fi
    done
    
    if [ -z "$ANDROID_HOME" ]; then
        echo -e "${RED}Error: Android SDK not found${NC}"
        echo "Please set ANDROID_HOME environment variable or install Android SDK"
        exit 1
    fi
fi

# Check if NDK is available
if [ ! -d "$ANDROID_HOME/ndk" ] && [ -z "$ANDROID_NDK_HOME" ]; then
    echo -e "${YELLOW}Warning: Android NDK not found${NC}"
    echo "Please install Android NDK via Android Studio or SDK Manager"
    echo "Attempting to continue anyway..."
fi

# Create output directory
echo -e "${YELLOW}Creating output directory...${NC}"
mkdir -p "$OUTPUT_DIR"

# Clean previous builds
if [ -d "$OUTPUT_DIR/$AAR_NAME.aar" ]; then
    echo -e "${YELLOW}Cleaning previous build...${NC}"
    rm -rf "$OUTPUT_DIR"/*
fi

# Detect build environment
# CI environment (GitHub Actions, etc.) builds production/release version
# Local development builds debug version with symbols
if [ -n "$CI" ] || [ -n "$GITHUB_ACTIONS" ]; then
    BUILD_TYPE="release"
    LDFLAGS="-s -w -checklinkname=0"
    echo -e "${GREEN}Building RELEASE version (symbols stripped)${NC}"
else
    BUILD_TYPE="debug"
    LDFLAGS="-checklinkname=0"
    echo -e "${YELLOW}Building DEBUG version (with symbols)${NC}"
fi

# Build for Android
echo -e "${YELLOW}Building Android AAR library...${NC}"
echo "Build type: $BUILD_TYPE"
echo "Package: $PACKAGE_NAME"
echo "Min SDK: $MIN_SDK"
echo "Architectures: arm64, arm, amd64, 386"
echo "LDFLAGS: $LDFLAGS"
echo ""

gomobile bind \
    -target=android \
    -androidapi=$MIN_SDK \
    -javapkg="$PACKAGE_NAME" \
    -ldflags="$LDFLAGS" \
    -o="$OUTPUT_DIR/$AAR_NAME.aar" \
    ./mobile/

# Check if build was successful
if [ -f "$OUTPUT_DIR/$AAR_NAME.aar" ]; then
    echo -e "${GREEN}========================================${NC}"
    echo -e "${GREEN}Build successful!${NC}"
    echo -e "${GREEN}========================================${NC}"
    echo ""
    echo "Output file: $OUTPUT_DIR/$AAR_NAME.aar"
    echo "Size: $(du -h "$OUTPUT_DIR/$AAR_NAME.aar" | cut -f1)"
    echo ""
    echo "To use in your Android project:"
    echo "1. Copy $AAR_NAME.aar to your Android project's libs folder"
    echo "2. Add to your app/build.gradle:"
    echo "   implementation files('libs/$AAR_NAME.aar')"
    echo ""
    echo "Java package: $PACKAGE_NAME"
    echo "Main class: $PACKAGE_NAME.Yggstack"
    echo ""
    
    # Display AAR contents
    echo "AAR contents:"
    if command -v unzip &> /dev/null; then
        unzip -l "$OUTPUT_DIR/$AAR_NAME.aar" | grep -E "\.(so|jar)$" || true
    fi
else
    echo -e "${RED}========================================${NC}"
    echo -e "${RED}Build failed!${NC}"
    echo -e "${RED}========================================${NC}"
    exit 1
fi
