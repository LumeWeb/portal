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
	"time"

	"github.com/samber/lo"

	"go.uber.org/zap"

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
	r          io.ReadCloser
	proof      Result
	read       uint64
	buffer     *bytes.Buffer
	logger     *zap.Logger
	readTime   []time.Duration
	verifyTime time.Duration
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
		readStart := time.Now()
		bytesRead, err := io.ReadFull(v.r, buf)
		if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
			return n, err // Return any read error immediately
		}

		readEnd := time.Now()

		v.readTime = append(v.readTime, readEnd.Sub(readStart))

		timeStart := time.Now()

		if bytesRead > 0 {
			if status, err := bao.Verify(buf[:bytesRead], v.read, v.proof.Proof, v.proof.Hash); err != nil || !status {
				return n, errors.Join(ErrVerifyFailed, err)
			}
			v.read += uint64(bytesRead)
			v.buffer.Write(buf[:bytesRead]) // Append new data to the buffer
		}

		timeEnd := time.Now()
		v.verifyTime += timeEnd.Sub(timeStart)

		if err == io.EOF {
			// If EOF, break the loop as no more data can be read
			break
		}
	}

	if len(v.readTime) > 0 {
		averageReadTime := lo.Reduce(v.readTime, func(acc time.Duration, cur time.Duration, _ int) time.Duration {
			return acc + cur
		}, time.Duration(0)) / time.Duration(len(v.readTime))

		v.logger.Debug("Read time", zap.Duration("average", averageReadTime))
	}

	averageVerifyTime := v.verifyTime / time.Duration(v.read/VERIFY_CHUNK_SIZE)
	v.logger.Debug("Verification time", zap.Duration("average", averageVerifyTime))

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

func NewVerifier(r io.ReadCloser, proof Result, logger *zap.Logger) *Verifier {
	return &Verifier{
		r:      r,
		proof:  proof,
		buffer: new(bytes.Buffer),
		logger: logger,
	}
}
