package node

import (
	"context"
	"fmt"
	"log"
	"sort"
	"testing"
	"time"

	"github.com/memoio/go-mefs-v2/config"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/repo"
)

func TestBaseNode(t *testing.T) {
	repoDir1 := "/home/fjt/testmemo1"

	repo.InitFSRepoDirect(repoDir1, repo.LatestVersion, config.NewDefaultConfig())

	bn1 := startBaseNode(repoDir1, t)
	defer bn1.Stop(context.Background())

	repoDir2 := "/home/fjt/testmemo4"

	cfg := config.NewDefaultConfig()

	cfg.Net.Addresses = []string{
		"/ip4/0.0.0.0/tcp/7002",
		"/ip6/::/tcp/7002",
	}

	repo.InitFSRepoDirect(repoDir2, repo.LatestVersion, cfg)

	ctx := context.Background()
	bn2 := startBaseNode(repoDir2, t)
	defer bn2.Stop(context.Background())

	time.Sleep(1 * time.Second)

	p1 := bn1.NetworkSubmodule.Host.ID()

	go func() {
		log.Println("start hello")
		res, err := bn2.Service.SendMetaRequest(ctx, p1, pb.NetMessage_SayHello, []byte("hello"))
		if err != nil {
			t.Fatal(err)
		}

		log.Println(string(res.Data.MsgInfo))
	}()

	go func() {
		log.Println("start get")
		res, err := bn2.Service.SendMetaRequest(ctx, p1, pb.NetMessage_Get, []byte("get"))
		if err != nil {
			t.Fatal(err)
		}

		log.Println(string(res.Data.MsgInfo))
	}()

	time.Sleep(5 * time.Second)

	log.Println(bn1.NetworkSubmodule.Host.Addrs())

	topic1, err := bn1.Pubsub.Join("sayhello")
	if err != nil {
		t.Fatal(err)
	}

	sub, err := topic1.Subscribe()
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		for {
			received, err := sub.Next(ctx)
			if err != nil {
				log.Fatal(err)
				return
			}

			log.Println("receive:", received.GetSignature())
		}
	}()

	topic2, err := bn2.Pubsub.Join("sayhello")
	if err != nil {
		t.Fatal(err)
	}

	log.Println("publish")
	topic2.Publish(ctx, []byte("ok"))

	time.Sleep(5 * time.Second)

	t.Fatal(bn1.NetworkSubmodule.Network.Peers(context.Background(), false, false, false))
}

func startBaseNode(repoDir string, t *testing.T) *BaseNode {
	rp, err := repo.OpenFSRepo(repoDir, repo.LatestVersion)
	if err != nil {
		t.Fatal(err)
	}

	defer rp.Close()

	opts, err := OptionsFromRepo(rp)
	if err != nil {
		t.Fatal(err)
	}

	bn, err := New(context.Background(), opts...)
	if err != nil {
		t.Fatal(err)
	}

	err = bn.Start(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	ifaceAddrs, err := bn.Host.Network().InterfaceListenAddresses()
	if err != nil {
		fmt.Errorf("failed to read listening addresses: %s", err)
	}

	var lisAddrs []string
	for _, addr := range ifaceAddrs {
		lisAddrs = append(lisAddrs, addr.String())
	}
	sort.Strings(lisAddrs)
	for _, addr := range lisAddrs {
		fmt.Printf("Swarm listening on %s\n", addr)
	}

	return bn
}
