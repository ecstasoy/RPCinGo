# Microservice Demo

This is a complete microservice example demonstrating how to use RPCinGo framework in a microservices architecture.

## Prerequisites

**This example uses Etcd Registry**, which requires etcd to be running.

### Start Etcd

You can start etcd using Docker:

```bash
docker run -d -p 2379:2379 --name etcd quay.io/coreos/etcd:v3.5.0 \
  etcd --advertise-client-urls=http://localhost:2379 \
       --listen-client-urls=http://0.0.0.0:2379
```

Or install etcd locally and run:

```bash
etcd --advertise-client-urls=http://localhost:2379 \
     --listen-client-urls=http://0.0.0.0:2379
```

## Structure

```
microservice/
├── api/
│   └── user/
│       ├── user.proto           # Proto definition
│       └── user.pb.go           # Generated code (run protoc first)
├── services/
│   └── user/
│       └── main.go              # Server implementation
├── clients/
│   └── user/
│       └── main.go              # Client implementation
└── README.md
```

## Setup

### 1. Start Etcd

Make sure etcd is running (see Prerequisites above).

### 2. Generate Proto Code

```bash
cd examples/microservice/api/user
protoc --go_out=. --go_opt=paths=source_relative user.proto
```

Or use the script:

```bash
cd examples/microservice
./scripts/gen-proto.sh
```

### 3. Start Server

```bash
cd examples/microservice/services/user
go run main.go
```

You should see:
```
UserService Server starting...
UserService Server started on 127.0.0.1:xxxxx
Registered service: UserService (GetUser, ListUsers)
Press Ctrl+C to stop
```

**Wait 1-2 seconds** for the server to register with etcd.

### 4. Run Client

In another terminal:

```bash
cd examples/microservice/clients/user
go run main.go
```

Press Enter when prompted.

You should see:
```
=== UserService Client Demo ===

1. Testing GetUser...
✅ GetUser success: ID=1, Name=Alice, Email=alice@example.com

2. Testing ListUsers...
✅ ListUsers success: Total=3
   - ID=1, Name=Alice, Email=alice@example.com
   - ID=2, Name=Bob, Email=bob@example.com
   - ID=3, Name=Charlie, Email=charlie@example.com

=== Demo completed ===
```

## Features Demonstrated

- ✅ Strong-typed service registration (`RegisterService`)
- ✅ Strong-typed client calls (`CallTyped`)
- ✅ Service discovery (Etcd Registry)
- ✅ Load balancing (RoundRobin)
- ✅ Circuit breaker
- ✅ Service watching
- ✅ Production-ready (Etcd Registry)

## Multiple Instances

Start multiple server instances to test load balancing:

```bash
# Terminal 1
PORT=8080 go run services/user/main.go

# Terminal 2
PORT=8081 go run services/user/main.go

# Terminal 3
PORT=8082 go run services/user/main.go
```

The client will automatically discover all instances and distribute requests using the configured load balancer.

## Troubleshooting

### "no available instances for UserService"

- Make sure etcd is running: `docker ps | grep etcd` or `etcdctl endpoint health`
- Wait 1-2 seconds after starting the server for it to register with etcd
- Check etcd connection: `etcdctl get /rpc/services/UserService --prefix`

### "Connect to etcd failed"

- Make sure etcd is running on `localhost:2379`
- Check etcd logs: `docker logs etcd`
- Try connecting manually: `etcdctl endpoint health`

### Connection errors

- Check that the server is running
- Verify etcd is accessible
- Check network connectivity
