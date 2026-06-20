package protocol

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
)

const (
	TypeAuth     byte = 0x01
	TypeAuthOK   byte = 0x02
	TypeAuthFail byte = 0x03
	TypePing     byte = 0x04
	TypePong     byte = 0x05
	TypeOpen     byte = 0x10
	TypeOpenOK   byte = 0x11
	TypeOpenFail byte = 0x12
	TypeData     byte = 0x20
	TypeClose    byte = 0x21
)

const MaxPayload = 64 * 1024

type Frame struct {
	Type     byte
	StreamID uint32
	Payload  []byte
}

type AuthPayload struct {
	Secret string `json:"secret"`
}

type AuthOKPayload struct {
	ServerID string `json:"server_id"`
}

type OpenPayload struct {
	Host string `json:"host"`
	Port uint16 `json:"port"`
}

type OpenFailPayload struct {
	Error string `json:"error"`
}

func WriteFrame(w io.Writer, f Frame) error {
	header := make([]byte, 9)
	header[0] = f.Type
	binary.BigEndian.PutUint32(header[1:5], f.StreamID)
	if len(f.Payload) > MaxPayload {
		return fmt.Errorf("payload too large: %d", len(f.Payload))
	}
	binary.BigEndian.PutUint32(header[5:9], uint32(len(f.Payload)))
	if _, err := w.Write(header); err != nil {
		return err
	}
	if len(f.Payload) == 0 {
		return nil
	}
	_, err := w.Write(f.Payload)
	return err
}

func ReadFrame(r io.Reader) (Frame, error) {
	header := make([]byte, 9)
	if _, err := io.ReadFull(r, header); err != nil {
		return Frame{}, err
	}
	length := binary.BigEndian.Uint32(header[5:9])
	if length > MaxPayload {
		return Frame{}, fmt.Errorf("frame payload too large: %d", length)
	}
	payload := make([]byte, length)
	if length > 0 {
		if _, err := io.ReadFull(r, payload); err != nil {
			return Frame{}, err
		}
	}
	return Frame{
		Type:     header[0],
		StreamID: binary.BigEndian.Uint32(header[1:5]),
		Payload:  payload,
	}, nil
}

func MarshalJSON(v any) ([]byte, error) {
	return json.Marshal(v)
}

func UnmarshalJSON(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

func EncodeFrame(f Frame) ([]byte, error) {
	buf := make([]byte, 9+len(f.Payload))
	buf[0] = f.Type
	binary.BigEndian.PutUint32(buf[1:5], f.StreamID)
	if len(f.Payload) > MaxPayload {
		return nil, fmt.Errorf("payload too large: %d", len(f.Payload))
	}
	binary.BigEndian.PutUint32(buf[5:9], uint32(len(f.Payload)))
	copy(buf[9:], f.Payload)
	return buf, nil
}

func DecodeFrame(data []byte) (Frame, error) {
	if len(data) < 9 {
		return Frame{}, fmt.Errorf("frame too short")
	}
	length := binary.BigEndian.Uint32(data[5:9])
	if length > MaxPayload {
		return Frame{}, fmt.Errorf("frame payload too large: %d", length)
	}
	if int(9+length) > len(data) {
		return Frame{}, fmt.Errorf("incomplete frame")
	}
	payload := make([]byte, length)
	copy(payload, data[9:9+length])
	return Frame{
		Type:     data[0],
		StreamID: binary.BigEndian.Uint32(data[1:5]),
		Payload:  payload,
	}, nil
}
