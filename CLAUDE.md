# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Eventrouter is a Kubernetes-native Go application that watches Kubernetes events and routes them to configurable sinks for long-term storage, analysis, and logging. It serves as an active watcher of event resources in the Kubernetes system and pushes them to user-specified destinations.

## Development Commands

### Building and Testing
- `make build` - Build the eventrouter binary
- `make test` - Run all tests with verbose output and 60s timeout
- `make vet` - Run go vet for code analysis
- `make` or `make all` - Run vet and test (default target)

### Docker
- Build image: `docker build -t eventrouter .`
- Multi-platform builds are supported via BuildKit (BUILDPLATFORM, TARGETOS, TARGETARCH)

### Kubernetes Deployment
- Deploy: `kubectl create -f https://raw.githubusercontent.com/kube-logging/eventrouter/master/yaml/eventrouter.yaml`
- Remove: `kubectl delete -f https://raw.githubusercontent.com/kube-logging/eventrouter/master/yaml/eventrouter.yaml`
- View logs: `kubectl logs -f deployment/eventrouter -n kube-system`

## Architecture

### Core Components
- **EventRouter** (`eventrouter.go`): Main controller that watches Kubernetes events using client-go informers
- **Sinks** (`sinks/`): Pluggable output destinations for events
- **Configuration**: Uses Viper for config management with support for JSON files and environment variables

### Available Sinks
Located in `sinks/` directory:
- `glogsink.go` - Default glog output (default sink)
- `stdoutsink.go` - Standard output
- `httpsink.go` - HTTP endpoints
- `kafkasink.go` - Apache Kafka
- `influxdb.go` - InfluxDB time series database
- `eventhub.go` - Azure Event Hubs
- `s3sink.go` - Amazon S3
- `rocksetsink.go` - Rockset real-time analytics

### Configuration System
- **Config files**: `/etc/eventrouter/config.json` or `./config.json`
- **Environment variables**: Supported via Viper
- **Command-line flags**: Limited set for metrics listening address
- **Default values**:
  - Sink: `glog`
  - Resync interval: 30 minutes
  - Prometheus metrics: enabled
  - Listen address: `:8080`

### Prometheus Metrics
Built-in metrics tracking event counts by type:
- `heptio_eventrouter_warnings_total`
- `heptio_eventrouter_normal_total`
- `heptio_eventrouter_info_total`
- `heptio_eventrouter_unknown_total`

All metrics include labels for involved object kind, name, namespace, reason, and source.

## Key Implementation Details

### Event Processing
- Uses Kubernetes client-go informers for efficient event watching
- Maintains resource version tracking for resumption after restarts
- Graceful shutdown handling for SIGINT, SIGTERM, and other signals
- Thread-safe event processing with proper error handling

### Sink Interface
All sinks implement `EventSinkInterface` (`sinks/interfaces.go`) with methods:
- `UpdateEvents(events []*v1.Event)`
- Configuration-specific initialization

### Dependencies
- **Go version**: 1.25 (as per go.mod and Dockerfile)
- **Kubernetes**: Uses client-go v0.27.4
- **Key libraries**: Viper (config), glog (logging), Prometheus (metrics)
- **Container base**: Uses distroless/static-debian12 for minimal attack surface

## Configuration Examples

### Basic glog output (default)
```json
{
  "sink": "glog"
}
```

### Kafka sink with custom config
```json
{
  "sink": "kafka",
  "kafkabrokers": "kafka:9092",
  "kafkatopic": "kubernetes-events"
}
```

## Testing Strategy

Tests are located in `tests/` directory and focus on:
- Sink-specific functionality testing
- Integration testing with different backends
- Configuration validation

Use `go test ./... -v -timeout 60s` to run the full test suite with proper timeouts.

## Deployment Notes

- Designed to run as a Kubernetes deployment in the `kube-system` namespace
- Requires appropriate RBAC permissions to watch events cluster-wide
- Supports both in-cluster and external kubeconfig authentication
- Can be deployed with custom configuration via ConfigMaps or environment variables