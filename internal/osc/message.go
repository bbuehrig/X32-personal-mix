package osc

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"
)

// Message is a minimal OSC 1.0 message.
type Message struct {
	Address   string
	Arguments []any
}

// Marshal encodes the message into an OSC packet.
func (m Message) Marshal() ([]byte, error) {
	var buf bytes.Buffer
	writePaddedString(&buf, m.Address)

	typeTag := ","
	for _, arg := range m.Arguments {
		switch arg.(type) {
		case int32:
			typeTag += "i"
		case float32:
			typeTag += "f"
		case string:
			typeTag += "s"
		case []byte:
			typeTag += "b"
		default:
			return nil, fmt.Errorf("unsupported OSC argument type %T", arg)
		}
	}
	writePaddedString(&buf, typeTag)

	for _, arg := range m.Arguments {
		switch v := arg.(type) {
		case int32:
			if err := binary.Write(&buf, binary.BigEndian, v); err != nil {
				return nil, err
			}
		case float32:
			if err := binary.Write(&buf, binary.BigEndian, v); err != nil {
				return nil, err
			}
		case string:
			writePaddedString(&buf, v)
		case []byte:
			if err := binary.Write(&buf, binary.BigEndian, int32(len(v))); err != nil {
				return nil, err
			}
			buf.Write(v)
			pad := (4 - (len(v) % 4)) % 4
			buf.Write(make([]byte, pad))
		}
	}

	return buf.Bytes(), nil
}

// Unmarshal parses an OSC message packet.
func Unmarshal(data []byte) (Message, error) {
	var msg Message
	r := bytes.NewReader(data)

	address, err := readPaddedString(r)
	if err != nil {
		return msg, fmt.Errorf("read address: %w", err)
	}
	msg.Address = address

	typeTag, err := readPaddedString(r)
	if err != nil {
		return msg, fmt.Errorf("read type tag: %w", err)
	}
	if !strings.HasPrefix(typeTag, ",") {
		return msg, fmt.Errorf("invalid type tag %q", typeTag)
	}

	for _, tag := range typeTag[1:] {
		switch tag {
		case 'i':
			var v int32
			if err := binary.Read(r, binary.BigEndian, &v); err != nil {
				return msg, err
			}
			msg.Arguments = append(msg.Arguments, v)
		case 'f':
			var v float32
			if err := binary.Read(r, binary.BigEndian, &v); err != nil {
				return msg, err
			}
			msg.Arguments = append(msg.Arguments, v)
		case 's':
			s, err := readPaddedString(r)
			if err != nil {
				return msg, err
			}
			msg.Arguments = append(msg.Arguments, s)
		case 'b':
			var size int32
			if err := binary.Read(r, binary.BigEndian, &size); err != nil {
				return msg, err
			}
			if size < 0 {
				return msg, fmt.Errorf("negative blob size")
			}
			blob := make([]byte, size)
			if _, err := r.Read(blob); err != nil {
				return msg, err
			}
			pad := (4 - (int(size) % 4)) % 4
			if pad > 0 {
				skip := make([]byte, pad)
				if _, err := r.Read(skip); err != nil {
					return msg, err
				}
			}
			msg.Arguments = append(msg.Arguments, blob)
		default:
			return msg, fmt.Errorf("unsupported OSC type tag %q", tag)
		}
	}

	return msg, nil
}

func writePaddedString(buf *bytes.Buffer, s string) {
	buf.WriteString(s)
	buf.WriteByte(0)
	pad := (4 - ((len(s) + 1) % 4)) % 4
	buf.Write(make([]byte, pad))
}

func readPaddedString(r *bytes.Reader) (string, error) {
	var b strings.Builder
	for {
		c, err := r.ReadByte()
		if err != nil {
			return "", err
		}
		if c == 0 {
			break
		}
		b.WriteByte(c)
	}
	// consume padding to 4-byte boundary (null already counted)
	n := b.Len() + 1
	pad := (4 - (n % 4)) % 4
	if pad > 0 {
		skip := make([]byte, pad)
		if _, err := r.Read(skip); err != nil {
			return "", err
		}
	}
	return b.String(), nil
}
