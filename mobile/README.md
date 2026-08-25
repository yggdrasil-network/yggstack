# Yggstack Mobile Bindings

Android bindings for Yggstack - Yggdrasil as SOCKS proxy without TUN interface.

## Features

- ✅ Full SOCKS5 proxy support
- ✅ No TUN interface required
- ✅ No VPN service needed on Android
- ✅ Pure userspace networking (gvisor/netstack)
- ✅ Built-in DNS resolver (.pk.ygg format)
- ✅ External DNS server support
- ✅ Configuration management
- ✅ Peer management
- ✅ Logging callbacks

## Files

- **`yggstack.go`** - Main mobile bindings implementation
- **`ANDROID.md`** - Complete Android integration guide with Kotlin examples

## Quick Start

### 1. Build the AAR library

```bash
./build-android.sh
```

### 2. Integrate in Android project

```gradle
dependencies {
    implementation files('libs/yggstack.aar')
}
```

### 3. Use in Kotlin

```kotlin
import link.yggdrasil.yggstack.Mobile

// Create instance
val yggstack = Mobile.NewYggstack()

// Generate config
val config = Mobile.GenerateConfig()
yggstack.loadConfigJSON(config)

// Add peers
yggstack.addPeer("tcp://1.2.3.4:5678")

// Start with SOCKS proxy
yggstack.start("127.0.0.1:1080", "")

// Stop
yggstack.stop()
```

## API Overview

### Core Functions
- `NewYggstack()` - Create new instance
- `GenerateConfig()` - Generate configuration
- `LoadConfigJSON(json)` - Load configuration
- `Start(socksAddr, nameserver)` - Start node
- `Stop()` - Stop node
- `IsRunning()` - Check status

### Configuration
- `AddPeer(uri)` - Add peer
- `RemovePeer(uri)` - Remove peer
- `GetPeers()` - Get peer list

### Node Info
- `GetAddress()` - Get IPv6 address
- `GetSubnet()` - Get IPv6 subnet
- `GetPublicKey()` - Get public key

### Logging
- `SetLogCallback(callback)` - Set log callback
- `SetLogLevel(level)` - Set log level
