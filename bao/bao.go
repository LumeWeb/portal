package bao

import (
	"bufio"
	"bytes"
	_ "embed"
	"errors"
	"github.com/docker/go-units"
	"github.com/hashicorp/go-plugin"
	"io"
	"math"
	"os"
	"os/exec"
)

//go:generate buf generate
//go:generate bash -c "cd rust && cargo build -r"
//go:embed rust/target/release/rust
var pluginBin []byte

var bao Bao
var client *plugin.Client

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

func Hash(r io.Reader) (*Result, int, error) {
	hasherId := bao.NewHasher()
	initialSize := 4 * units.KiB
	maxSize := 3.5 * units.MiB
	bufSize := initialSize

	reader := bufio.NewReaderSize(r, bufSize)
	var totalReadSize int

	for {
		buf := make([]byte, bufSize)
		n, err := reader.Read(buf)
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, 0, err
		}
		totalReadSize += n

		if !bao.Hash(hasherId, buf[:n]) {
			return nil, 0, errors.New("hashing failed")
		}

		// Adaptively adjust buffer size based on read patterns
		if n == bufSize && float64(bufSize) < maxSize {
			// If buffer was fully used, consider increasing buffer size
			bufSize = int(math.Min(float64(bufSize*2), float64(maxSize))) // Double the buffer size, up to a maximum
			reader = bufio.NewReaderSize(r, bufSize)                      // Apply new buffer size
		}
	}

	result := bao.Finish(hasherId)

	return &result, totalReadSize, nil
}
