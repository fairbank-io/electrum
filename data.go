package electrum

import "encoding/json"

type Tx struct {
	Hash   string `json:"tx_hash"`
	Pos    uint64 `json:"tx_pos"`
	Height uint64 `json:"height"`
	Value  uint64 `json:"value"`
}

type Balance struct {
	Confirmed   uint64 `json:"confirmed"`
	Unconfirmed uint64 `json:"unconfirmed"`
}

type BlockHeader struct {
	BlockHeight   uint64 `json:"block_height"`
	PrevBlockHash string `json:"prev_block_hash"`
	Timestamp     uint64 `json:"timestamp"`
	Nonce         uint64 `json:"nonce"`
	MerkleRoot    string `json:"merkle_root"`
	UtxoRoot      string `json:"utxo_root"`
	Version       int    `json:"version"`
	Bits          uint64 `json:"bits"`
}

// Protocol response structure
// http://docs.electrum.org/en/latest/protocol.html#response
type response struct {
	Id     int         `json:"id"`
	Method string      `json:"method"`
	Params interface{} `json:"params"`
	Result interface{} `json:"result"`
}

// Protocol request structure
// http://docs.electrum.org/en/latest/protocol.html#request
type request struct {
	Id     int      `json:"id"`
	Method string   `json:"method"`
	Params []string `json:"params"`
}

// Properly encode a request object and append the message delimiter
func (r *request) encode() ([]byte, error) {
	b, err := json.Marshal(r)
	if err != nil {
		return nil, err
	}
	b = append(b, delimiter)
	return b, nil
}
