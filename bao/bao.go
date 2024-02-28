package bao

import (
	"bufio"
	"bytes"
	_ "embed"
	"errors"
	"io"
	"math"
	"os"
	"os/exec"

	"github.com/docker/go-units"
	"github.com/hashicorp/go-plugin"
)

//go:generate buf generate
//go:generate bash -c "cd rust && cargo build -r"
//go:embed rust/target/release/rust
var pluginBin []byte

var bao Bao
var client *plugin.Client

var _ io.ReadCloser = (*Verifier)(nil)

var ErrVerifyFailed = errors.New("verification failed")

type Verifier struct {
	r      io.ReadCloser
	proof  Result
	read   uint64
	buffer *bytes.Buffer
}

func (v *Verifier) Read(p []byte) (int, error) {
	// Initial attempt to read from the buffer
	n, err := v.buffer.Read(p)
	if n == len(p) {
		// If the buffer already had enough data to fulfill the request, return immediately
		return n, nil
	} else if err != nil && err != io.EOF {
		// For errors other than EOF, return the error immediately
		return n, err
	}

	buf := make([]byte, VERIFY_CHUNK_SIZE)
	// Continue reading from the source and verifying until we have enough data or hit an error
	for v.buffer.Len() < len(p)-n {
		bytesRead, err := io.ReadFull(v.r, buf)
		if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
			return n, err // Return any read error immediately
		}

		if !bao.Verify(buf[:bytesRead], v.read, v.proof.Proof, v.proof.Hash) {
			return n, ErrVerifyFailed
		}

		v.read += uint64(bytesRead)
		v.buffer.Write(buf[:bytesRead]) // Append new data to the buffer

		if err == io.EOF {
			// If EOF, break the loop as no more data can be read
			break
		}
	}

	// Attempt to read the remainder of the data from the buffer
	additionalBytes, _ := v.buffer.Read(p[n:])
	return n + additionalBytes, nil
}

func (v *Verifier) Close() error {
	return v.r.Close()
}

func init() {
	temp, err := os.CreateTemp(os.TempDir(), "bao")
	if err != nil {
		panic(err)
	}

	err = temp.Chmod(1755)
	if err != nil {
		panic(err)
	}

	_, err = io.Copy(temp, bytes.NewReader(pluginBin))
	if err != nil {
		panic(err)
	}

	err = temp.Close()
	if err != nil {
		panic(err)
	}

	clientInst := plugin.NewClient(&plugin.ClientConfig{
		HandshakeConfig: plugin.HandshakeConfig{
			ProtocolVersion: 1,
		},
		Plugins: plugin.PluginSet{
			"bao": &BaoPlugin{},
		},
		Cmd:              exec.Command(temp.Name()),
		AllowedProtocols: []plugin.Protocol{plugin.ProtocolGRPC},
	})

	rpcClient, err := clientInst.Client()
	if err != nil {
		panic(err)
	}

	pluginInst, err := rpcClient.Dispense("bao")
	if err != nil {
		panic(err)
	}

	bao = pluginInst.(Bao)
}

func Shutdown() {
	client.Kill()
}

func Hash(r io.Reader) (*Result, error) {
	hasherId := bao.NewHasher()
	initialSize := 4 * units.KiB
	maxSize := 3.5 * units.MiB
	bufSize := initialSize

	reader := bufio.NewReaderSize(r, bufSize)
	var totalReadSize int

	buf := make([]byte, bufSize)
	for {

		n, err := reader.Read(buf)
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		totalReadSize += n

		if !bao.Hash(hasherId, buf[:n]) {
			return nil, errors.New("hashing failed")
		}

		// Adaptively adjust buffer size based on read patterns
		if n == bufSize && float64(bufSize) < maxSize {
			// If buffer was fully used, consider increasing buffer size
			bufSize = int(math.Min(float64(bufSize*2), float64(maxSize))) // Double the buffer size, up to a maximum
			buf = make([]byte, bufSize)                                   // Apply new buffer size
			reader = bufio.NewReaderSize(r, bufSize)                      // Apply new buffer size
		}
	}

	result := bao.Finish(hasherId)
	result.Length = uint(totalReadSize)

	return &result, nil
}

func NewVerifier(r io.ReadCloser, proof Result) *Verifier {
	return &Verifier{
		r:      r,
		proof:  proof,
		buffer: new(bytes.Buffer),
	}
}
