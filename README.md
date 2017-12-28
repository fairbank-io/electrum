# Electrum Client

[![Build Status](https://travis-ci.org/fairbank-io/electrum.svg?branch=master)](https://travis-ci.org/fairbank-io/electrum)
[![GoDoc](https://godoc.org/github.com/fairbank-io/electrum?status.svg)](https://godoc.org/github.com/fairbank-io/electrum)
[![Version](https://img.shields.io/github/tag/fairbank-io/electrum.svg)](https://github.com/fairbank-io/electrum/releases)
[![Software License](https://img.shields.io/badge/license-MIT-brightgreen.svg)](LICENSE)

Provides a pure Go electrum protocol client implementation.

Features include:

 * Simple to use
 * Subscriptions are managed via __channels__ and __context__
 * Full TCP and __TSL__ support
 * Safe for concurrent execution

## Example

```go
// Start a new client instance
client, _ := electrum.New(&electrum.Options{
  Address:   "node.xbt.eu:50002",
  KeepAlive: true,
})

// Execute synchronous operation
version, _ := client.ServerVersion()

// Start a subscription, will terminate automatically after 30 seconds
ctx, cancel := context.WithTimeout(context.Background(), 30 * time.Second)
defer cancel()
headers, _ := client.NotifyBlockHeaders(ctx)
for header := range headers {
  // Use header
}

// Finish client execution
client.Close();
```
