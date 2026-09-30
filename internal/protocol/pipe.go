package protocol

import (
	"encoding/binary"
	"errors"
	"io"
)

const (
	KindText       byte = 0
	KindBinary     byte = 1
	MaxFrame            = 1 << 20
	MaxUploadChunk      = 256 << 10
	MaxChannels         = 64
)

var (
	ErrFrameTooLarge = errors.New("protocol: frame too large")
	ErrBadKind       = errors.New("protocol: bad frame kind")
)

type Frame struct {
	Kind byte
	Data []byte
}

// WritePipeFrame ghi một frame bằng đúng một lời gọi Write, để người gọi chỉ
// cần một khoá quanh nó là các frame không xen vào nhau.
func WritePipeFrame(w io.Writer, f Frame) error {
	if f.Kind != KindText && f.Kind != KindBinary {
		return ErrBadKind
	}
	if len(f.Data) > MaxFrame {
		return ErrFrameTooLarge
	}
	buf := make([]byte, 5+len(f.Data))
	binary.BigEndian.PutUint32(buf, uint32(len(f.Data)))
	buf[4] = f.Kind
	copy(buf[5:], f.Data)
	_, err := w.Write(buf)
	return err
}

func ReadPipeFrame(r io.Reader) (Frame, error) {
	var hdr [5]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return Frame{}, err // io.EOF nếu hết sạch, io.ErrUnexpectedEOF nếu cụt
	}
	n := binary.BigEndian.Uint32(hdr[:4])
	if n > MaxFrame {
		return Frame{}, ErrFrameTooLarge
	}
	if hdr[4] != KindText && hdr[4] != KindBinary {
		return Frame{}, ErrBadKind
	}
	data := make([]byte, n)
	if _, err := io.ReadFull(r, data); err != nil {
		if err == io.EOF {
			err = io.ErrUnexpectedEOF
		}
		return Frame{}, err
	}
	return Frame{Kind: hdr[4], Data: data}, nil
}
