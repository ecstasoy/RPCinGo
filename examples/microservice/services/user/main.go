package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ecstasoy/RPCinGo/examples/microservice/api/user"
	"github.com/ecstasoy/RPCinGo/pkg/protocol"
	"github.com/ecstasoy/RPCinGo/pkg/registry/etcd"
	"github.com/ecstasoy/RPCinGo/pkg/server"
)

type UserService struct {
	users map[int64]*user.User
}

func NewUserService() *UserService {
	users := make(map[int64]*user.User)
	users[1] = &user.User{Id: 1, Name: "Alice", Email: "alice@example.com"}
	users[2] = &user.User{Id: 2, Name: "Bob", Email: "bob@example.com"}
	users[3] = &user.User{Id: 3, Name: "Charlie", Email: "charlie@example.com"}

	return &UserService{
		users: users,
	}
}

func (s *UserService) GetUser(ctx context.Context, req *user.GetUserRequest) (*user.GetUserResponse, error) {
	u, exists := s.users[req.Id]
	if !exists {
		return nil, fmt.Errorf("user not found: %d", req.Id)
	}

	fmt.Printf("[Server] GetUser called: id=%d\n", req.Id)

	return &user.GetUserResponse{
		Id:    u.Id,
		Name:  u.Name,
		Email: u.Email,
	}, nil
}

func (s *UserService) ListUsers(ctx context.Context, req *user.ListUsersRequest) (*user.ListUsersResponse, error) {
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = 10
	}

	var usersList []*user.User
	start := int((req.Page - 1) * req.PageSize)
	end := start + int(req.PageSize)

	count := 0
	for _, u := range s.users {
		if count >= start && count < end {
			usersList = append(usersList, u)
		}
		count++
	}

	fmt.Printf("[Server] ListUsers called: page=%d, page_size=%d\n", req.Page, req.PageSize)

	return &user.ListUsersResponse{
		Users: usersList,
		Total: int32(len(s.users)),
	}, nil
}

func main() {
	etcdConfig := etcd.DefaultConfig()
	etcdConfig.Endpoints = []string{"localhost:2379"}
	etcdConfig.DialTimeout = 5 * time.Second

	etcdReg, err := etcd.NewEtcdRegistry(etcdConfig)
	if err != nil {
		fmt.Printf("Connect to etcd failed: %v\n", err)
		fmt.Println("Please make sure etcd is running on localhost:2379")
		fmt.Println("You can start etcd with: docker run -d -p 2379:2379 quay.io/coreos/etcd:v3.5.0 etcd --advertise-client-urls=http://localhost:2379 --listen-client-urls=http://0.0.0.0:2379")
		os.Exit(1)
	}
	defer func() { _ = etcdReg.Close() }()

	srv := server.NewServer(
		server.WithAddress("127.0.0.1:0"),
		server.WithCodec(protocol.CodecTypeProtobuf, protocol.CompressTypeGzip),
		server.WithRegistry("UserService", "v1.0.0", etcdReg),
	)

	userService := NewUserService()
	if err := srv.RegisterService("UserService", userService); err != nil {
		fmt.Printf("RegisterService failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("UserService Server starting...\n")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := srv.Start(ctx); err != nil {
			fmt.Printf("Server error: %v\n", err)
		}
	}()

	time.Sleep(500 * time.Millisecond)

	addr := srv.Addr()
	if addr != "" {
		fmt.Printf("UserService Server started on %s\n", addr)
		fmt.Println("Registered service: UserService (GetUser, ListUsers)")
	} else {
		fmt.Println("Waiting for server to start...")
		time.Sleep(200 * time.Millisecond)
		addr = srv.Addr()
		fmt.Printf("UserService Server started on %s\n", addr)
	}

	fmt.Println("Press Ctrl+C to stop")

	<-sigChan
	fmt.Println("\nShutting down server...")
	_ = srv.Stop()
	fmt.Println("Server stopped")
}
