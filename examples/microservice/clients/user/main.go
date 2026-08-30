package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/ecstasoy/RPCinGo/examples/microservice/api/user"
	"github.com/ecstasoy/RPCinGo/pkg/client"
	"github.com/ecstasoy/RPCinGo/pkg/loadbalancer"
	"github.com/ecstasoy/RPCinGo/pkg/protocol"
	"github.com/ecstasoy/RPCinGo/pkg/registry/etcd"
)

func main() {
	etcdConfig := etcd.DefaultConfig()
	etcdConfig.Endpoints = []string{"localhost:2379"}
	etcdConfig.DialTimeout = 5 * time.Second

	etcdDisc, err := etcd.NewEtcdDiscovery(etcdConfig)
	if err != nil {
		fmt.Printf("Connect to etcd failed: %v\n", err)
		fmt.Println("Please make sure etcd is running on localhost:2379")
		fmt.Println("You can start etcd with: docker run -d -p 2379:2379 quay.io/coreos/etcd:v3.5.0 etcd --advertise-client-urls=http://localhost:2379 --listen-client-urls=http://0.0.0.0:2379")
		os.Exit(1)
	}
	defer etcdDisc.Close()

	fmt.Println("=== UserService Client Demo ===")
	fmt.Println()

	fmt.Println("⚠️  Note: Start the server first!")
	fmt.Println("Run: cd ../services/user && go run main.go")
	fmt.Println("Wait 1-2 seconds for server to register, then press Enter...")
	fmt.Println()

	fmt.Print("Press Enter to continue...")
	fmt.Scanln()

	cli, err := client.NewDiscoveryClient(
		client.WithDiscovery(etcdDisc),
		client.WithLoadBalancer(loadbalancer.NewRoundRobin()),
		client.WithCodec(protocol.CodecTypeProtobuf, protocol.CompressTypeNone),
		client.WithWatch(true),
		client.WithCircuitBreaker(true),
	)
	if err != nil {
		fmt.Printf("Create client failed: %v\n", err)
		os.Exit(1)
	}
	defer cli.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	fmt.Println("1. Testing GetUser...")
	getUserReq := &user.GetUserRequest{Id: 1}
	getUserResp := &user.GetUserResponse{}

	if _, err := cli.CallTyped(ctx, "UserService", "GetUser", getUserReq, getUserResp); err != nil {
		fmt.Printf("GetUser failed: %v\n", err)
	} else {
		fmt.Printf("✅ GetUser success: ID=%d, Name=%s, Email=%s\n\n",
			getUserResp.Id, getUserResp.Name, getUserResp.Email)
	}

	fmt.Println("2. Testing ListUsers...")
	listUsersReq := &user.ListUsersRequest{Page: 1, PageSize: 10}
	listUsersResp := &user.ListUsersResponse{}

	if _, err := cli.CallTyped(ctx, "UserService", "ListUsers", listUsersReq, listUsersResp); err != nil {
		fmt.Printf("ListUsers failed: %v\n", err)
	} else {
		fmt.Printf("✅ ListUsers success: Total=%d\n", listUsersResp.Total)
		for _, u := range listUsersResp.Users {
			fmt.Printf("   - ID=%d, Name=%s, Email=%s\n", u.Id, u.Name, u.Email)
		}
	}

	fmt.Println("\n=== Demo completed ===")
}
