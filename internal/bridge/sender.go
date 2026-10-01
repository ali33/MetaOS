// Package bridge định tuyến frame giữa metaos-ws (qua stdin/stdout) và các kênh.
package bridge

import (
	"io"
	"sync"

	"github.com/ali33/MetaOS/internal/protocol"
)

type Sender struct {
	mu sync.Mutex
	w  io.Writer
}

func NewSender(w io.Writer) *Sender { return &Sender{w: w} }

func (s *Sender) SendText(m protocol.Message) error {
	b, err := m.Encode()
	if err != nil {
		return err
	}
	return s.write(protocol.Frame{Kind: protocol.KindText, Data: b})
}

func (s *Sender) SendBinary(ch string, p []byte) error {
	b, err := protocol.EncodeBinary(ch, p)
	if err != nil {
		return err
	}
	return s.write(protocol.Frame{Kind: protocol.KindBinary, Data: b})
}

func (s *Sender) write(f protocol.Frame) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return protocol.WritePipeFrame(s.w, f)
}
