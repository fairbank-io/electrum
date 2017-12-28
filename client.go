package electrum

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"log"
	"strconv"
	"sync"
	"time"
)

// Message Delimiter, according to the protocol specification
// http://docs.electrum.org/en/latest/protocol.html#format
const delimiter = byte('\n')

// Options define the available configuration options
type Options struct {
	// Address of the server to use for network communications
	Address   string
	
	// Version advertised by the client instance
	Version   string
	
	// Protocol version preferred by the client instance
	Protocol  string
	
	// If set to true, will enable the client to continuously dispatch
	// a 'server.version' operation every 60 seconds
	KeepAlive bool
	
	// If provided, will be used to setup a secure network connection with the server
	TLS       *tls.Config
	
	// If provided, will be used as logging sink
	Log       *log.Logger
}

// Client defines the protocol client instance structure and interface
type Client struct {
	// Address of the remote server to use for communication
	Address string

	// Version of the client
	Version string

	// Protocol version preferred by the client instance
	Protocol string

	done      chan bool
	transport *transport
	counter   int
	subs      map[int]*subscription
	ping      *time.Ticker
	log       *log.Logger
	sync.Mutex
}

type subscription struct {
	method   string
	params   []string
	messages chan *response
	handler  func(*response)
	ctx      context.Context
}

// New will create and start processing on a new client instance
func New(options *Options) (*Client, error) {
	t, err := getTransport(&transportOptions{
		address: options.Address,
		tls:     options.TLS,
	})
	if err != nil {
		return nil, err
	}

	// By default use the latest supported protocol version
	if options.Protocol == "" {
		options.Protocol = "0.10"
	}

	// Use library version ad the default client version
	if options.Version == "" {
		options.Version = "0.1.0"
	}

	client := &Client{
		transport: t,
		counter:   0,
		done:      make(chan bool),
		subs:      make(map[int]*subscription),
		log:       options.Log,
		Address:   options.Address,
		Version:   options.Version,
		Protocol:  options.Protocol,
	}

	// Automatically send a 'server.version' request every 60 seconds as a keep-alive
	// signal to the server
	if options.KeepAlive {
		client.ping = time.NewTicker(60 * time.Second)
		go func() {
			for range client.ping.C {
				req := client.req("server.version", client.Version, client.Protocol)
				b, _ := req.encode()
				client.transport.sendMessage(b)
			}
		}()
	}

	go client.handleMessages()
	return client, nil
}

// Receive incoming network messages and the 'stop' signal
func (c *Client) handleMessages() {
	for {
		select {
		case <-c.done:
			if c.ping != nil {
				c.ping.Stop()
			}
			for i := range c.subs {
				c.removeSubscription(i)
			}
			return
		case err := <-c.transport.errors:
			if c.log != nil {
				c.log.Println(err)
			}
		case m := <-c.transport.messages:
			if c.log != nil {
				c.log.Println(m)
			}
			resp := &response{}
			if err := json.Unmarshal(m, resp); err != nil {
				break
			}

			if resp.Method != "" {
				c.Lock()
				for _, sub := range c.subs {
					if sub.method == resp.Method {
						sub.messages <- resp
					}
				}
				c.Unlock()
				break
			}

			// Message routed by ID
			c.Lock()
			sub, ok := c.subs[resp.Id]
			c.Unlock()
			if ok {
				sub.messages <- resp
			}
		}
	}
}

// Remove and existing messages subscription
func (c *Client) removeSubscription(id int) {
	c.Lock()
	defer c.Unlock()
	sub, ok := c.subs[id]
	if ok {
		close(sub.messages)
		delete(c.subs, id)
	}
}

// Build a request object
func (c *Client) req(name string, params ...string) *request {
	c.Lock()
	defer c.Unlock()

	// If no parameters are specified send an empty array
	// http://docs.electrum.org/en/latest/protocol.html#request
	if len(params) == 0 {
		params = []string{}
	}
	req := &request{
		Id:     c.counter,
		Method: name,
		Params: params,
	}
	c.counter++
	return req
}

// Dispatch a synchronous request, i.e. wait for it's result
func (c *Client) syncRequest(req *request) (*response, error) {
	// Setup a subscription for the request with proper cleanup
	res := make(chan *response)
	c.Lock()
	c.subs[req.Id] = &subscription{messages: res}
	c.Unlock()
	defer c.removeSubscription(req.Id)

	// Encode and dispatch the request
	b, err := req.encode()
	if err != nil {
		return nil, err
	}
	if err := c.transport.sendMessage(b); err != nil {
		return nil, err
	}

	// Log request
	if c.log != nil {
		c.log.Println(req)
	}

	// Wait for the response
	return <-res, nil
}

