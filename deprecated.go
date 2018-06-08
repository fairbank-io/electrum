package electrum

import "context"

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
