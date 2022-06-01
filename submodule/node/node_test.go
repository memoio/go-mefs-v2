package node

import (
	"context"
	"fmt"
	"log"
	"sort"
	"testing"
	"time"

	"github.com/memoio/go-mefs-v2/app/minit"
	"github.com/memoio/go-mefs-v2/config"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/repo"
)

func TestBaseNode(t *testing.T) {
	ctx := context.Background()

	repoDir1 := "/home/fjt/testmemo10"

	cfg1 := config.NewDefaultConfig()
	cfg1.Identity.Role = "keeper"

	bn1 := startBaseNode(repoDir1, cfg1, t)
	defer bn1.Stop(context.Background())

	repoDir2 := "/home/fjt/testmemo11"

	cfg2 := config.NewDefaultConfig()
	cfg2.Net.Addresses = []string{
		"/ip4/0.0.0.0/tcp/7002",
		"/ip6/::/tcp/7002",
	}

	bn2 := startBaseNode(repoDir2, cfg2, t)
	defer bn2.Stop(context.Background())

	time.Sleep(1 * time.Second)

	p1 := bn1.RoleID()

	go func() {
		log.Println("start hello")
		res, err := bn2.SendMetaRequest(ctx, p1, pb.NetMessage_SayHello, []byte("hello"), nil)
		if err != nil {
			t.Log(err)
			return
		}

		log.Println(string(res.Data.MsgInfo))
	}()

	go func() {
		log.Println("start get")
		res, err := bn2.SendMetaRequest(ctx, p1, pb.NetMessage_Get, []byte("get"), nil)
		if err != nil {
			t.Log(err)
			return
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

	time.Sleep(10 * time.Second)

	fn := func(key, val []byte) error {
		fmt.Println(string(key))
		return nil
	}

	fmt.Println("=========")
	bn1.Repo.MetaStore().Iter([]byte("/"), fn)
	fmt.Println("=========")
	bn2.Repo.MetaStore().Iter([]byte("/"), fn)

	t.Fatal(bn1.NetworkSubmodule.NetPeers(context.Background()))
}

func startBaseNode(repoDir string, cfg *config.Config, t *testing.T) *BaseNode {
	rp, err := repo.NewFSRepo(repoDir, cfg)
	if err != nil {
		t.Fatal(err)
	}

	err = minit.Create(context.Background(), rp, "memoriae", "")
	if err != nil {
		t.Fatal(err)
	}

	err = rp.ReplaceConfig(rp.Config())
	if err != nil {
		t.Fatal(err)
	}

	opts, err := OptionsFromRepo(rp)
	if err != nil {
		t.Fatal(err)
	}

	opts = append(opts, SetPassword("memoriae"))

	bn, err := New(context.Background(), opts...)
	if err != nil {
		t.Fatal(err)
	}

	err = bn.Start(true)
	if err != nil {
		t.Fatal(err)
	}

	ifaceAddrs, err := bn.Host.Network().InterfaceListenAddresses()
	if err != nil {
		t.Log("failed to read listening addresses: %w", err)
	}

	var lisAddrs []string
	for _, addr := range ifaceAddrs {
		lisAddrs = append(lisAddrs, addr.String())
	}
	sort.Strings(lisAddrs)
	for _, addr := range lisAddrs {
		logger.Info("Swarm listening on: ", addr)
	}

	return bn
}
