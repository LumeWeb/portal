//go:build ignore

package main

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/ulikunitz/xz"
)

func main() {
	url := "https://nodejs.org/dist/v20.12.2/node-v20.12.2-linux-x64.tar.xz"
	outputDir := "node/app"

	// Create the output directory if it doesn't exist
	err := os.MkdirAll(outputDir, os.ModePerm)
	if err != nil {
		fmt.Println("Error creating output directory:", err)
		return
	}

	// Compute the node binary path based on the URL
	nodeBinary := computeNodeBinaryPath(url)

	// Download the archive and extract the node binary
	err = downloadAndExtractBinary(url, nodeBinary, outputDir)
	if err != nil {
		fmt.Println("Error downloading and extracting binary:", err)
		return
	}

	fmt.Println("Node.js binary extracted successfully.")
}
func computeNodeBinaryPath(url string) string {
	// Extract the filename from the URL
	parts := strings.Split(url, "/")
	filename := parts[len(parts)-1]

	// Remove the file extension
	filename = strings.TrimSuffix(filename, ".tar.xz")

	// Construct the node binary path
	nodeBinary := filepath.Join(filename, "bin", "node")

	return nodeBinary
}

func downloadAndExtractBinary(url, nodeBinary, outputDir string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			fmt.Println("Error closing response body:", err)
		}
	}(resp.Body)

	xzReader, err := xz.NewReader(resp.Body)
	if err != nil {
		return err
	}

	tarReader := tar.NewReader(xzReader)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		} else if err != nil {
			return err
		}

		if header.Name == nodeBinary {
			var buf bytes.Buffer
			_, err = io.Copy(&buf, tarReader)
			if err != nil {
				return err
			}

			outputPath := filepath.Join(outputDir, "node")
			err = os.WriteFile(outputPath, buf.Bytes(), 0755)
			if err != nil {
				return err
			}

			return nil
		}
	}

	return fmt.Errorf("node binary not found in the archive")
}
