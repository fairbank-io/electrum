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

// Available configuration options
type Options struct {
	Address   string
	Version   string
	Protocol  string
	KeepAlive bool
	TLS       *tls.Config
	Log       *log.Logger
}

// Protocol client instance
type Client struct {
	Address   string
	Version   string
	Protocol  string
	transport *transport
	counter   int
	ctx       context.Context
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
}

// Starts a new client instance; all operations and communications can be
// terminated using the provided context
func New(ctx context.Context, options *Options) (*Client, error) {
	t, err := getTransport(ctx, &transportOptions{
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
		ctx:       ctx,
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
		case <-c.ctx.Done():
			if c.ping != nil {
				c.ping.Stop()
			}
			for i := range c.subs {
				c.removeSubscription(i)
			}
			return
		case m := <-c.transport.messages:
			if c.log != nil {
				c.log.Println(m)
			}
			resp := &response{}
			if err := json.Unmarshal(m, resp); err != nil {
				break
			}

			if resp.Method != "" {
				for _, sub := range c.subs {
					if sub.method == resp.Method {
						sub.messages <- resp
					}
				}
				break
			}

			// Message routed by ID
			sub, ok := c.subs[resp.Id]
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

// Start a subscription processing loop; will be terminated when the subscription
// messages channel is closed, i.e., when the subscription is removed
func (c *Client) startSubscription(sub *subscription) error {
	// Start processing loop
	go func() {
		for msg := range sub.messages {
			sub.handler(msg)
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

// 'server.version'
// http://docs.electrum.org/en/latest/protocol.html#server-version
func (c *Client) ServerVersion() (string, error) {
	res, err := c.syncRequest(c.req("server.version", c.Version, c.Protocol))
	if err != nil {
		return "", err
	}
	return res.Result.(string), nil
}

// 'server.banner'
// http://docs.electrum.org/en/latest/protocol.html#server-banner
func (c *Client) ServerBanner() (string, error) {
	res, err := c.syncRequest(c.req("server.banner"))
	if err != nil {
		return "", err
	}
	return res.Result.(string), nil
}

// 'server.donation_address'
// http://docs.electrum.org/en/latest/protocol.html#server-donation-address
func (c *Client) ServerDonationAddress() (string, error) {
	res, err := c.syncRequest(c.req("server.donation_address"))
	if err != nil {
		return "", err
	}
	return res.Result.(string), nil
}

// 'blockchain.headers.subscribe'
// http://docs.electrum.org/en/latest/protocol.html#blockchain-headers-subscribe
func (c *Client) NotifyBlockHeaders() (<-chan *BlockHeader, error) {
	headers := make(chan *BlockHeader)
	sub := &subscription{
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

// blockchain.numblocks.subscribe
// http://docs.electrum.org/en/latest/protocol.html#blockchain-numblocks-subscribe
func (c *Client) NotifyBlockNums() (<-chan int, error) {
	nums := make(chan int)
	sub := &subscription{
		method:   "blockchain.numblocks.subscribe",
		messages: make(chan *response),
		handler: func(m *response) {
			if m.Result != nil {
				nums <- int(m.Result.(float64))
				return
			}

			if m.Params != nil {
				log.Printf("parsing: %+v", m)
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

// blockchain.address.subscribe
// http://docs.electrum.org/en/latest/protocol.html#blockchain-address-subscribe
func (c *Client) NotifyAddressTransactions(address string) (<-chan string, error) {
	txs := make(chan string)
	sub := &subscription{
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

// 'blockchain.address.get_balance'
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

// 'blockchain.address.get_history'
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

// 'blockchain.address.get_mempool'
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

// 'blockchain.address.listunspent'
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

// 'blockchain.block.get_header'
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

// 'blockchain.transaction.broadcast'
// http://docs.electrum.org/en/latest/protocol.html#blockchain-transaction-broadcast
func (c *Client) BroadcastTransaction(hex string) (string, error) {
	res, err := c.syncRequest(c.req("blockchain.transaction.broadcast", hex))
	if err != nil {
		return "", err
	}
	return res.Result.(string), nil
}

// 'blockchain.transaction.get'
// http://docs.electrum.org/en/latest/protocol.html#blockchain-transaction-get
func (c *Client) GetTransaction(hash string) (string, error) {
	res, err := c.syncRequest(c.req("blockchain.transaction.get", hash))
	if err != nil {
		return "", err
	}
	return res.Result.(string), nil
}

// 'blockchain.estimatefee'
// http://docs.electrum.org/en/latest/protocol.html#blockchain-estimatefee
func (c *Client) EstimateFee(blocks int) (float64, error) {
	res, err := c.syncRequest(c.req("blockchain.estimatefee", strconv.Itoa(blocks)))
	if err != nil {
		return 0, err
	}
	return res.Result.(float64), nil
}

// Not implemented protocol methods for lack of documentation

// blockchain.address.get_proof
func (c *Client) AddressProof(address string) (string, error) {
	return "", errors.New("not implemented")
}

// blockchain.utxo.get_address
func (c *Client) UTXOAddress(utxo string) (string, error) {
	return "", errors.New("not implemented")
}

// blockchain.block.get_chunk
func (c *Client) BlockChunk(index int) (interface{}, error) {
	return nil, errors.New("not implemented")
}

// blockchain.transaction.get_merkle
func (c *Client) TransactionMerkle(tx string) (string, error) {
	return "", errors.New("not implemented")
}

// server.peers.subscribe
func (c *Client) NotifyServerPeers() (<-chan string, error) {
	return nil, errors.New("not implemented")
}
