package tunnel

import (
	"context"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/doguab/proxypool-agent/internal/config"
	"github.com/doguab/proxypool-agent/internal/limits"
	"github.com/doguab/proxypool-agent/internal/protocol"
	"github.com/gorilla/websocket"
)

type Client struct {
	cfg       config.Config
	conn      *websocket.Conn
	writeMu   sync.Mutex
	streams   map[uint32]net.Conn
	streamsMu sync.Mutex
	sem       *limits.Semaphore
}

func NewClient(cfg config.Config) *Client {
	return &Client{
		cfg:     cfg,
		streams: make(map[uint32]net.Conn),
		sem:     limits.New(cfg.MaxConnections),
	}
}

func (c *Client) Run(ctx context.Context) error {
	backoff := time.Second
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		err := c.connectAndServe(ctx)
		if err != nil {
			log.Printf("tunnel disconnected: %v; reconnecting in %s", err, backoff)
		}
		c.cleanupStreams()

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
}

func (c *Client) connectAndServe(ctx context.Context) error {
	dialer := websocket.Dialer{HandshakeTimeout: 15 * time.Second}
	conn, _, err := dialer.DialContext(ctx, c.cfg.HubURL, nil)
	if err != nil {
		return fmt.Errorf("dial hub: %w", err)
	}
	c.conn = conn
	defer func() {
		_ = conn.Close()
		c.conn = nil
	}()

	authPayload, _ := protocol.MarshalJSON(protocol.AuthPayload{Secret: c.cfg.Secret})
	if err := c.writeFrame(protocol.Frame{Type: protocol.TypeAuth, Payload: authPayload}); err != nil {
		return err
	}

	frame, err := c.readFrame()
	if err != nil {
		return fmt.Errorf("auth response: %w", err)
	}
	switch frame.Type {
	case protocol.TypeAuthOK:
		var ok protocol.AuthOKPayload
		if err := protocol.UnmarshalJSON(frame.Payload, &ok); err == nil && ok.ServerID != "" {
			log.Printf("authenticated as server %s", ok.ServerID)
		} else {
			log.Printf("authenticated with hub")
		}
	case protocol.TypeAuthFail:
		return fmt.Errorf("authentication rejected by hub")
	default:
		return fmt.Errorf("unexpected auth response type: %d", frame.Type)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- c.readLoop()
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errCh:
		return err
	}
}

func (c *Client) readLoop() error {
	for {
		frame, err := c.readFrame()
		if err != nil {
			return err
		}
		switch frame.Type {
		case protocol.TypePing:
			if err := c.writeFrame(protocol.Frame{Type: protocol.TypePong}); err != nil {
				return err
			}
		case protocol.TypeOpen:
			go c.handleOpen(frame)
		case protocol.TypeData:
			c.handleData(frame)
		case protocol.TypeClose:
			c.closeStream(frame.StreamID)
		default:
			log.Printf("ignored frame type %d", frame.Type)
		}
	}
}

func (c *Client) handleOpen(frame protocol.Frame) {
	var req protocol.OpenPayload
	if err := protocol.UnmarshalJSON(frame.Payload, &req); err != nil {
		_ = c.writeOpenFail(frame.StreamID, "invalid open payload")
		return
	}

	c.sem.Acquire()
	defer c.sem.Release()

	target := fmt.Sprintf("%s:%d", req.Host, req.Port)
	dialer := &net.Dialer{Timeout: 30 * time.Second}
	remote, err := dialer.Dial("tcp", target)
	if err != nil {
		_ = c.writeOpenFail(frame.StreamID, err.Error())
		return
	}

	c.streamsMu.Lock()
	c.streams[frame.StreamID] = remote
	c.streamsMu.Unlock()

	if err := c.writeFrame(protocol.Frame{Type: protocol.TypeOpenOK, StreamID: frame.StreamID}); err != nil {
		c.closeStream(frame.StreamID)
		return
	}

	go c.pumpRemoteToHub(frame.StreamID, remote)
}

func (c *Client) pumpRemoteToHub(streamID uint32, remote net.Conn) {
	defer c.closeStream(streamID)
	buf := make([]byte, 32*1024)
	for {
		n, err := remote.Read(buf)
		if n > 0 {
			if werr := c.writeFrame(protocol.Frame{
				Type:     protocol.TypeData,
				StreamID: streamID,
				Payload:  append([]byte(nil), buf[:n]...),
			}); werr != nil {
				return
			}
		}
		if err != nil {
			return
		}
	}
}

func (c *Client) handleData(frame protocol.Frame) {
	c.streamsMu.Lock()
	remote, ok := c.streams[frame.StreamID]
	c.streamsMu.Unlock()
	if !ok || len(frame.Payload) == 0 {
		return
	}
	if _, err := remote.Write(frame.Payload); err != nil {
		c.closeStream(frame.StreamID)
	}
}

func (c *Client) closeStream(streamID uint32) {
	c.streamsMu.Lock()
	remote, ok := c.streams[streamID]
	if ok {
		delete(c.streams, streamID)
	}
	c.streamsMu.Unlock()
	if ok {
		_ = remote.Close()
	}
	_ = c.writeFrame(protocol.Frame{Type: protocol.TypeClose, StreamID: streamID})
}

func (c *Client) writeOpenFail(streamID uint32, msg string) error {
	payload, _ := protocol.MarshalJSON(protocol.OpenFailPayload{Error: msg})
	return c.writeFrame(protocol.Frame{Type: protocol.TypeOpenFail, StreamID: streamID, Payload: payload})
}

func (c *Client) readFrame() (protocol.Frame, error) {
	if c.conn == nil {
		return protocol.Frame{}, fmt.Errorf("not connected")
	}
	_, data, err := c.conn.ReadMessage()
	if err != nil {
		return protocol.Frame{}, err
	}
	return protocol.DecodeFrame(data)
}

func (c *Client) writeFrame(frame protocol.Frame) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if c.conn == nil {
		return fmt.Errorf("not connected")
	}
	data, err := protocol.EncodeFrame(frame)
	if err != nil {
		return err
	}
	return c.conn.WriteMessage(websocket.BinaryMessage, data)
}

func (c *Client) cleanupStreams() {
	c.streamsMu.Lock()
	defer c.streamsMu.Unlock()
	for id, conn := range c.streams {
		_ = conn.Close()
		delete(c.streams, id)
	}
}
