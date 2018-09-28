package electrum

import (
	"bufio"
	"crypto/tls"
	"io"
	"net"
	"sync"
	"time"
)

// ConnectionState represents known connection state values
type ConnectionState string

// Connection state flags
const (
	Ready        ConnectionState = "READY"
	Disconnected ConnectionState = "DISCONNECTED"
	Reconnecting ConnectionState = "RECONNECTING"
	Reconnected  ConnectionState = "RECONNECTED"
	Closed       ConnectionState = "CLOSED"
)

type transport struct {
	conn     net.Conn
	messages chan []byte
	errors   chan error
	done     chan bool
	ready    bool
	opts     *transportOptions
	state    chan ConnectionState
	r        *bufio.Reader
	mu       sync.Mutex
}

type transportOptions struct {
	address string
	tls     *tls.Config
}

// Get network connection
func connect(opts *transportOptions) (net.Conn, error) {
	conn, err := net.Dial("tcp", opts.address)
	if err != nil {
		return nil, err
	}
	conn.(*net.TCPConn).SetKeepAlive(true)
	conn.(*net.TCPConn).SetKeepAlivePeriod(30 * time.Second)

	if opts.tls != nil {
		return tls.Client(conn, opts.tls), nil
	}
	return conn, nil
}

// Initialize a proper handler for the underlying network connection
func getTransport(opts *transportOptions) (*transport, error) {
	conn, err := connect(opts)
	if err != nil {
		return nil, err
	}

	t := &transport{
		done:     make(chan bool),
		messages: make(chan []byte),
		errors:   make(chan error),
		state:    make(chan ConnectionState),
		opts:     opts,
	}
	t.setup(conn)
	go t.listen()
	return t, nil
}

// Prepare transport instance for usage with a given network connection
func (t *transport) setup(conn net.Conn) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.conn = conn
	t.ready = true
	t.r = bufio.NewReader(t.conn)
}

// Attempt automatic reconnection
func (t *transport) reconnect() {
	t.conn.Close()
	t.mu.Lock()
	t.ready = false
	t.mu.Unlock()
	t.state <- Reconnecting

	// Future implementations could include support for a max number of retries
	// and dynamically increasing the interval
	rt := time.NewTicker(5 * time.Second)
	go func() {
		defer rt.Stop()
		for range rt.C {
			conn, err := connect(t.opts)
			if err == nil {
				t.setup(conn)
				t.state <- Reconnected
				break
			}
		}
		go t.listen()
	}()
}

// Send raw bytes across the network
func (t *transport) sendMessage(message []byte) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.ready {
		return ErrUnreachableHost
	}

	_, err := t.conn.Write(message)
	return err
}

// Finish execution and close network connection
func (t *transport) close() {
	close(t.done)
}

// Wait for new messages on the network connection until
// the instance is signaled to stop
func (t *transport) listen() {
	t.state <- Ready
LOOP:
	for {
		select {
		case <-t.done:
			t.conn.Close()
			t.state <- Closed
			break LOOP
		default:
			line, err := t.r.ReadBytes(delimiter)

			// Detect dropped connections
			if err == io.EOF {
				t.state <- Disconnected
				t.reconnect()
				break LOOP
			}

			if err != nil {
				t.errors <- err
				break
			}
			t.messages <- line
		}
	}
}
