package bao

import (
	_ "embed"
	"github.com/hashicorp/go-plugin"
	"io"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
)

//go:generate protoc --proto_path=proto/ bao.proto --go_out=proto  --go_opt=paths=source_relative --go-grpc_out=proto --go-grpc_opt=paths=source_relative

//go:embed target/release/bao
var baoPlugin []byte
var baoInstance Bao

type Bao interface {
	Init() (uint32, error)
	Write(id uint32, data []byte) error
	Finalize(id uint32) ([]byte, error)
	Destroy(id uint32) error
	ComputeFile(path string) ([]byte, error)
}

func init() {
	baoExec, err := os.CreateTemp("", "lumeportal")

	_, err = baoExec.Write(baoPlugin)
	if err != nil {
		log.Fatalf("Error:", err.Error())
	}

	err = baoExec.Sync()
	if err != nil {
		log.Fatalf("Error:", err.Error())
	}

	err = baoExec.Chmod(fs.ModePerm)
	if err != nil {
		log.Fatalf("Error:", err.Error())
	}

	err = baoExec.Close()
	if err != nil {
		log.Fatalf("Error:", err.Error())
	}
	pluginMap := map[string]plugin.Plugin{
		"bao": &BAOPlugin{},
	}
	client := plugin.NewClient(&plugin.ClientConfig{
		HandshakeConfig: plugin.HandshakeConfig{
			ProtocolVersion:  1,
			MagicCookieKey:   "foo",
			MagicCookieValue: "bar",
		},
		Plugins:          pluginMap,
		Cmd:              exec.Command("sh", "-c", baoExec.Name()),
		AllowedProtocols: []plugin.Protocol{plugin.ProtocolGRPC},
	})

	// Connect via RPC
	rpcClient, err := client.Client()
	if err != nil {
		log.Fatalf("Error:", err.Error())
	}

	// Request the plugin
	raw, err := rpcClient.Dispense("bao")
	if err != nil {
		log.Fatalf("Error:", err.Error())
	}

	baoInstance = raw.(Bao)

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-signalCh
		err := os.Remove(baoExec.Name())
		if err != nil {
			log.Fatalf("Error:", err.Error())
		}
	}()

}

func ComputeTreeStreaming(reader io.Reader) ([]byte, error) {
	instance, err := baoInstance.Init()
	if err != nil {
		return nil, err
	}

	b := make([]byte, 512)
	for {
		n, err := reader.Read(b)

		if n > 0 {
			err := write(instance, &b)
			if err != nil {
				return nil, err
			}
		}

		if err != nil {
			var result []byte
			if err == io.EOF {
				result, err = finalize(instance)
				if err == nil {
					return result, nil
				}
			}
			return nil, err
		}
	}
}

func ComputeTreeFile(file *os.File) ([]byte, error) {
	tree, err := baoInstance.ComputeFile(file.Name())
	if err != nil {
		return nil, err
	}

	return tree, nil
}

func write(instance uint32, bytes *[]byte) error {
	err := baoInstance.Write(instance, *bytes)
	if err != nil {
		derr := destroy(instance)
		if derr != nil {
			return derr
		}
		return err
	}
	if err != nil {
		derr := destroy(instance)
		if derr != nil {
			return derr
		}
		return err
	}

	return nil
}

func finalize(instance uint32) ([]byte, error) {
	result, err := baoInstance.Finalize(instance)
	if err != nil {
		derr := destroy(instance)
		if derr != nil {
			return nil, derr
		}
		return nil, err
	}

	return result, nil
}
func destroy(instance uint32) error {
	return baoInstance.Destroy(instance)
}
