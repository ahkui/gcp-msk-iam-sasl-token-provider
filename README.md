# GCP MSK IAM SASL Token Provider for Go

[![Go CI](https://github.com/ahkui/gcp-msk-iam-sasl-token-provider/actions/workflows/go.yml/badge.svg)](https://github.com/ahkui/gcp-msk-iam-sasl-token-provider/actions/workflows/go.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/ahkui/gcp-msk-iam-sasl-token-provider.svg)](https://pkg.go.dev/github.com/ahkui/gcp-msk-iam-sasl-token-provider)
[![Go Report Card](https://goreportcard.com/badge/github.com/ahkui/gcp-msk-iam-sasl-token-provider)](https://goreportcard.com/report/github.com/ahkui/gcp-msk-iam-sasl-token-provider)
[![License](https://img.shields.io/github/license/ahkui/gcp-msk-iam-sasl-token-provider)](LICENSE)

`gcp-msk-iam-sasl-token-provider` is a Go library that provides a GCP Managed Service for Apache Kafka IAM SASL (OAUTHBEARER) Token Provider.

This package focuses on retrieving and querying the underlying Google Access Token, and formatting it into the standard JWT format required by GCP Managed Service for Apache Kafka. Designed to be lightweight and zero-dependency, it does not rely on any specific Kafka client libraries (e.g., Sarama), offering maximum generalizability.

This package requires `Go 1.25` or later.

> [!NOTE]
> **About GCP Authentication Mechanism**
> GCP Managed Service for Apache Kafka utilizes a **Google OAuth2 Access Token**. This package retrieves a valid Google access token and formats it along with the credential's Email identity into the three-part Base64-encoded JWT format mandated by GCP Managed Service for Apache Kafka's OAUTHBEARER authentication specifications.

---

## Getting Started

### 1. Add Dependency

```sh
go get github.com/ahkui/gcp-msk-iam-sasl-token-provider
```

### 2. Integration Example with Kafka Client (IBM/sarama)

You can directly connect this token provider to [IBM/sarama](https://github.com/IBM/sarama)'s `AccessTokenProvider` interface:

```go
package main

import (
	"context"
	"crypto/tls"
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/IBM/sarama"
	provider "github.com/ahkui/gcp-msk-iam-sasl-token-provider"
)

var (
	kafkaBrokers = []string{"your-gcp-kafka-bootstrap-endpoint:9098"}
	KafkaTopic   = "my-topic"
)

// GCPTokenProvider implements the sarama.AccessTokenProvider interface
type GCPTokenProvider struct{}

func (p *GCPTokenProvider) Token() (*sarama.AccessToken, error) {
	// Generate connection token using Google Application Default Credentials
	token, _, err := provider.GenerateToken(context.Background())
	if err != nil {
		return nil, err
	}
	return &sarama.AccessToken{Token: token}, nil
}

func main() {
	sarama.Logger = log.New(os.Stdout, "[sarama] ", log.LstdFlags)

	config := sarama.NewConfig()
	config.Net.SASL.Enable = true
	config.Net.SASL.Mechanism = sarama.SASLTypeOAuth
	config.Net.SASL.TokenProvider = &GCPTokenProvider{}

	config.Net.TLS.Enable = true
	config.Net.TLS.Config = &tls.Config{
		MinVersion: tls.VersionTLS12,
	}

	producer, err := sarama.NewAsyncProducer(kafkaBrokers, config)
	if err != nil {
		log.Fatalf("Failed to start producer: %v", err)
	}
	defer producer.Close()

	log.Println("GCP Kafka AsyncProducer is running!")

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt)

	go func() {
		for {
			time.Sleep(time.Second)
			msg := &sarama.ProducerMessage{
				Topic: KafkaTopic,
				Value: sarama.StringEncoder("hello from gcp sasl token provider"),
			}
			producer.Input() <- msg
		}
	}()

	<-signals
	log.Println("Shutting down...")
}
```

---

## Advanced Configurations (Options)

`GenerateTokenWithOptions` provides several functional options to meet the configuration requirements of specialized architectural environments or testing environments:

```go
import (
	"golang.org/x/oauth2"
	provider "github.com/ahkui/gcp-msk-iam-sasl-token-provider"
)

// 1. Custom OAuth2 TokenSource (e.g., manually loading a specific Service Account key)
var customTokenSource oauth2.TokenSource = ...
token, expiryMs, err := provider.GenerateTokenWithOptions(
	ctx,
	provider.WithTokenSource(customTokenSource),
)

// 2. Custom HTTP Client (e.g., when calling Google UserInfo API needs to go through an enterprise proxy)
var customClient *http.Client = ...
token, expiryMs, err := provider.GenerateTokenWithOptions(
	ctx,
	provider.WithHTTPClient(customClient),
)

// 3. Custom UserInfo Endpoint URL (primarily used for local development, testing, or mock integration)
token, expiryMs, err := provider.GenerateTokenWithOptions(
	ctx,
	provider.WithUserInfoURL("https://my-internal-mock-server/userinfo"),
)
```

---

## Local Development and Contribution

If you are a developer for this project, or wish to build and debug this library locally, please refer to our [Development Guide](docs/DEVELOPMENT.md).

---

## License

This project is licensed under the Apache License 2.0. See the [LICENSE](LICENSE) file for details.
