# Yggstack Android Mobile Bindings

This document describes how to use Yggstack mobile bindings in your Android application.

## Building the Library

### Prerequisites

1. Go 1.22 or later
2. gomobile tools
3. Android SDK and NDK

### Build Steps

1. Install gomobile (if not already installed):
```bash
go install golang.org/x/mobile/cmd/gomobile@latest
go install golang.org/x/mobile/cmd/gobind@latest
```

2. Set Android SDK path:
```bash
export ANDROID_HOME=$HOME/Library/Android/sdk  # macOS
# or
export ANDROID_HOME=$HOME/Android/Sdk  # Linux
```

3. Run build script:
```bash
chmod +x build-android.sh
./build-android.sh
```

The AAR library will be created in `build/android/yggstack.aar`.

## Integration in Android Project

### 1. Add the AAR to your project

Copy `yggstack.aar` to your Android project's `app/libs` folder.

### 2. Update build.gradle

Add to your `app/build.gradle`:

```gradle
dependencies {
    implementation files('libs/yggstack.aar')
}
```

### 3. Add required permissions

Add to your `AndroidManifest.xml`:

```xml
<uses-permission android:name="android.permission.INTERNET" />
<uses-permission android:name="android.permission.ACCESS_NETWORK_STATE" />
```

## API Usage

### Basic Setup

```kotlin
import link.yggdrasil.yggstack.Mobile

class YggstackService {
    private var yggstack: Mobile.Yggstack? = null
    
    fun initialize() {
        // Create new instance
        yggstack = Mobile.NewYggstack()
        
        // Set log callback (optional)
        yggstack?.setLogCallback(object : Mobile.LogCallback {
            override fun onLog(message: String) {
                Log.d("Yggstack", message)
            }
        })
        
        // Set log level
        yggstack?.setLogLevel("info")
    }
}
```

### Generate or Load Configuration

#### Generate new configuration:

```kotlin
fun generateConfig(): String {
    val configJson = Mobile.GenerateConfig()
    // Save configJson to SharedPreferences or file
    return configJson
}
```

#### Load existing configuration:

```kotlin
fun loadConfig(configJson: String) {
    try {
        yggstack?.loadConfigJSON(configJson)
    } catch (e: Exception) {
        Log.e("Yggstack", "Failed to load config: ${e.message}")
    }
}
```

### Manage Peers

```kotlin
// Add a peer
fun addPeer(peerUri: String) {
    try {
        yggstack?.addPeer(peerUri)
        // Example: "tcp://1.2.3.4:5678"
    } catch (e: Exception) {
        Log.e("Yggstack", "Failed to add peer: ${e.message}")
    }
}

// Remove a peer
fun removePeer(peerUri: String) {
    try {
        yggstack?.removePeer(peerUri)
    } catch (e: Exception) {
        Log.e("Yggstack", "Failed to remove peer: ${e.message}")
    }
}

// Get all peers
fun getPeers(): List<String> {
    return try {
        val peersJson = yggstack?.getPeers() ?: "[]"
        Gson().fromJson(peersJson, Array<String>::class.java).toList()
    } catch (e: Exception) {
        emptyList()
    }
}
```

### Get Node Information

```kotlin
// Get IPv6 address
fun getAddress(): String? {
    return try {
        yggstack?.getAddress()
    } catch (e: Exception) {
        null
    }
}

// Get IPv6 subnet
fun getSubnet(): String? {
    return try {
        yggstack?.getSubnet()
    } catch (e: Exception) {
        null
    }
}

// Get public key
fun getPublicKey(): String? {
    return try {
        yggstack?.getPublicKey()
    } catch (e: Exception) {
        null
    }
}
```

### Start and Stop Yggstack

#### Start with SOCKS proxy:

```kotlin
fun start(socksAddress: String = "127.0.0.1:1080", nameserver: String = "") {
    try {
        yggstack?.start(socksAddress, nameserver)
        // SOCKS proxy is now running on 127.0.0.1:1080
    } catch (e: Exception) {
        Log.e("Yggstack", "Failed to start: ${e.message}")
    }
}
```

#### Start without SOCKS proxy:

```kotlin
fun startWithoutSocks() {
    try {
        yggstack?.start("", "")
        // Running as a node only, no SOCKS proxy
    } catch (e: Exception) {
        Log.e("Yggstack", "Failed to start: ${e.message}")
    }
}
```

#### Stop:

```kotlin
fun stop() {
    try {
        yggstack?.stop()
    } catch (e: Exception) {
        Log.e("Yggstack", "Failed to stop: ${e.message}")
    }
}
```

#### Check if running:

```kotlin
fun isRunning(): Boolean {
    return yggstack?.isRunning() ?: false
}
```

### Using DNS Nameserver

To resolve domain names (not just .pk.ygg domains), provide a Yggdrasil DNS server:

