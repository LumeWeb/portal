package bao

import (
	_ "embed"
	"errors"
	"github.com/second-state/WasmEdge-go/wasmedge"
	bindgen "github.com/second-state/wasmedge-bindgen/host/go"
	"io"
	"os"
)

//go:embed target/wasm32-wasi/release/bao.wasm
var wasm []byte

var conf *wasmedge.Configure

func init() {
	wasmedge.SetLogErrorLevel()
	conf = wasmedge.NewConfigure(wasmedge.WASI)
}

func ComputeBaoTree(reader io.Reader) ([]byte, error) {
	var vm = wasmedge.NewVMWithConfig(conf)
	var wasi = vm.GetImportModule(wasmedge.WASI)
	wasi.InitWasi(
		os.Args[1:],     // The args
		os.Environ(),    // The envs
		[]string{".:."}, // The mapping preopens
	)
	err := vm.LoadWasmBuffer(wasm)
	if err != nil {
		return nil, err
	}
	err = vm.Validate()
	if err != nil {
		return nil, err
	}

	bg := bindgen.New(vm)
	bg.Instantiate()

	_, _, err = bg.Execute("init")
	if err != nil {
		bg.Release()
		return nil, err
	}

	b := make([]byte, 512)
	for {
		n, err := reader.Read(b)

		if n > 0 {
			err := write(*bg, &b)
			if err != nil {
				return nil, err
			}
		}

		if err != nil {
			var result []byte
			if err == io.EOF {
				result, err = finalize(*bg)
				if err == nil {
					return result, nil
				}
			}
			return nil, err
		}
	}
}

func write(bg bindgen.Bindgen, bytes *[]byte) error {
	_, _, err := bg.Execute("write", *bytes)
	if err != nil {
		bg.Release()
		return err
	}

	return nil
}

func finalize(bg bindgen.Bindgen) ([]byte, error) {
	var byteResult []byte

	result, _, err := bg.Execute("finalize")
	if err != nil {
		bg.Release()
		return nil, err
	}

	// Iterate over each element in the result slice
	for _, elem := range result {
		// Type assert the element to []byte
		byteSlice, ok := elem.([]byte)
		if !ok {
			// If the element is not a byte slice, return an error
			return nil, errors.New("result element is not a byte slice")
		}

		// Concatenate the byte slice to the byteResult slice
		byteResult = append(byteResult, byteSlice...)
	}

	return byteResult, nil
}
