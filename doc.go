/*
Package electrum provides an Electrum protocol client implementation.

The client supports two kind of operations, synchronous and asynchronous; for simplicity must
methods are exported as sync operations and only long running methods, i.e. subscriptions,
are exported as asynchronous.

Subscriptions take a context object that allows the client to cancel/close an instance at any
given time; subscriptions also returned a channel for data transfer, the channel will be
automatically closed by the client instance when the subscription is terminated.

The client supports TCP and TSL connections.

Creating a Client


First start a new client instance

  client, _ := electrum.New(&electrum.Options{
    Address:   "node.xbt.eu:50002",
    KeepAlive: true,
  })


Synchronous Operations

Execute operations as regular methods

  version, _ := client.ServerVersion()

Subscriptions

Get notifications using regular channels and context


  ctx, cancel := context.WithTimeout(context.Background(), 30 * time.Second)
  defer cancel()
  headers, _ := client.NotifyBlockHeaders(ctx)
    for header := range headers {
    // Use header
  }

Terminating a Client

When done with the client instance free-up resources and terminate network communications

  client.Close();

Protocol specification is available at:
http://docs.electrum.org/en/latest/protocol.html
*/
package electrum
