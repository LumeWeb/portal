package bao

import (
	"bufio"
	_ "embed"
	"io"
	"lukechampine.com/blake3"
)

func ComputeTree(reader io.Reader, size int64) ([]byte, [32]byte, error) {
	bufSize := blake3.BaoEncodedSize(int(size), true)
	buf := bufferAt{buf: make([]byte, bufSize)}

	hash, err := blake3.BaoEncode(&buf, bufio.NewReader(reader), size, true)
	if err != nil {
		return nil, [32]byte{}, err
	}

	return buf.buf, hash, nil
}

type bufferAt struct {
	buf []byte
}

func (b *bufferAt) WriteAt(p []byte, off int64) (int, error) {
	if copy(b.buf[off:], p) != len(p) {
		panic("bad buffer size")
	}
	return len(p), nil
}
