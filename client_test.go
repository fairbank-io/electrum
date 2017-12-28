package electrum

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"testing"
	"time"
)

func TestClient(t *testing.T) {
	const testAddress = "1ErbiumBjW4ScHNhLCcNWK5fFsKFpsYpWb"
	const testServer = "erbium1.sytes.net:50002"
	
	client, err := New(&Options{
		Address:   testServer,
		TLS:       &tls.Config{InsecureSkipVerify: true},
		KeepAlive: true,
	})
	if err != nil {
		t.Error(err)
	}
	defer client.Close()

	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	go func() {
		for range ticker.C {
			log.Println("*")
		}
	}()

	t.Run("ServerVersion", func(t *testing.T) {
		res, err := client.ServerVersion()
		if err != nil {
			t.Error(err)
		}
		log.Printf("Server Version: %s\n", res)
	})

	t.Run("ServerBanner", func(t *testing.T) {
		res, err := client.ServerBanner()
		if err != nil {
			t.Error(err)
		}
		log.Printf("Server Banner: %s\n", res)
	})

	t.Run("ServerDonationAddress", func(t *testing.T) {
		res, err := client.ServerDonationAddress()
		if err != nil {
			t.Error(err)
		}
		log.Printf("Server Donation Address: %s\n", res)
	})

	t.Run("AddressBalance", func(t *testing.T) {
		balance, err := client.AddressBalance(testAddress)
		if err != nil {
			t.Error(err)
		}
		log.Printf("Balance: %+v\n", balance)
	})

	t.Run("AddressMempool", func(t *testing.T) {
		mempool, err := client.AddressMempool(testAddress)
		if err != nil {
			t.Error(err)
		}
		log.Printf("Mempool: %+v\n", mempool)
	})

	t.Run("AddressHistory", func(t *testing.T) {
		history, err := client.AddressHistory(testAddress)
		if err != nil {
			t.Error(err)
		}
		log.Printf("History: %+v\n", history)
	})

	t.Run("AddressListUnspent", func(t *testing.T) {
		utxo, err := client.AddressListUnspent(testAddress)
		if err != nil {
			t.Error(err)
		}
		log.Printf("Unspent: %+v\n", utxo)
	})

	t.Run("BlockHeader", func(t *testing.T) {
		header, err := client.BlockHeader(56770)
		if err != nil {
			t.Error(err)
		}
		log.Printf("Header: %+v\n", header)
	})

	t.Run("BroadcastTransaction", func(t *testing.T) {
		res, err := client.BroadcastTransaction("01000000010000000000000000000000000000000000000000000000000000000000000000ffffffff0702621b03cfd201ffffffff010000000000000000016a00000000")
		if err != nil {
			t.Error(err)
		}
		log.Printf("%+v\n", res)
	})

	t.Run("GetTransaction", func(t *testing.T) {
		res, err := client.GetTransaction("4f73e43b92d337da8e69417601de1476bd7577cbac901fa28dba37ce1362adb9")
		if err != nil {
			t.Error(err)
		}
		log.Printf("Tx: %+v\n", res)
	})

	t.Run("EstimateFee", func(t *testing.T) {
		fee, err := client.EstimateFee(6)
		if err != nil {
			t.Error(err)
		}
		log.Printf("Fee: %+v\n", fee)
	})

	t.Run("NotifyBlockNums", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		nums, err := client.NotifyBlockNums(ctx)
		if err != nil {
			t.Error(err)
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

	// Not implemented methods

	t.Run("AddressProof", func(t *testing.T) {
		_, err := client.AddressProof(testAddress)
		if err == nil {
			t.Error(errors.New("unexpected result"))
		}
	})

	t.Run("UTXOAddress", func(t *testing.T) {
		_, err := client.UTXOAddress("4f73e43b92d337da8e69417601de1476bd7577cbac901fa28dba37ce1362adb9")
		if err == nil {
			t.Error(errors.New("unexpected result"))
		}
	})

	t.Run("BlockChunk", func(t *testing.T) {
		_, err := client.BlockChunk(7777)
		if err == nil {
			t.Error(errors.New("unexpected result"))
		}
	})

	t.Run("TransactionMerkle", func(t *testing.T) {
		_, err := client.TransactionMerkle("4f73e43b92d337da8e69417601de1476bd7577cbac901fa28dba37ce1362adb9")
		if err == nil {
			t.Error(errors.New("unexpected result"))
		}
	})

	t.Run("NotifyServerPeers", func(t *testing.T) {
		_, err := client.NotifyServerPeers()
		if err == nil {
			t.Error(errors.New("unexpected result"))
		}
	})

	return
}

func ExampleClient_ServerVersion() {
	client, _ := New(&Options{
		Address: "erbium1.sytes.net:50002",
		TLS:       &tls.Config{InsecureSkipVerify: true},
	})
	defer client.Close()
	version, _ := client.ServerVersion()
	fmt.Println(version)
	// Output: ElectrumX 1.2.1
}

func ExampleClient_ServerDonationAddress() {
	client, _ := New(&Options{
		Address: "erbium1.sytes.net:50002",
		TLS:       &tls.Config{InsecureSkipVerify: true},
	})
	defer client.Close()
	addr, _ := client.ServerDonationAddress()
	fmt.Println(addr)
	// Output: 1ErbiumBjW4ScHNhLCcNWK5fFsKFpsYpWb
}