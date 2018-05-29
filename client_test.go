package electrum

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"strings"
	"testing"
	"time"
)

func TestClient(t *testing.T) {
	const testAddress = "1ErbiumBjW4ScHNhLCcNWK5fFsKFpsYpWb"
	const testServer = "electrum.villocq.com:50002" // erbium1.sytes.net:50002 | ex-btc.server-on.net:50002

	t.Run("Protocol_1.0", func(t *testing.T) {
		client, err := New(&Options{
			Address:   testServer,
			TLS:       &tls.Config{InsecureSkipVerify: true},
			Protocol:  Protocol10,
			KeepAlive: true,
		})
		if err != nil {
			t.Error(err)
			return
		}
		defer client.Close()

		t.Run("ServerPing", func(t *testing.T) {
			err := client.ServerPing()
			if err != ErrUnavailableMethod {
				t.Error(err)
				return
			}
			log.Println("Pong")
		})

		t.Run("ServerVersion", func(t *testing.T) {
			res, err := client.ServerVersion()
			if err != nil {
				t.Error(err)
				return
			}
			log.Printf("Server Software: %s\n", res.Software)
			log.Printf("Server Protocol: %s\n", res.Protocol)
		})

		t.Run("ServerBanner", func(t *testing.T) {
			res, err := client.ServerBanner()
			if err != nil {
				t.Error(err)
				return
			}
			log.Printf("Server Banner: %s\n", res)
		})

		t.Run("ServerDonationAddress", func(t *testing.T) {
			res, err := client.ServerDonationAddress()
			if err != nil {
				t.Error(err)
				return
			}
			log.Printf("Server Donation Address: %s\n", res)
		})

		t.Run("ServerFeatures", func(t *testing.T) {
			_, err := client.ServerFeatures()
			if err != ErrUnavailableMethod {
				t.Error(err)
				return
			}
		})

		t.Run("ServerPeers", func(t *testing.T) {
			peers, err := client.ServerPeers()
			if err != nil {
				t.Error(err)
				return
			}
			log.Println("Server peers")
			for _, p := range peers {
				log.Printf("%s > %s (%v)", p.Name, p.Address, p.Features)
			}
		})

		t.Run("AddressBalance", func(t *testing.T) {
			balance, err := client.AddressBalance(testAddress)
			if err != nil {
				t.Error(err)
				return
			}
			log.Printf("Balance: %+v\n", balance)
		})

		t.Run("AddressMempool", func(t *testing.T) {
			mempool, err := client.AddressMempool(testAddress)
			if err != nil {
				t.Error(err)
				return
			}
			log.Printf("Mempool: %+v\n", mempool)
		})

		t.Run("AddressHistory", func(t *testing.T) {
			history, err := client.AddressHistory(testAddress)
			if err != nil {
				t.Error(err)
				return
			}
			log.Printf("History: %+v\n", history)
		})

		t.Run("AddressListUnspent", func(t *testing.T) {
			utxo, err := client.AddressListUnspent(testAddress)
			if err != nil {
				t.Error(err)
				return
			}
			log.Printf("Unspent: %+v\n", utxo)
		})

		t.Run("BlockHeader", func(t *testing.T) {
			header, err := client.BlockHeader(56770)
			if err != nil {
				t.Error(err)
				return
			}
			log.Printf("Header: %+v\n", header)
		})

		t.Run("BroadcastTransaction", func(t *testing.T) {
			_, err := client.BroadcastTransaction("01000000010000000000000000000000000000000000000000000000000000000000000000ffffffff0702621b03cfd201ffffffff010000000000000000016a00000000")
			if err != ErrRejectedTx {
				t.Error(errors.New("unexpected result"))
				return
			}
		})

		t.Run("GetTransaction", func(t *testing.T) {
			res, err := client.GetTransaction("4f73e43b92d337da8e69417601de1476bd7577cbac901fa28dba37ce1362adb9")
			if err != nil {
				t.Error(err)
				return
			}
			log.Printf("Tx: %+v\n", res)
		})

		t.Run("EstimateFee", func(t *testing.T) {
			fee, err := client.EstimateFee(6)
			if err != nil {
				t.Error(err)
				return
			}
			log.Printf("Fee: %+v\n", fee)
		})

		t.Run("NotifyBlockNums", func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			nums, err := client.NotifyBlockNums(ctx)
			if err != ErrDeprecatedMethod {
				t.Error(err)
				return
			}
			for {
				select {
				case n := <-nums:
					log.Printf("%+v\n", n)
				case <-ctx.Done():
					return
				}
			}
		})

		t.Run("NotifyBlockHeaders", func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10 * time.Second)
			defer cancel()
			headers, err := client.NotifyBlockHeaders(ctx)
			if err != nil {
				t.Error(err)
				return
			}
			for {
				select {
				case h := <-headers:
					log.Printf("%+v\n", h)
				case <-ctx.Done():
					return
				}
			}
		})

		t.Run("NotifyAddressTransactions", func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			txs, err := client.NotifyAddressTransactions(ctx, testAddress)
			if err != nil {
				t.Error(err)
				return
			}
			for {
				select {
				case t := <-txs:
					log.Printf("%+v\n", t)
				case <-ctx.Done():
					return
				}
			}
		})

		t.Run("TransactionMerkle", func(t *testing.T) {
			m, err := client.TransactionMerkle("c011c74e1d0938003fbcd25ce8f60343766a5665da8a57412c50a9029b5f0056", 522232)
			if err != nil {
				t.Error(err)
				return
			}
			log.Printf("%v", m)
		})

		t.Run("UTXOAddress", func(t *testing.T) {
			_, err := client.UTXOAddress("4f73e43b92d337da8e69417601de1476bd7577cbac901fa28dba37ce1362adb9")
			if err != ErrDeprecatedMethod {
				t.Error(err)
			}
		})

		t.Run("BlockChunk", func(t *testing.T) {
			_, err := client.BlockChunk(7777)
			if err == nil {
				t.Error(errors.New("unexpected result"))
			}
		})
	})

	t.Run("Protocol_1.1", func(t *testing.T) {
		client, err := New(&Options{
			Address:   testServer,
			TLS:       &tls.Config{InsecureSkipVerify: true},
			Protocol:  Protocol11,
			KeepAlive: true,
		})
		if err != nil {
			t.Error(err)
			return
		}
		defer client.Close()

		t.Run("ServerPing", func(t *testing.T) {
			err := client.ServerPing()
			if err != ErrUnavailableMethod {
				t.Error(err)
				return
			}
			log.Println("Pong")
		})

		t.Run("ServerVersion", func(t *testing.T) {
			res, err := client.ServerVersion()
			if err != nil {
				t.Error(err)
				return
			}
			log.Printf("Server Software: %s\n", res.Software)
			log.Printf("Server Protocol: %s\n", res.Protocol)
		})

		t.Run("ServerBanner", func(t *testing.T) {
			res, err := client.ServerBanner()
			if err != nil {
				t.Error(err)
				return
			}
			log.Printf("Server Banner: %s\n", res)
		})

		t.Run("ServerDonationAddress", func(t *testing.T) {
			res, err := client.ServerDonationAddress()
			if err != nil {
				t.Error(err)
				return
			}
			log.Printf("Server Donation Address: %s\n", res)
		})

		t.Run("ServerFeatures", func(t *testing.T) {
			res, err := client.ServerFeatures()
			if err != nil {
				t.Error(err)
				return
			}
			log.Printf("Genesis Hash: %s", res.GenesisHash)
			log.Printf("Hash Function: %s", res.HashFunction)
			log.Printf("Max Protocol: %s", res.ProtocolMax)
			log.Printf("Min Protocol: %s", res.ProtocolMin)
		})

		t.Run("ServerPeers", func(t *testing.T) {
			peers, err := client.ServerPeers()
			if err != nil {
				t.Error(err)
				return
			}
			log.Println("Server peers")
			for _, p := range peers {
				log.Printf("%s > %s (%v)", p.Name, p.Address, p.Features)
			}
		})

		t.Run("AddressBalance", func(t *testing.T) {
			balance, err := client.AddressBalance(testAddress)
			if err != nil {
				t.Error(err)
				return
			}
			log.Printf("Balance: %+v\n", balance)
		})

		t.Run("AddressMempool", func(t *testing.T) {
			mempool, err := client.AddressMempool(testAddress)
			if err != nil {
				t.Error(err)
				return
			}
			log.Printf("Mempool: %+v\n", mempool)
		})

		t.Run("AddressHistory", func(t *testing.T) {
			history, err := client.AddressHistory(testAddress)
			if err != nil {
				t.Error(err)
				return
			}
			log.Printf("History: %+v\n", history)
		})

		t.Run("AddressListUnspent", func(t *testing.T) {
			utxo, err := client.AddressListUnspent(testAddress)
			if err != nil {
				t.Error(err)
				return
			}
			log.Printf("Unspent: %+v\n", utxo)
		})

		t.Run("BlockHeader", func(t *testing.T) {
			header, err := client.BlockHeader(56770)
			if err != nil {
				t.Error(err)
				return
			}
			log.Printf("Header: %+v\n", header)
		})

		t.Run("BroadcastTransaction", func(t *testing.T) {
			res, err := client.BroadcastTransaction("01000000010000000000000000000000000000000000000000000000000000000000000000ffffffff0702621b03cfd201ffffffff010000000000000000016a00000000")
			if err == nil {
				t.Error(errors.New("unexpected result"))
				return
			}
			log.Printf("%+v\n", res)
		})

		t.Run("GetTransaction", func(t *testing.T) {
			res, err := client.GetTransaction("4f73e43b92d337da8e69417601de1476bd7577cbac901fa28dba37ce1362adb9")
			if err != nil {
				t.Error(err)
				return
			}
			log.Printf("Tx: %+v\n", res)
		})

		t.Run("EstimateFee", func(t *testing.T) {
			fee, err := client.EstimateFee(6)
			if err != nil {
				t.Error(err)
				return
			}
			log.Printf("Fee: %+v\n", fee)
		})

		t.Run("NotifyBlockNums", func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			nums, err := client.NotifyBlockNums(ctx)
			if err != ErrDeprecatedMethod {
				t.Error(err)
				return
			}
			for {
				select {
				case n := <-nums:
					log.Printf("%+v\n", n)
				case <-ctx.Done():
					return
				}
			}
		})

		t.Run("NotifyBlockHeaders", func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10 * time.Second)
			defer cancel()
			headers, err := client.NotifyBlockHeaders(ctx)
			if err != nil {
				t.Error(err)
				return
			}
			for {
				select {
				case h := <-headers:
					log.Printf("%+v\n", h)
				case <-ctx.Done():
					return
				}
			}
		})

		t.Run("NotifyAddressTransactions", func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			txs, err := client.NotifyAddressTransactions(ctx, testAddress)
			if err != nil {
				t.Error(err)
				return
			}
			for {
				select {
				case t := <-txs:
					log.Printf("%+v\n", t)
				case <-ctx.Done():
					return
				}
			}
		})

		t.Run("TransactionMerkle", func(t *testing.T) {
			m, err := client.TransactionMerkle("c011c74e1d0938003fbcd25ce8f60343766a5665da8a57412c50a9029b5f0056", 522232)
			if err != nil {
				t.Error(err)
				return
			}
			log.Printf("%v", m)
		})

		t.Run("UTXOAddress", func(t *testing.T) {
			_, err := client.UTXOAddress("4f73e43b92d337da8e69417601de1476bd7577cbac901fa28dba37ce1362adb9")
			if err != ErrDeprecatedMethod {
				t.Error(err)
			}
		})

		t.Run("BlockChunk", func(t *testing.T) {
			_, err := client.BlockChunk(7777)
			if err == nil {
				t.Error(errors.New("unexpected result"))
			}
		})
	})

	t.Run("Protocol_1.2", func(t *testing.T) {
		client, err := New(&Options{
			Address:   testServer,
			TLS:       &tls.Config{InsecureSkipVerify: true},
			Protocol:  Protocol12,
			KeepAlive: true,
		})
		if err != nil {
			t.Error(err)
			return
		}
		defer client.Close()

		t.Run("ServerPing", func(t *testing.T) {
			err := client.ServerPing()
			if !strings.Contains(err.Error(), "unknown method") {
				t.Error(err)
				return
			}
			log.Println("Pong")
		})

		t.Run("ServerVersion", func(t *testing.T) {
			res, err := client.ServerVersion()
			if err != nil {
				t.Error(err)
				return
			}
			log.Printf("Server Software: %s\n", res.Software)
			log.Printf("Server Protocol: %s\n", res.Protocol)
		})

		t.Run("ServerBanner", func(t *testing.T) {
			res, err := client.ServerBanner()
			if err != nil {
				t.Error(err)
				return
			}
			log.Printf("Server Banner: %s\n", res)
		})

		t.Run("ServerDonationAddress", func(t *testing.T) {
			res, err := client.ServerDonationAddress()
			if err != nil {
				t.Error(err)
				return
			}
			log.Printf("Server Donation Address: %s\n", res)
		})

		t.Run("ServerFeatures", func(t *testing.T) {
			res, err := client.ServerFeatures()
			if err != nil {
				t.Error(err)
				return
			}
			log.Printf("Genesis Hash: %s", res.GenesisHash)
			log.Printf("Hash Function: %s", res.HashFunction)
			log.Printf("Max Protocol: %s", res.ProtocolMax)
			log.Printf("Min Protocol: %s", res.ProtocolMin)
		})

		t.Run("ServerPeers", func(t *testing.T) {
			peers, err := client.ServerPeers()
			if err != nil {
				t.Error(err)
				return
			}
			log.Println("Server peers")
			for _, p := range peers {
				log.Printf("%s > %s (%v)", p.Name, p.Address, p.Features)
			}
		})

		t.Run("AddressBalance", func(t *testing.T) {
			balance, err := client.AddressBalance(testAddress)
			if err != nil {
				t.Error(err)
				return
			}
			log.Printf("Balance: %+v\n", balance)
		})

		t.Run("AddressMempool", func(t *testing.T) {
			mempool, err := client.AddressMempool(testAddress)
			if err != nil {
				t.Error(err)
				return
			}
			log.Printf("Mempool: %+v\n", mempool)
		})

		t.Run("AddressHistory", func(t *testing.T) {
			history, err := client.AddressHistory(testAddress)
			if err != nil {
				t.Error(err)
				return
			}
			log.Printf("History: %+v\n", history)
		})

		t.Run("AddressListUnspent", func(t *testing.T) {
			utxo, err := client.AddressListUnspent(testAddress)
			if err != nil {
				t.Error(err)
				return
			}
			log.Printf("Unspent: %+v\n", utxo)
		})

		t.Run("BlockHeader", func(t *testing.T) {
			header, err := client.BlockHeader(56770)
			if err != nil {
				t.Error(err)
				return
			}
			log.Printf("Header: %+v\n", header)
		})

		t.Run("BroadcastTransaction", func(t *testing.T) {
			res, err := client.BroadcastTransaction("01000000010000000000000000000000000000000000000000000000000000000000000000ffffffff0702621b03cfd201ffffffff010000000000000000016a00000000")
			if err == nil {
				t.Error(errors.New("unexpected result"))
				return
			}
			log.Printf("%+v\n", res)
		})

		t.Run("GetTransaction", func(t *testing.T) {
			res, err := client.GetTransaction("4f73e43b92d337da8e69417601de1476bd7577cbac901fa28dba37ce1362adb9")
			if err != nil {
				t.Error(err)
				return
			}
			log.Printf("Tx: %+v\n", res)
		})

		t.Run("EstimateFee", func(t *testing.T) {
			fee, err := client.EstimateFee(6)
			if err != nil {
				t.Error(err)
				return
			}
			log.Printf("Fee: %+v\n", fee)
		})

		t.Run("NotifyBlockHeaders", func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10 * time.Second)
			defer cancel()
			headers, err := client.NotifyBlockHeaders(ctx)
			if err != nil {
				t.Error(err)
				return
			}
			for {
				select {
				case h := <-headers:
					log.Printf("%+v\n", h)
				case <-ctx.Done():
					return
				}
			}
		})

		t.Run("NotifyAddressTransactions", func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			txs, err := client.NotifyAddressTransactions(ctx, testAddress)
			if err != nil {
				t.Error(err)
				return
			}
			for {
				select {
				case t := <-txs:
					log.Printf("%+v\n", t)
				case <-ctx.Done():
					return
				}
			}
		})

		t.Run("TransactionMerkle", func(t *testing.T) {
			m, err := client.TransactionMerkle("c011c74e1d0938003fbcd25ce8f60343766a5665da8a57412c50a9029b5f0056", 522232)
			if err != nil {
				t.Error(err)
				return
			}
			log.Printf("%v", m)
		})

		// Deprecated methods

		t.Run("UTXOAddress", func(t *testing.T) {
			_, err := client.UTXOAddress("4f73e43b92d337da8e69417601de1476bd7577cbac901fa28dba37ce1362adb9")
			if err != ErrDeprecatedMethod {
				t.Error(err)
			}
		})

		t.Run("BlockChunk", func(t *testing.T) {
			_, err := client.BlockChunk(7777)
			if err != ErrDeprecatedMethod {
				t.Error(errors.New("unexpected result"))
			}
		})

		t.Run("NotifyBlockNums", func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			nums, err := client.NotifyBlockNums(ctx)
			if err != ErrDeprecatedMethod {
				t.Error(err)
				return
			}
			for {
				select {
				case n := <-nums:
					log.Printf("%+v\n", n)
				case <-ctx.Done():
					return
				}
			}
		})
	})

	return
}

func ExampleClient_ServerVersion() {
	client, _ := New(&Options{
		Address: "electrum.villocq.com:50002",
		TLS:     &tls.Config{
			InsecureSkipVerify: true,
		},
	})
	defer client.Close()
	version, _ := client.ServerVersion()
	fmt.Println(version.Software)
	// Output: ElectrumX 1.4.3
}

func ExampleClient_ServerDonationAddress() {
	client, _ := New(&Options{
		Address: "electrum.villocq.com:50002",
		TLS:     &tls.Config{
			InsecureSkipVerify: true,
		},
	})
	defer client.Close()
	addr, _ := client.ServerDonationAddress()
	fmt.Println(addr)
	// Output: bc1q3jc48stsmulrvsyulpgyekfggfapxrpc3ertgk
}