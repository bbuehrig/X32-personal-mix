package osc

import (
	"fmt"
	"net"
	"sync"
	"time"
)

const DefaultPort = 10023

// Collected is one datagram gathered by Collect.
type Collected struct {
	Msg  Message
	From *net.UDPAddr
}

// Client sends and receives OSC over a single UDP socket with a background reader.
type Client struct {
	conn *net.UDPConn

	sendMu sync.Mutex

	pendMu  sync.Mutex
	pending map[string]chan Message

	updateMu sync.Mutex
	onUpdate func(Message)

	collectMu   sync.Mutex
	collecting  bool
	collectCh    chan Collected
	collectMatch func(Message) bool

	closed chan struct{}
	done   sync.WaitGroup
}

// NewClient binds a UDP socket and starts the read loop.
func NewClient() (*Client, error) {
	addr := &net.UDPAddr{IP: net.IPv4zero, Port: 0}
	conn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		return nil, fmt.Errorf("bind udp: %w", err)
	}
	if err := conn.SetReadBuffer(1 << 20); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("set read buffer: %w", err)
	}
	c := &Client{
		conn:    conn,
		pending: make(map[string]chan Message),
		closed:  make(chan struct{}),
	}
	c.done.Add(1)
	go c.readLoop()
	return c, nil
}

// OnUpdate registers a handler for unsolicited OSC messages (e.g. /xremote).
func (c *Client) OnUpdate(fn func(Message)) {
	c.updateMu.Lock()
	c.onUpdate = fn
	c.updateMu.Unlock()
}

// Close releases the UDP socket and stops the reader.
func (c *Client) Close() error {
	select {
	case <-c.closed:
		return nil
	default:
		close(c.closed)
	}
	c.sendMu.Lock()
	conn := c.conn
	c.conn = nil
	c.sendMu.Unlock()
	if conn != nil {
		_ = conn.Close()
	}
	c.done.Wait()
	return nil
}

// Send writes an OSC message to dest.
func (c *Client) Send(dest *net.UDPAddr, msg Message) error {
	packet, err := msg.Marshal()
	if err != nil {
		return err
	}
	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	if c.conn == nil {
		return fmt.Errorf("client closed")
	}
	_, err = c.conn.WriteToUDP(packet, dest)
	return err
}

// EnableBroadcast turns on SO_BROADCAST.
func (c *Client) EnableBroadcast() error {
	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	if c.conn == nil {
		return fmt.Errorf("client closed")
	}
	raw, err := c.conn.SyscallConn()
	if err != nil {
		return err
	}
	var setErr error
	controlErr := raw.Control(func(fd uintptr) {
		setErr = setBroadcast(fd)
	})
	if controlErr != nil {
		return controlErr
	}
	return setErr
}

// SendMsg sends an OSC message without waiting for a reply.
func (c *Client) SendMsg(dest *net.UDPAddr, msg Message) error {
	return c.Send(dest, msg)
}

// Request sends an OSC address query and waits for a matching reply.
func (c *Client) Request(dest *net.UDPAddr, address string, timeout time.Duration) (Message, error) {
	ch := make(chan Message, 1)
	c.pendMu.Lock()
	if old, ok := c.pending[address]; ok {
		close(old)
	}
	c.pending[address] = ch
	c.pendMu.Unlock()

	defer func() {
		c.pendMu.Lock()
		if c.pending[address] == ch {
			delete(c.pending, address)
		}
		c.pendMu.Unlock()
	}()

	if err := c.Send(dest, Message{Address: address}); err != nil {
		return Message{}, err
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case msg, ok := <-ch:
		if !ok {
			return Message{}, fmt.Errorf("request cancelled for %s", address)
		}
		return msg, nil
	case <-timer.C:
		return Message{}, fmt.Errorf("timeout waiting for %s", address)
	case <-c.closed:
		return Message{}, fmt.Errorf("client closed")
	}
}

// Collect runs setup, then gathers matching messages until timeout.
func (c *Client) Collect(timeout time.Duration, match func(Message) bool, setup func() error) ([]Collected, error) {
	ch := make(chan Collected, 64)
	c.collectMu.Lock()
	c.collecting = true
	c.collectMatch = match
	c.collectCh = ch
	c.collectMu.Unlock()

	defer func() {
		c.collectMu.Lock()
		c.collecting = false
		c.collectMatch = nil
		c.collectCh = nil
		c.collectMu.Unlock()
	}()

	if err := setup(); err != nil {
		return nil, err
	}

	deadline := time.After(timeout)
	var out []Collected
	for {
		select {
		case item := <-ch:
			out = append(out, item)
		case <-deadline:
			return out, nil
		case <-c.closed:
			return out, fmt.Errorf("client closed")
		}
	}
}

func (c *Client) readLoop() {
	defer c.done.Done()
	buf := make([]byte, 65535)
	for {
		c.sendMu.Lock()
		conn := c.conn
		c.sendMu.Unlock()
		if conn == nil {
			return
		}
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, from, err := conn.ReadFromUDP(buf)
		if err != nil {
			select {
			case <-c.closed:
				return
			default:
			}
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			continue
		}
		msg, err := Unmarshal(buf[:n])
		if err != nil {
			continue
		}
		c.dispatch(msg, from)
	}
}

func (c *Client) dispatch(msg Message, from *net.UDPAddr) {
	c.collectMu.Lock()
	collecting := c.collecting && c.collectMatch != nil && c.collectMatch(msg)
	collectCh := c.collectCh
	c.collectMu.Unlock()
	if collecting {
		select {
		case collectCh <- Collected{Msg: msg, From: from}:
		default:
		}
		// still fall through to pending/onUpdate so live sync is not starved
	}

	c.pendMu.Lock()
	if ch, ok := c.pending[msg.Address]; ok {
		delete(c.pending, msg.Address)
		c.pendMu.Unlock()
		select {
		case ch <- msg:
		default:
		}
	} else {
		c.pendMu.Unlock()
	}

	// Always notify subscribers (xremote). Pending replies used to swallow
	// these, which could delay the first live fader update.
	c.updateMu.Lock()
	fn := c.onUpdate
	c.updateMu.Unlock()
	if fn != nil {
		fn(msg)
	}
}
