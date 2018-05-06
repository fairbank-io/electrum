package electrum

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"sync"
	"time"
)

// Client version
const Version = "0.3.0"

// Protocol tags
const Protocol10 = "1.0"
const Protocol11 = "1.1"
const Protocol12 = "1.2"

// Common errors
var (
	ErrDeprecatedMethod  = errors.New("DEPRECATED_METHOD")
	ErrUnavailableMethod = errors.New("UNAVAILABLE_METHOD")
	ErrRejectedTx        = errors.New("REJECTED_TRANSACTION")
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

	// Agent identifier that will be transmitted to the server when required;
	// will be concatenated with the client version
	Agent string
	
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
	agent     string
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
	// https://electrumx.readthedocs.io/en/latest/protocol-changes.html
	if options.Protocol == "" {
		options.Protocol = Protocol12
	}

	// Use library version as default client version
	if options.Version == "" {
		options.Version = Version
	}

	// Use library identifier as default agent name
	if options.Agent == "" {
		options.Agent = "fairbank-electrum"
	}

	client := &Client{
		transport: t,
		counter:   0,
		done:      make(chan bool),
		subs:      make(map[int]*subscription),
		log:       options.Log,
		agent:     fmt.Sprintf("%s-%s", options.Agent, options.Version),
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
				var req *request
				switch client.Protocol {
				case Protocol12:
					req = client.req("server.ping")
				default:
					req = client.req("server.version", client.Version, client.Protocol)
				}
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

// Ping the server to ensure it is responding, and to keep the session alive.
// The server may disconnect clients that have sent no requests for roughly 10 minutes.
//
// https://electrumx.readthedocs.io/en/latest/protocol-methods.html#server-ping
func (c *Client) ServerPing() error {
	switch c.Protocol {
	case Protocol12:
		_, err := c.syncRequest(c.req("server.ping"))
		return err
	default:
		return ErrUnavailableMethod
	}
}

// ServerVersion will synchronously run a 'server.version' operation
//
// https://electrumx.readthedocs.io/en/latest/protocol-methods.html#server-version
func (c *Client) ServerVersion() (*VersionInfo, error) {
	res, err := c.syncRequest(c.req("server.version", c.agent, c.Protocol))
	if err != nil {
		return nil, err
	}

	if res.Error != nil {
		return nil, errors.New(res.Error.Message)
	}

	info := &VersionInfo{}
	switch c.Protocol {
	case Protocol10:
		info.Software = res.Result.(string)
	case Protocol11:
		fallthrough
	case Protocol12:
		var d []string
		b, _ := json.Marshal(res.Result)
		json.Unmarshal(b, &d)
		info.Software = d[0]
		info.Protocol = d[1]
	}
	return info, nil
}

// ServerBanner will synchronously run a 'server.banner' operation
//
// https://electrumx.readthedocs.io/en/latest/protocol-methods.html#server-banner
func (c *Client) ServerBanner() (string, error) {
	res, err := c.syncRequest(c.req("server.banner"))
	if err != nil {
		return "", err
	}

	if res.Error != nil {
		return "", errors.New(res.Error.Message)
	}

	return res.Result.(string), nil
}

// ServerDonationAddress will synchronously run a 'server.donation_address' operation
//
// https://electrumx.readthedocs.io/en/latest/protocol-methods.html#server-donation-address
func (c *Client) ServerDonationAddress() (string, error) {
	res, err := c.syncRequest(c.req("server.donation_address"))
	if err != nil {
		return "", err
	}

	if res.Error != nil {
		return "", errors.New(res.Error.Message)
	}

	return res.Result.(string), nil
}

// Return a list of features and services supported by the server
//
// https://electrumx.readthedocs.io/en/latest/protocol-methods.html#server-donation-address
func (c *Client) ServerFeatures() (info *ServerInfo, err error) {
	switch c.Protocol {
	case Protocol10:
		err = ErrUnavailableMethod
	default:
		res, err := c.syncRequest(c.req("server.features"))
		if err != nil {
			break
		}

		if res.Error != nil {
			err = errors.New(res.Error.Message)
			break
		}

		b, _ := json.Marshal(res.Result)
		json.Unmarshal(b, &info)
	}
	return
}

// Return a list of peer servers
//
// https://electrumx.readthedocs.io/en/latest/protocol-methods.html#server-peers-subscribe
func (c *Client) ServerPeers() (peers []*Peer, err error) {
	res, err := c.syncRequest(c.req("server.peers.subscribe"))
	if err != nil {
		return
	}

	if res.Error != nil {
		err = errors.New(res.Error.Message)
		return
	}

	var list []interface{}
	b, _ := json.Marshal(res.Result)
	json.Unmarshal(b, &list)

	for _, l := range list {
		p := &Peer{
			Address: l.([]interface{})[0].(string),
			Name:    l.([]interface{})[1].(string),
		}
		b, _ := json.Marshal(l.([]interface{})[2])
		json.Unmarshal(b, &p.Features)
		peers = append(peers, p)
	}
	return
}

// NotifyBlockHeaders will setup a subscription for the method 'blockchain.headers.subscribe'
//
// https://electrumx.readthedocs.io/en/latest/protocol-methods.html#blockchain-headers-subscribe
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

// NotifyAddressTransactions will setup a subscription for the method 'blockchain.address.subscribe'
//
// https://electrumx.readthedocs.io/en/latest/protocol-methods.html#blockchain-address-subscribe
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
// https://electrumx.readthedocs.io/en/latest/protocol-methods.html#blockchain-address-get-balance
func (c *Client) AddressBalance(address string) (balance *Balance, err error) {
	res, err := c.syncRequest(c.req("blockchain.address.get_balance", address))
	if err != nil {
		return
	}

	if res.Error != nil {
		err = errors.New(res.Error.Message)
		return
	}

	b, _ := json.Marshal(res.Result)
	json.Unmarshal(b, &balance)
	return
}

// AddressHistory will synchronously run a 'blockchain.address.get_history' operation
//
// https://electrumx.readthedocs.io/en/latest/protocol-methods.html#blockchain-address-get-history
func (c *Client) AddressHistory(address string) (list *[]Tx, err error) {
	res, err := c.syncRequest(c.req("blockchain.address.get_history", address))
	if err != nil {
		return
	}

	if res.Error != nil {
		err = errors.New(res.Error.Message)
		return
	}

	b, _ := json.Marshal(res.Result)
	json.Unmarshal(b, &list)
	return
}

// AddressMempool will synchronously run a 'blockchain.address.get_mempool' operation
//
// https://electrumx.readthedocs.io/en/latest/protocol-methods.html#blockchain-address-get-mempool
func (c *Client) AddressMempool(address string) (list *[]Tx, err error) {
	res, err := c.syncRequest(c.req("blockchain.address.get_mempool", address))
	if err != nil {
		return
	}

	if res.Error != nil {
		err = errors.New(res.Error.Message)
		return
	}

	b, _ := json.Marshal(res.Result)
	json.Unmarshal(b, &list)
	return
}

// AddressListUnspent will synchronously run a 'blockchain.address.listunspent' operation
//
// https://electrumx.readthedocs.io/en/latest/protocol-methods.html#blockchain-address-listunspent
func (c *Client) AddressListUnspent(address string) (list *[]Tx, err error) {
	res, err := c.syncRequest(c.req("blockchain.address.listunspent", address))
	if err != nil {
		return
	}

	if res.Error != nil {
		err = errors.New(res.Error.Message)
		return
	}

	b, _ := json.Marshal(res.Result)
	json.Unmarshal(b, &list)
	return
}

// BlockHeader will synchronously run a 'blockchain.block.get_header' operation
//
// https://electrumx.readthedocs.io/en/latest/protocol-methods.html#blockchain-block-get-header
func (c *Client) BlockHeader(index int) (header *BlockHeader, err error) {
	res, err := c.syncRequest(c.req("blockchain.block.get_header", strconv.Itoa(index)))
	if err != nil {
		return
	}

	if res.Error != nil {
		err = errors.New(res.Error.Message)
		return
	}

	b, _ := json.Marshal(res.Result)
	json.Unmarshal(b, &header)
	return
}

// BroadcastTransaction will synchronously run a 'blockchain.transaction.broadcast' operation
//
// https://electrumx.readthedocs.io/en/latest/protocol-methods.html#blockchain-transaction-broadcast
func (c *Client) BroadcastTransaction(hex string) (string, error) {
	res, err := c.syncRequest(c.req("blockchain.transaction.broadcast", hex))
	if err != nil {
		return "", err
	}

	if res.Error != nil {
		return "", errors.New(res.Error.Message)
	}

	if res.Result == nil {
		return "", ErrRejectedTx
	}

	return res.Result.(string), nil
}

// GetTransaction will synchronously run a 'blockchain.transaction.get' operation
//
// https://electrumx.readthedocs.io/en/latest/protocol-methods.html#blockchain.transaction.get
func (c *Client) GetTransaction(hash string) (string, error) {
	res, err := c.syncRequest(c.req("blockchain.transaction.get", hash))
	if err != nil {
		return "", err
	}

	if res.Error != nil {
		return "", errors.New(res.Error.Message)
	}

	return res.Result.(string), nil
}

// EstimateFee will synchronously run a 'blockchain.estimatefee' operation
//
// https://electrumx.readthedocs.io/en/latest/protocol-methods.html#blockchain-estimatefee
func (c *Client) EstimateFee(blocks int) (float64, error) {
	res, err := c.syncRequest(c.req("blockchain.estimatefee", strconv.Itoa(blocks)))
	if err != nil {
		return 0, err
	}

	if res.Error != nil {
		return 0, errors.New(res.Error.Message)
	}

	return res.Result.(float64), nil
}

// TransactionMerkle will synchronously run a 'blockchain.transaction.get_merkle' operation
//
// https://electrumx.readthedocs.io/en/latest/protocol-methods.html#blockchain-transaction-get-merkle
func (c *Client) TransactionMerkle(tx string, height int) (tm *TxMerkle, err error) {
	res, err := c.syncRequest(c.req("blockchain.transaction.get_merkle", tx, strconv.Itoa(height)))
	if err != nil {
		return
	}

	if res.Error != nil {
		err = errors.New(res.Error.Message)
		return
	}

	b, _ := json.Marshal(res.Result)
	json.Unmarshal(b, &tm)
	return
}

/*-----------------
 DEPRECATED METHODS
 -----------------*/

// UTXOAddress will synchronously run a 'blockchain.utxo.get_address' operation
// https://electrumx.readthedocs.io/en/latest/protocol-methods.html#blockchain.utxo.get_address
//
// Deprecated: Since protocol 1.0
// https://electrumx.readthedocs.io/en/latest/protocol-changes.html#deprecated-methods
func (c *Client) UTXOAddress(utxo string) (string, error) {
	return "", ErrDeprecatedMethod
}

// BlockChunk will synchronously run a 'blockchain.block.get_chunk' operation
// https://electrumx.readthedocs.io/en/latest/protocol-methods.html#blockchain.block.get_chunk
//
// Deprecated: Since protocol 1.2
// https://electrumx.readthedocs.io/en/latest/protocol-changes.html#version-1-2
func (c *Client) BlockChunk(index int) (interface{}, error) {
	return nil, ErrDeprecatedMethod
}

// NotifyBlockNums will setup a subscription for the method 'blockchain.numblocks.subscribe'
// https://electrumx.readthedocs.io/en/latest/protocol-methods.html#blockchain.numblocks.subscribe
//
// Deprecated: Since protocol 1.0
// https://electrumx.readthedocs.io/en/latest/protocol-changes.html#deprecated-methods
func (c *Client) NotifyBlockNums(ctx context.Context) (<-chan int, error) {
	return nil, ErrDeprecatedMethod

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
