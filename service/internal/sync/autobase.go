package sync

import (
	"crypto/ed25519"
	"math"
)

var (
	NS_SIGNER_NAMESPACE []byte
	NS_VIEW_BLOCK_KEY   []byte
)

func init() {
	list := namespace("autobase", 2)
	NS_SIGNER_NAMESPACE = list[0]
	NS_VIEW_BLOCK_KEY = list[1]
}

func createAutobaseManifest(name string, bootstrapNode ed25519.PublicKey) *manifest {
	bootstrapManifest, _ := nodeKeyManifest(bootstrapNode, map[string]interface{}{})
	signers := []signer{
		{
			PublicKey: bootstrapNode,
			Signature: "ed25519",
			Namespace: deriveAutobaseNamespace(name, bootstrapManifest.Signers[0].Namespace, bootstrapNode),
		},
	}

	return &manifest{
		Version:    1,
		Hash:       "blake2b",
		AllowPatch: true,
		Quorum:     int(math.Min(float64(len(signers)), float64(len(signers)>>1)) + 1),
		Signers:    signers,
		Prologue:   nil,
	}
}

func deriveAutobaseNamespace(name string, entropy []byte, bootstrap []byte) []byte {
	encryptionId := hash([]byte{}, nil)
	version := []byte{1}

	var buf [][]byte

	bootstrap, _ = NodeKey(ed25519.PublicKey(bootstrap), map[string]interface{}{})

	buf = append(buf, NS_SIGNER_NAMESPACE)
	buf = append(buf, version)
	buf = append(buf, bootstrap)
	buf = append(buf, encryptionId)
	buf = append(buf, entropy)
	buf = append(buf, []byte(name))

	return hash(buf, nil)
}
func AutoBaseKey(bootstrapNode ed25519.PublicKey) ed25519.PublicKey {
	m := createAutobaseManifest("autobee", bootstrapNode)
	return manifestHash(m)
}
