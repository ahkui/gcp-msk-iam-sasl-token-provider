# Development Guide

This document is written specifically for developers of `gcp-msk-iam-sasl-token-provider` to explain the design philosophy, architecture, and how to perform local development and testing.

---

## 1. Design Philosophy

### GCP MSK Authentication Mechanism

The IAM SASL (OAuth) authentication logic for GCP Managed Service for Apache Kafka is simple and direct:

1. **Retrieve GCP Access Token**: Obtain a temporary Google Access Token from the Google authorization server via the Google OAuth2 protocol.
2. **Retrieve Identity (Email)**: Request the Service Account or User Email address corresponding to the current credentials by calling the official Google UserInfo API (`https://www.googleapis.com/oauth2/v3/userinfo`).
3. **Format as JWT**: Encode the following three parts into Base64 (with padding trimmed) and join them with `.`:
   - **Header**: `{"typ": "JWT", "alg": "GOOG_OAUTH2_TOKEN"}`
   - **Payload**: Contains expiration time `exp`, issue time `iat`, issuer `iss: "Google"`, identity `sub: "<email>"`, and scope `scope: "kafka"`.
   - **Access Token**: The Google Access Token itself.

This format conforms to the standard three-part JWT structure, where the third part is directly the Google Access Token. Thus, this is a pure Access Token retrieval and formatting wrapper package (Token Provider).

---

## 2. Project Architecture

This project adopts a minimalist and highly cohesive Go project layout. The directory structure is as follows:

```text
gcp-msk-iam-sasl-token-provider/
├── go.mod                 # Go module file
├── go.sum                 # Go module checksum file
├── LICENSE                # Apache 2.0 License
├── README.md              # Project usage documentation
├── token_provider.go      # Core Logic: Access Token retrieval and formatting (package provider)
├── token_provider_test.go # Complete unit tests
└── docs/
    └── DEVELOPMENT.md     # This Developer Guide
```

### File Responsibilities

- **`token_provider.go`**:
  - Provides `GenerateToken` and `GenerateTokenWithOptions`.
  - Encapsulates Google OAuth2 Token retrieval and `userinfo` API requests.
  - Implements the Functional Options pattern (`WithHTTPClient`, `WithTokenSource`, `WithUserInfoURL`) to support flexible extensibility and local mock/testing capabilities.
- **`token_provider_test.go`**:
  - Simulates the Google UserInfo endpoint using `httptest.NewServer`.
  - Isolates actual GCP credential chains using a custom `mockTokenSource`.
  - Achieves 100% local unit testing speed without relying on real GCP credentials.

---

## 3. Local Development and Testing

### Prerequisites

- Go `1.21` or later.

### Dependency Download and Clean

After modifying the code, run the following command to ensure `go.mod` and `go.sum` remain up to date:

```bash
go mod tidy
```

### Running Unit Tests

Execute the following command to run the unit tests:

```bash
go test -v ./...
```

### Integration Testing in Real GCP Environment

If you want to verify the token generation logic under a real GCP environment, you can use the following simple scratch script (place it in your project or run with local credentials):

1. **Configure Application Default Credentials (ADC)**:

   ```bash
   gcloud auth application-default login
   ```

2. **Write Integration Test Code**:

   ```go
   package main

   import (
   	"context"
   	"fmt"
   	"log"

   	provider "github.com/ahkui/gcp-msk-iam-sasl-token-provider"
   )

   func main() {
   	token, expiry, err := provider.GenerateToken(context.Background())
   	if err != nil {
   		log.Fatalf("Failed to generate token: %v", err)
   	}

   	fmt.Printf("Token Generated successfully!\n")
   	fmt.Printf("Expiry (Unix Ms): %d\n", expiry)
   	fmt.Printf("Raw Token Chunk (Preview): %s...\n", token[:30])
   }
   ```