// Start a subscription processing loop
func (c *Client) startSubscription(sub *subscription) error {
	// Start processing loop
	// Will be terminating when closing the subscription's context or
	// by closing it's messages channel
	go func() {
		for {
			select {
			case msg, ok := <-sub.messages:
				if !ok {
					return
				}
				sub.handler(msg)
			case <-sub.ctx.Done():
				return
			}
		}
	}()

	// Register subscription
	req := c.req(sub.method, sub.params...)
	c.Lock()
	c.subs[req.Id] = sub
	c.Unlock()

	// Send request to the server
	b, err := req.encode()
	if err != nil {
		c.removeSubscription(req.Id)
		return err
	}
	if err := c.transport.sendMessage(b); err != nil {
		c.removeSubscription(req.Id)
		return err
	}
	return nil
}

// Close will finish execution and properly terminate the underlying network transport
func (c *Client) Close() {
	close(c.done)
	c.transport.close()
}

// ServerVersion will synchronously run a 'server.version' operation
//
// http://docs.electrum.org/en/latest/protocol.html#server-version
func (c *Client) ServerVersion() (string, error) {
	res, err := c.syncRequest(c.req("server.version", c.Version, c.Protocol))
	if err != nil {
		return "", err
	}
	return res.Result.(string), nil
}

// ServerBanner will synchronously run a 'server.banner' operation
//
// http://docs.electrum.org/en/latest/protocol.html#server-banner
func (c *Client) ServerBanner() (string, error) {
	res, err := c.syncRequest(c.req("server.banner"))
	if err != nil {
		return "", err
	}
	return res.Result.(string), nil
}

// ServerDonationAddress will synchronously run a 'server.donation_address' operation
//
// http://docs.electrum.org/en/latest/protocol.html#server-donation-address
func (c *Client) ServerDonationAddress() (string, error) {
	res, err := c.syncRequest(c.req("server.donation_address"))
	if err != nil {
		return "", err
	}
	return res.Result.(string), nil
}

// NotifyBlockHeaders will setup a subscription for the method 'blockchain.headers.subscribe'
//
// http://docs.electrum.org/en/latest/protocol.html#blockchain-headers-subscribe
func (c *Client) NotifyBlockHeaders(ctx context.Context) (<-chan *BlockHeader, error) {
	headers := make(chan *BlockHeader)
	sub := &subscription{
		ctx:      ctx,
		method:   "blockchain.headers.subscribe",
		messages: make(chan *response),
		handler: func(m *response) {
			if m.Result != nil {
				h := &BlockHeader{}
				b, _ := json.Marshal(m.Result)
				json.Unmarshal(b, h)
				headers <- h
			}

			if m.Params != nil {
				for _, i := range m.Params.([]interface{}) {
					h := &BlockHeader{}
					b, _ := json.Marshal(i)
					json.Unmarshal(b, h)
					headers <- h
				}
			}
		},
	}
	if err := c.startSubscription(sub); err != nil {
		close(headers)
		return nil, err
	}
	return headers, nil
}

// NotifyBlockNums will setup a subscription for the method 'blockchain.numblocks.subscribe'
//
// http://docs.electrum.org/en/latest/protocol.html#blockchain-numblocks-subscribe
func (c *Client) NotifyBlockNums(ctx context.Context) (<-chan int, error) {
	nums := make(chan int)
	sub := &subscription{
		ctx:      ctx,
		method:   "blockchain.numblocks.subscribe",
		messages: make(chan *response),
		handler: func(m *response) {
			if m.Result != nil {
				nums <- int(m.Result.(float64))
				return
			}

			if m.Params != nil {
				for _, v := range m.Params.([]interface{}) {
					nums <- int(v.(float64))
				}
			}
		},
	}
	if err := c.startSubscription(sub); err != nil {
		close(nums)
		return nil, err
	}
	return nums, nil
}

// NotifyAddressTransactions will setup a subscription for the method 'blockchain.address.subscribe'
//
// http://docs.electrum.org/en/latest/protocol.html#blockchain-address-subscribe
func (c *Client) NotifyAddressTransactions(ctx context.Context, address string) (<-chan string, error) {
	txs := make(chan string)
	sub := &subscription{
		ctx:      ctx,
		method:   "blockchain.address.subscribe",
		params:   []string{address},
		messages: make(chan *response),
		handler: func(m *response) {
			if m.Result != nil {
				txs <- m.Result.(string)
			}

			if m.Params != nil {
				for _, i := range m.Params.([]interface{}) {
					txs <- i.(string)
				}
			}
		},
	}
	if err := c.startSubscription(sub); err != nil {
		close(txs)
		return nil, err
	}
	return txs, nil
}

