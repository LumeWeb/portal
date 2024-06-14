package sync

import (
	"crypto/ed25519"
	"errors"
	"fmt"
)

var (
	MANIFEST          []byte
	DEFAULT_NAMESPACE []byte
)

func init() {
	list := namespace("hypercore", 6)
	MANIFEST = list[3]
	DEFAULT_NAMESPACE = list[4]
}

type signer struct {
	Signature string
	Namespace []byte
	PublicKey ed25519.PublicKey
}

type manifest struct {
	Version    int
	Hash       string
	AllowPatch bool
	Quorum     int
	Signers    []signer
	Prologue   *Prologue
}

type Prologue struct {
	Hash   []byte
	Length int
}

func createManifest(inp map[string]interface{}) (*manifest, error) {
	if inp == nil {
		return nil, nil
	}

	manifest := &manifest{
		Version:    1,
		Hash:       "blake2b",
		AllowPatch: false,
		Quorum:     defaultQuorum(inp),
		Signers:    []signer{},
		Prologue:   nil,
	}

	if version, ok := inp["version"].(int); ok {
		manifest.Version = version
	}

	if allowPatch, ok := inp["allowPatch"].(bool); ok {
		manifest.AllowPatch = allowPatch
	}

	if hash, ok := inp["hash"].(string); ok {
		if hash != "blake2b" {
			return nil, errors.New("Only Blake2b hashes are supported")
		}
		manifest.Hash = hash
	}

	if signers, ok := inp["signers"].([]map[string]interface{}); ok {
		for _, signer := range signers {
			parsedSigner, err := parseSigner(signer)
			if err != nil {
				return nil, err
			}
			manifest.Signers = append(manifest.Signers, parsedSigner)
		}
	}

	if prologue, ok := inp["prologue"].(map[string]interface{}); ok {
		hash, ok := prologue["hash"].([]byte)
		if !ok || len(hash) != 32 {
			return nil, errors.New("Invalid prologue")
		}

		length, ok := prologue["length"].(int)
		if !ok || length < 0 {
			return nil, errors.New("Invalid prologue")
		}

		manifest.Prologue = &Prologue{
			Hash:   hash,
			Length: length,
		}
	}

	return manifest, nil
}

func NodeKey(manifest interface{}, opts map[string]interface{}) (ed25519.PublicKey, error) {
	compat, _ := opts["compat"].(bool)
	m, err := nodeKeyManifest(manifest, opts)
	if err != nil {
		return nil, err
	}

	if compat {
		return m.Signers[0].PublicKey, nil
	}

	return manifestHash(m), nil
}

func nodeKeyManifest(manifest interface{}, opts map[string]interface{}) (*manifest, error) {
	version, _ := opts["version"].(int)
	_namespace, _ := opts["namespace"].([]byte)

	var manifestMap map[string]interface{}

	if buf, ok := manifest.(ed25519.PublicKey); ok {
		manifestMap = map[string]interface{}{
			"version": version,
			"signers": []map[string]interface{}{{
				"publicKey": buf,
				"namespace": _namespace,
			}},
		}
	} else {
		var ok bool
		manifestMap, ok = manifest.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("invalid manifest type")
		}
	}

	m, err := createManifest(manifestMap)
	if err != nil {
		return nil, err
	}

	return m, nil
}

func defaultQuorum(inp map[string]interface{}) int {
	if quorum, ok := inp["quorum"].(int); ok {
		return quorum
	}

	signers, ok := inp["signers"].([]map[string]interface{})
	if !ok || len(signers) == 0 {
		return 0
	}

	return (len(signers) >> 1) + 1
}

func parseSigner(_signer interface{}) (signer, error) {
	signerMap, ok := _signer.(map[string]interface{})
	if !ok {
		return signer{}, errors.New("Invalid signer format")
	}

	err := validateSigner(signerMap)
	if err != nil {
		return signer{}, err
	}

	namespace, _ := signerMap["namespace"].([]byte)
	if namespace == nil {
		namespace = DEFAULT_NAMESPACE
	}

	return signer{
		Signature: "ed25519",
		Namespace: namespace,
		PublicKey: signerMap["publicKey"].(ed25519.PublicKey),
	}, nil
}

func validateSigner(signer map[string]interface{}) error {
	if signer == nil {
		return errors.New("Signer is nil")
	}

	publicKey, ok := signer["publicKey"].(ed25519.PublicKey)
	if !ok || len(publicKey) == 0 {
		return errors.New("Signer missing public key")
	}

	signature, _ := signer["signature"].(string)
	if signature != "" && signature != "ed25519" {
		return errors.New("Only Ed25519 signatures are supported")
	}

	return nil
}
