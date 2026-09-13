// Package ipc speaks the Discord local RPC protocol: framed JSON over a
// Unix socket (Linux/macOS) or named pipe (Windows). It discovers the
// client socket itself, including sandboxed installs (Flatpak/Snap)
// whose sockets live outside the standard runtime dir.
package ipc

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"time"
)

// IPC opcodes.
const (
	opHandshake = 0
	opFrame     = 1
	opClose     = 2
	opPing      = 3
	opPong      = 4
)

const ioTimeout = 3 * time.Second

// Client is one Discord IPC connection. Safe for concurrent use.
type Client struct {
	mu   chan struct{} // binary semaphore: one frame at a time
	conn net.Conn
}

// Handshake payload.
type handshake struct {
	V        int    `json:"v"`
	ClientID string `json:"client_id"`
}

// frame is a decoded incoming frame.
type frame struct {
	Opcode int32
	Data   json.RawMessage
}

// Dial connects to addr and completes the handshake for clientID.
func Dial(addr, clientID string, timeout time.Duration) (*Client, error) {
	conn, err := dialAddr(addr, timeout)
	if err != nil {
		return nil, err
	}
	c := &Client{mu: make(chan struct{}, 1), conn: conn}
	c.mu <- struct{}{}
	if err := c.roundTrip(opHandshake, handshake{V: 1, ClientID: clientID}); err != nil {
		conn.Close()
		return nil, err
	}
	return c, nil
}

// SetActivity sends a SET_ACTIVITY frame. activity is marshaled as the
// "activity" object of the args.
func (c *Client) SetActivity(cmd string, args map[string]any) error {
	return c.roundTrip(opFrame, map[string]any{"cmd": cmd, "args": args, "nonce": nonce()})
}

// Close shuts the connection down.
func (c *Client) Close() error {
	<-c.mu
	defer func() { c.mu <- struct{}{} }()
	return c.conn.Close()
}

func (c *Client) roundTrip(opcode int32, payload any) error {
	<-c.mu
	defer func() { c.mu <- struct{}{} }()

	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	_ = binary.Write(&buf, binary.LittleEndian, opcode)
	_ = binary.Write(&buf, binary.LittleEndian, int32(len(raw)))
	buf.Write(raw)

	_ = c.conn.SetDeadline(time.Now().Add(ioTimeout))
	if _, err := c.conn.Write(buf.Bytes()); err != nil {
		return err
	}
	f, err := readFrame(c.conn)
	if err != nil {
		return err
	}
	if f.Opcode == opClose {
		return fmt.Errorf("discord closed the connection")
	}
	// Surface Discord-side errors instead of silently pretending.
	var evt struct {
		Evt  string `json:"evt"`
		Data struct {
			Error struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		} `json:"data"`
	}
	if json.Unmarshal(f.Data, &evt) == nil && evt.Evt == "ERROR" {
		return fmt.Errorf("discord: %s (%d)", evt.Data.Error.Message, evt.Data.Error.Code)
	}
	return nil
}

func readFrame(r io.Reader) (frame, error) {
	var f frame
	var hdr [8]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return f, err
	}
	f.Opcode = int32(binary.LittleEndian.Uint32(hdr[0:4]))
	n := int(binary.LittleEndian.Uint32(hdr[4:8]))
	if n < 0 || n > 1<<20 {
		return f, fmt.Errorf("discord: bogus frame size %d", n)
	}
	data := make([]byte, n)
	if _, err := io.ReadFull(r, data); err != nil {
		return f, err
	}
	f.Data = data
	return f, nil
}

func nonce() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
