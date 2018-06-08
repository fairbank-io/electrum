package electrum

import (
	"context"
	"encoding/json"
)

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
