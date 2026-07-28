# Third-Party Notices

This file lists third-party software, source code, and test data incorporated
into the `core/keyidentity/caip122` package, along with their original authors and
licenses.

---

## github.com/spruceid/siwe-go

- **Source:** https://github.com/spruceid/siwe-go
- **Copyright:** Copyright (c) 2021 Spruce Systems, Inc.
- **License:** Apache License 2.0 / MIT (dual-licensed)
- **Usage:** EIP-4361 (SIWE) message parsing, construction, verification, and
  regex parser. Source code vendored and adapted into `core/keyidentity/caip122/message.go`.
  Original tests ported into `core/keyidentity/caip122/message_test.go`.
- **Modifications:**
  - Replaced `go-ethereum` crypto dependencies with `hyperledger-firefly/signer`
    for EIP-191 signature recovery.
  - Replaced `map[string]interface{}` options pattern with typed `MessageOptions`
    struct to eliminate unchecked type assertions.
  - Removed dead helpers (`parseTimestamp`, `isStringAndNotEmpty`).
  - Added address format validation (`0x` prefix, 42-char length).

## github.com/spruceid/siwe (siwe-js)

- **Source:** https://github.com/spruceid/siwe
- **Copyright:** Copyright (c) 2021 Spruce Systems, Inc.
- **License:** Apache License 2.0 / MIT (dual-licensed)
- **Usage:** JSON test vectors for EIP-4361 parsing and verification. Copied
 into `core/keyidentity/caip122/testdata/`:
  - `parsing_positive.json`
  - `parsing_negative.json`
  - `verification_positive.json`
  - `verification_negative.json`

## github.com/hyperledger-firefly/signer

- **Source:** https://github.com/hyperledger-firefly/signer
- **License:** Apache License 2.0
- **Usage:** secp256k1 key generation, EIP-191 signature recovery, and compact
  RSV signature decoding. Used as a Go module dependency (not vendored source).

## github.com/dchest/uniuri

- **Source:** https://github.com/dchest/uniuri
- **License:** MIT (Public Domain dedication)
- **Usage:** Cryptographically-secure random nonce generation in `GenerateNonce()`.

## github.com/relvacode/iso8601

- **Source:** https://github.com/relvacode/iso8601
- **Copyright:** Copyright (c) 2017-2020 Jason Kingsbury
- **License:** MIT
- **Usage:** ISO 8601 timestamp parsing for EIP-4361 message fields
  (`issuedAt`, `expirationTime`, `notBefore`).