```kotlin
// Start with external DNS resolver
val nameserver = "[324:71e:281a:9ed3::53]:53"  // Example Yggdrasil DNS server
yggstack?.start("127.0.0.1:1080", nameserver)
```

## Complete Example

```kotlin
import android.app.Service
import android.content.Intent
import android.os.Binder
import android.os.IBinder
import android.util.Log
import com.google.gson.Gson
import link.yggdrasil.yggstack.Mobile

class YggstackService : Service() {
    
    private var yggstack: Mobile.Yggstack? = null
    private val binder = LocalBinder()
    
    inner class LocalBinder : Binder() {
        fun getService(): YggstackService = this@YggstackService
    }
    
    override fun onBind(intent: Intent): IBinder {
        return binder
    }
    
    override fun onCreate() {
        super.onCreate()
        
        // Initialize Yggstack
        yggstack = Mobile.NewYggstack()
        yggstack?.setLogCallback(object : Mobile.LogCallback {
            override fun onLog(message: String) {
                Log.d("Yggstack", message)
            }
        })
        yggstack?.setLogLevel("info")
        
        // Load or generate configuration
        val prefs = getSharedPreferences("yggstack", MODE_PRIVATE)
        var config = prefs.getString("config", null)
        
        if (config == null) {
            // Generate new config
            config = Mobile.GenerateConfig()
            prefs.edit().putString("config", config).apply()
        }
        
        try {
            yggstack?.loadConfigJSON(config)
            
            // Add some peers
            yggstack?.addPeer("tcp://1.2.3.4:5678")
            
        } catch (e: Exception) {
            Log.e("Yggstack", "Failed to initialize: ${e.message}")
        }
    }
    
    fun start(socksPort: Int = 1080) {
        try {
            val socksAddress = "127.0.0.1:$socksPort"
            yggstack?.start(socksAddress, "")
            Log.i("Yggstack", "Started on $socksAddress")
        } catch (e: Exception) {
            Log.e("Yggstack", "Failed to start: ${e.message}")
        }
    }
    
    fun stop() {
        try {
            yggstack?.stop()
            Log.i("Yggstack", "Stopped")
        } catch (e: Exception) {
            Log.e("Yggstack", "Failed to stop: ${e.message}")
        }
    }
    
    fun getNodeInfo(): Map<String, String> {
        return mapOf(
            "address" to (yggstack?.getAddress() ?: ""),
            "subnet" to (yggstack?.getSubnet() ?: ""),
            "publicKey" to (yggstack?.getPublicKey() ?: "")
        )
    }
    
    fun isRunning(): Boolean = yggstack?.isRunning() ?: false
    
    override fun onDestroy() {
        stop()
        super.onDestroy()
    }
}
```

## Features

### SOCKS5 Proxy
- ✅ Full SOCKS5 support
- ✅ Works without root/VPN
- ✅ Compatible with any SOCKS5-aware app

### Port Forwarding
- ⏳ Local TCP/UDP forwarding (planned)
- ⏳ Remote TCP/UDP forwarding (planned)

### DNS Resolution
- ✅ Built-in .pk.ygg resolver (publickey.pk.ygg format)
- ✅ External Yggdrasil DNS server support
- ✅ Standard .ygg domain support (with nameserver)

### Network
- ✅ No TUN interface required
- ✅ No VPN service needed
- ✅ Pure userspace networking via gvisor/netstack
- ✅ Multiple instances supported

## Configuration Options

### NodeConfig JSON Structure

```json
{
  "PrivateKey": "...",
  "Certificate": null,
  "Listen": ["tcp://[::]:0"],
  "Peers": [
    "tcp://1.2.3.4:5678"
  ],
  "InterfacePeers": {},
  "AllowedPublicKeys": [],
  "MulticastInterfaces": [
    {
      "Regex": ".*",
      "Beacon": true,
      "Listen": true,
      "Port": 0,
      "Priority": 0
    }
  ],
  "IfName": "none",
  "NodeInfo": {},
  "NodeInfoPrivacy": false
}
```

## Limitations

- Port forwarding API is work in progress
- Admin socket is disabled in mobile builds
- Multicast discovery may not work reliably on all Android devices

## Troubleshooting

### Build Issues

**Problem**: gomobile not found
```bash
go install golang.org/x/mobile/cmd/gomobile@latest
go install golang.org/x/mobile/cmd/gobind@latest
```

**Problem**: Android SDK not found
```bash
export ANDROID_HOME=/path/to/android/sdk
```

### Runtime Issues

**Problem**: SOCKS proxy not working
- Check if port is already in use
- Ensure INTERNET permission is granted
- Verify configuration is loaded correctly

**Problem**: Cannot resolve domains
- Provide a nameserver parameter when starting
- Use publickey.pk.ygg format for direct resolution

## License

This code is released under the LGPLv3 license. See LICENSE file for details.
