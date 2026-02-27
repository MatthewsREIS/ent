<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-02-08 -->

# dialect/gremlin

Gremlin graph database dialect driver. Implements TinkerPop Gremlin support for graph traversals via HTTP transport with GraphSON encoding.

## Key Responsibilities

- **Driver Implementation**: `gremlin.Driver` implements `dialect.Driver` for TinkerPop-compatible servers
  - Wraps `*Client` and delegates to transport
  - `Exec` and `Query` both execute Gremlin traversal scripts
  - `Tx` returns `*Response` with vertex/edge results

- **HTTP Transport**: `HTTPTransport` sends Gremlin requests to Gremlin Server
  - Manages connection pooling
  - Implements `RoundTripper` interface for pluggable transport
  - Supports interceptors for middleware

- **GraphSON Encoding**: Serialization of graph elements to/from GraphSON format
  - Vertex, Edge, VertexProperty, Property objects
  - Type extensions (UUID, LocalDate, BigDecimal, etc.)
  - Lazy evaluation for large result sets

- **Graph Elements**: Vertex, Edge, ValueMap representations
  - Property access via label and key
  - Graph traversal state management

- **Request/Response Protocol**: Gremlin Server JSON-RPC protocol
  - `Request` with bindings (script parameters)
  - `Response` with vertex/edge data and metadata

## Architecture

```
gremlin/
├── driver.go           # Driver implementation wrapping Client
├── client.go           # Client and transport abstraction
├── config.go           # ClientConfig for driver setup
├── http.go             # HTTPTransport implementation
├── request.go          # Request struct and options
├── response.go         # Response struct and result parsing
├── encoding/           # GraphSON codec
│   ├── graphson/       # GraphSON encode/decode (15+ files)
│   │   ├── encode.go   # Type serialization
│   │   ├── decode.go   # Type deserialization
│   │   ├── extension.go # Custom type registry
│   │   └── ...
│   └── ...
├── graph/              # Graph element types
│   ├── element.go      # Element interface
│   ├── vertex.go       # Vertex type
│   ├── edge.go         # Edge type
│   ├── valuemap.go     # Property map
│   └── dsl/            # Gremlin DSL (bytecode generation)
│       ├── g/          # g.* traversal starters
│       ├── p/          # Predicate functions
│       └── __/         # Anonymous traversals
├── internal/           # WebSocket transport (not for public use)
│   └── ...
└── ocgremlin/          # OpenCensus instrumentation
    └── ...
```

## Key Files

- `driver.go` (50+ lines): `Driver` struct, `NewDriver()`, `Dialect()`, `Exec()`, `Query()`
- `client.go` (100+ lines): `Client` with `Transport`, `RoundTripper` interface, interceptor support
- `config.go` (100+ lines): `Config` struct, `Build()` method, client configuration
- `http.go` (200+ lines): `HTTPTransport`, connection pooling, request/response handling
- `request.go` (100+ lines): `Request` struct, `WithBindings()`, script execution
- `response.go` (100+ lines): `Response` struct, result parsing, vertex/edge extraction
- `encoding/graphson/encode.go` (400+ lines): Type-specific serialization
- `encoding/graphson/decode.go` (400+ lines): Type-specific deserialization
- `graph/vertex.go`: `Vertex` type with ID, label, properties
- `graph/edge.go`: `Edge` type with ID, label, from/to vertex references
- `graph/valuemap.go`: Property map access

## Key Types

- `Driver`: `dialect.Driver` implementation for Gremlin
- `Client`: Gremlin client with transport
- `RoundTripper`: HTTP request executor (like net/http)
- `Request`: Gremlin script execution request
- `Response`: Gremlin Server response with results
- `Vertex`: Graph vertex with properties
- `Edge`: Graph edge with source/target references
- `Config`: Client configuration builder

## Dependencies

- `context` (stdlib)
- `net/http` (stdlib)
- `encoding/json` (stdlib)
- `entgo.io/ent/dialect` - dialect.Driver interface
- `entgo.io/ent/dialect/gremlin/graph/dsl` - Gremlin DSL
- Go 1.18+ (generics for type extensions)

## Related Directories

- `encoding/graphson/`: GraphSON codec (see subdirectory)
- `graph/`: Graph element types and DSL
- `graph/dsl/`: Gremlin query DSL (not documented separately - leaf code)
- `internal/`: WebSocket transport (internal use only)
- `ocgremlin/`: OpenCensus instrumentation
- `../`: Parent dialect layer

## Development Notes

- Gremlin scripts are sent as strings with variable bindings
- GraphSON is the wire protocol (JSON representation with type annotations)
- Transport is pluggable via `RoundTripper` interface
- Interceptors allow middleware (logging, tracing, auth)
- Graph DSL generates bytecode that is serialized to Gremlin script
- Results include vertex/edge metadata (ID, label, properties)
- Maximum response size is 2 MB (MaxResponseSize = 2 << 20)
- WebSocket transport is internal; HTTP is the standard transport