// AddressBalance will synchronously run a 'blockchain.address.get_balance' operation
//
// http://docs.electrum.org/en/latest/protocol.html#blockchain-address-get-balance
func (c *Client) AddressBalance(address string) (balance *Balance, err error) {
	res, err := c.syncRequest(c.req("blockchain.address.get_balance", address))
	if err != nil {
		return
	}

	b, _ := json.Marshal(res.Result)
	json.Unmarshal(b, &balance)
	return
}

// AddressHistory will synchronously run a 'blockchain.address.get_history' operation
//
// http://docs.electrum.org/en/latest/protocol.html#blockchain-address-get-history
func (c *Client) AddressHistory(address string) (list *[]Tx, err error) {
	res, err := c.syncRequest(c.req("blockchain.address.get_history", address))
	if err != nil {
		return
	}

	b, _ := json.Marshal(res.Result)
	json.Unmarshal(b, &list)
	return
}

// AddressMempool will synchronously run a 'blockchain.address.get_mempool' operation
//
// http://docs.electrum.org/en/latest/protocol.html#blockchain-address-get-mempool
func (c *Client) AddressMempool(address string) (list *[]Tx, err error) {
	res, err := c.syncRequest(c.req("blockchain.address.get_mempool", address))
	if err != nil {
		return
	}

	b, _ := json.Marshal(res.Result)
	json.Unmarshal(b, &list)
	return
}

// AddressListUnspent will synchronously run a 'blockchain.address.listunspent' operation
//
// http://docs.electrum.org/en/latest/protocol.html#blockchain-address-listunspent
func (c *Client) AddressListUnspent(address string) (list *[]Tx, err error) {
	res, err := c.syncRequest(c.req("blockchain.address.listunspent", address))
	if err != nil {
		return
	}

	b, _ := json.Marshal(res.Result)
	json.Unmarshal(b, &list)
	return
}

// BlockHeader will synchronously run a 'blockchain.block.get_header' operation
//
// http://docs.electrum.org/en/latest/protocol.html#blockchain-block-get-header
func (c *Client) BlockHeader(index int) (header *BlockHeader, err error) {
	res, err := c.syncRequest(c.req("blockchain.block.get_header", strconv.Itoa(index)))
	if err != nil {
		return
	}

	b, _ := json.Marshal(res.Result)
	json.Unmarshal(b, &header)
	return
}

// BroadcastTransaction will synchronously run a 'blockchain.transaction.broadcast' operation
//
// http://docs.electrum.org/en/latest/protocol.html#blockchain-transaction-broadcast
func (c *Client) BroadcastTransaction(hex string) (string, error) {
	res, err := c.syncRequest(c.req("blockchain.transaction.broadcast", hex))
	if err != nil {
		return "", err
	}
	return res.Result.(string), nil
}

// GetTransaction will synchronously run a 'blockchain.transaction.get' operation
//
// http://docs.electrum.org/en/latest/protocol.html#blockchain-transaction-get
func (c *Client) GetTransaction(hash string) (string, error) {
	res, err := c.syncRequest(c.req("blockchain.transaction.get", hash))
	if err != nil {
		return "", err
	}
	return res.Result.(string), nil
}

// EstimateFee will synchronously run a 'blockchain.estimatefee' operation
//
// http://docs.electrum.org/en/latest/protocol.html#blockchain-estimatefee
func (c *Client) EstimateFee(blocks int) (float64, error) {
	res, err := c.syncRequest(c.req("blockchain.estimatefee", strconv.Itoa(blocks)))
	if err != nil {
		return 0, err
	}
	return res.Result.(float64), nil
}

// Not implemented protocol methods for lack of documentation

// AddressProof will synchronously run a 'blockchain.address.get_proof' operation
//
// Not implemented yet
func (c *Client) AddressProof(address string) (string, error) {
	return "", errors.New("not implemented")
}

// UTXOAddress will synchronously run a 'blockchain.utxo.get_address' operation
//
// Not implemented yet
func (c *Client) UTXOAddress(utxo string) (string, error) {
	return "", errors.New("not implemented")
}

// BlockChunk will synchronously run a 'blockchain.block.get_chunk' operation
//
// Not implemented yet
func (c *Client) BlockChunk(index int) (interface{}, error) {
	return nil, errors.New("not implemented")
}

// TransactionMerkle will synchronously run a 'blockchain.transaction.get_merkle' operation
//
// Not implemented yet
func (c *Client) TransactionMerkle(tx string) (string, error) {
	return "", errors.New("not implemented")
}

// NotifyServerPeers will setup a subscription for the method 'server.peers.subscribe'
//
// Not implemented yet
func (c *Client) NotifyServerPeers() (<-chan string, error) {
	return nil, errors.New("not implemented")
}
