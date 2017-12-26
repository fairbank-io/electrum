package electrum

import (
	"bufio"
	"crypto/tls"
	"net"
)

type transport struct {
	conn     net.Conn
	messages chan []byte
	errors   chan error
	done     chan bool
	w        *bufio.Writer
	r        *bufio.Reader
}

type transportOptions struct {
	address string
	tls     *tls.Config
}

// Initialize a proper underlying network connection that can be terminated
// by the provided context
func getTransport(opts *transportOptions) (*transport, error) {
	var conn net.Conn
	var err error

	if opts.tls != nil {
		conn, err = tls.Dial("tcp", opts.address, opts.tls)
		if err != nil {
			return nil, err
		}
	} else {
		conn, err = net.Dial("tcp", opts.address)
		if err != nil {
			return nil, err
		}
	}

	t := &transport{
		conn:     conn,
		messages: make(chan []byte),
		errors:   make(chan error),
		done:     make(chan bool),
		w:        bufio.NewWriter(conn),
		r:        bufio.NewReader(conn),
	}
	go t.listen()
	return t, nil
}

// Send raw bytes across the network
func (t *transport) sendMessage(message []byte) error {
	_, err := t.w.Write(message)
	if err == nil {
		t.w.Flush()
	}
	return err
}

// Finish execution and close network connection
func (t *transport) close() {
	close(t.done)
}

// Wait for new messages on the network connection until
// the instance is signaled to stop
func (t *transport) listen() {
	for {
		select {
		case <-t.done:
			t.conn.Close()
			return
		default:
			line, err := t.r.ReadBytes(delimiter)
			if err != nil {
				t.errors <- err
				break
			}
			t.messages <- line
		}
	}
}
