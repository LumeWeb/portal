package bao

import (
	"bufio"
	_ "embed"
	"io"
	"lukechampine.com/blake3"
)

const (
	chunkSize = 1024
)

func ComputeTree(reader io.Reader, size int64) ([]byte, [32]byte, error) {
	bufSize := baoOutboardSize(int(size))
	buf := bufferAt{buf: make([]byte, bufSize)}

	hash, err := blake3.BaoEncode(&buf, bufio.NewReader(reader), size, true)
	if err != nil {
		return nil, [32]byte{}, err
	}

	return buf.buf, hash, nil
}

func baoOutboardSize(dataLen int) int {
	if dataLen == 0 {
		return 8
	}
	chunks := (dataLen + chunkSize - 1) / chunkSize
	cvs := 2*chunks - 2 // no I will not elaborate
	return 8 + cvs*32
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
