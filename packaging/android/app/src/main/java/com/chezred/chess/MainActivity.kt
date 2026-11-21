package com.chezred.chess

import android.annotation.SuppressLint
import android.content.Context
import android.net.ConnectivityManager
import android.net.LinkProperties
import android.net.Network
import android.net.NetworkCapabilities
import android.net.wifi.WifiManager
import android.os.Bundle
import android.webkit.WebView
import android.webkit.WebViewClient
import android.widget.Toast
import androidx.activity.ComponentActivity
import com.chezred.go.android.Android

class MainActivity : ComponentActivity() {
    private lateinit var webView: WebView
    private var multicastLock: WifiManager.MulticastLock? = null

    private val connectivityCallback = object : ConnectivityManager.NetworkCallback() {
        override fun onCapabilitiesChanged(network: Network, capabilities: NetworkCapabilities) {
            updateNetworkAddresses()
        }

        override fun onLinkPropertiesChanged(network: Network, linkProperties: LinkProperties) {
            updateNetworkAddresses()
        }
    }

    @SuppressLint("SetJavaScriptEnabled")
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)

        val wifi = applicationContext.getSystemService(Context.WIFI_SERVICE) as WifiManager
        multicastLock = wifi.createMulticastLock("AjedrezEnRedDiscovery").apply {
            setReferenceCounted(true)
            acquire()
        }

        updateNetworkAddresses()

        val url = try {
            Android.start(8765)
        } catch (error: Exception) {
            Toast.makeText(this, "No se pudo iniciar el juego: ${error.message}", Toast.LENGTH_LONG).show()
            null
        }

        webView = WebView(this).apply {
            settings.javaScriptEnabled = true
            settings.domStorageEnabled = true
            settings.databaseEnabled = true
            settings.allowFileAccess = false
            settings.allowContentAccess = false
            webViewClient = WebViewClient()
        }
        setContentView(webView)

        if (url != null) {
            webView.loadUrl(url)
        }

        val connectivity = getSystemService(Context.CONNECTIVITY_SERVICE) as ConnectivityManager
        connectivity.registerDefaultNetworkCallback(connectivityCallback)
    }

    private fun updateNetworkAddresses() {
        val connectivity = getSystemService(Context.CONNECTIVITY_SERVICE) as ConnectivityManager
        val network = connectivity.activeNetwork ?: return
        val capabilities = connectivity.getNetworkCapabilities(network) ?: return
        if (!capabilities.hasTransport(NetworkCapabilities.TRANSPORT_WIFI)) return

        val addresses = connectivity.getLinkProperties(network)?.linkAddresses
            ?.mapNotNull { it.address.hostAddress }
            ?.filter { it.matches(Regex("\\d{1,3}(\\.\\d{1,3}){3}")) }
            ?.distinct()
            .orEmpty()

        val primary = addresses.firstOrNull().orEmpty()
        if (primary.isNotEmpty()) {
            Android.setLocalNetwork(primary, addresses.joinToString(","))
        }
    }

    override fun onDestroy() {
        try {
            val connectivity = getSystemService(Context.CONNECTIVITY_SERVICE) as ConnectivityManager
            connectivity.unregisterNetworkCallback(connectivityCallback)
        } catch (_: Exception) {
        }

        multicastLock?.takeIf { it.isHeld }?.release()

        try {
            Android.shutdown()
        } catch (_: Exception) {
        }

        if (::webView.isInitialized) {
            webView.loadUrl("about:blank")
            webView.destroy()
        }
        super.onDestroy()
    }
}
