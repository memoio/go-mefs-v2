package main

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/memoio/go-mefs-v2/api/client"
	"github.com/memoio/go-mefs-v2/app/cmd"
	"github.com/memoio/go-mefs-v2/lib/utils"
	"github.com/modood/table"

	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"
)

var OrderCmd = &cli.Command{
	Name:  "order",
	Usage: "Interact with order",
	Subcommands: []*cli.Command{
		orderListJobCmd,
		orderListPayCmd,
		orderGetCmd,
		orderDetailCmd,
		orderListProvidersCmd,
	},
}

var orderListProvidersCmd = &cli.Command{
	Name:  "proList",
	Usage: "list all pros",
	Action: func(cctx *cli.Context) error {
		repoDir := cctx.String(cmd.FlagNodeRepo)
		addr, headers, err := client.GetMemoClientInfo(repoDir)
		if err != nil {
			return err
		}

		api, closer, err := client.NewUserNode(cctx.Context, addr, headers)
		if err != nil {
			return err
		}
		defer closer()

		ois, err := api.OrderGetJobInfo(cctx.Context)
		if err != nil {
			return err
		}

		type outPutInfo struct {
			ID     uint64
			Ready  bool
			InStop bool
			PeerID string
			Addrs  []string
		}
		outPut := make([]outPutInfo, 0)
		var tmp outPutInfo
		for _, oi := range ois {
			if oi.PeerID != "" {
				pid, err := peer.Decode(oi.PeerID)
				if err != nil {
					continue
				}
				epi, err := api.NetPeerInfo(cctx.Context, pid)
				if err == nil {
					addrs := make([]string, 0, len(epi.Addrs))
					for _, maddr := range epi.Addrs {
						if strings.Contains(maddr, "/127.0.0.1/") {
							continue
						}

						if strings.Contains(maddr, "/::1/") {
							continue
						}

						addrs = append(addrs, maddr)
					}

					if len(addrs) == 0 {
						pi, err := api.StateGetNetInfo(cctx.Context, oi.ID)
						if err == nil {
							for _, maddr := range pi.Addrs {
								if strings.Contains(maddr.String(), "/127.0.0.1/") {
									continue
								}

								if strings.Contains(maddr.String(), "/::1/") {
									continue
								}

								addrs = append(addrs, maddr.String())
							}
						}
					}

					tmp = outPutInfo{oi.ID, oi.Ready, oi.InStop, oi.PeerID, addrs}
					outPut = append(outPut, tmp)
					continue
				}
			}

			tmp = outPutInfo{oi.ID, oi.Ready, oi.InStop, oi.PeerID, nil}
			outPut = append(outPut, tmp)
		}

		sort.Slice(outPut, func(i, j int) bool { return outPut[i].ID < outPut[j].ID })
		table.Output(outPut)
		return nil
	},
}

var orderListJobCmd = &cli.Command{
	Name:  "jobList",
	Usage: "list jobs of all pros",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "verbose",
			Usage: "filter output",
			Value: false,
		},
	},
	Action: func(cctx *cli.Context) error {
		repoDir := cctx.String(cmd.FlagNodeRepo)
		addr, headers, err := client.GetMemoClientInfo(repoDir)
		if err != nil {
			return err
		}

		api, closer, err := client.NewUserNode(cctx.Context, addr, headers)
		if err != nil {
			return err
		}
		defer closer()

		ois, err := api.OrderGetJobInfo(cctx.Context)
		if err != nil {
			return err
		}

		sort.Slice(ois, func(i, j int) bool { return ois[i].ID < ois[j].ID })

		verbose := cctx.Bool("verbose")

		type outPutOrderJobInfo struct {
			ID         uint64
			Jobs       int
			Nonce      uint64
			OrderState string
			Time       string
			SeqNum     uint32
			SeqState   string
			Ready      bool
			InStop     bool
			AvailTime  string
		}

		tmpOutputWait := make([]outPutOrderJobInfo, 0)
		tmpOutputRunning := make([]outPutOrderJobInfo, 0)
		tmpOutputClosing := make([]outPutOrderJobInfo, 0)
		tmpOutputDone := make([]outPutOrderJobInfo, 0)
		tmpOutputInit := make([]outPutOrderJobInfo, 0)
		var tmp outPutOrderJobInfo

		for _, oi := range ois {
			if !verbose && oi.InStop || oi.OrderTime == 0 {
				continue
			}
			tmp = outPutOrderJobInfo{oi.ID, oi.Jobs, oi.Nonce, oi.OrderState, time.Unix(int64(oi.OrderTime), 0).Format(utils.SHOWTIME), oi.SeqNum, oi.SeqState, oi.Ready, oi.InStop, time.Unix(int64(oi.AvailTime), 0).Format(utils.SHOWTIME)}
			switch oi.OrderState {
			case "wait":
				tmpOutputWait = append(tmpOutputWait, tmp)
			case "running":
				tmpOutputRunning = append(tmpOutputRunning, tmp)
			case "closing":
				tmpOutputClosing = append(tmpOutputClosing, tmp)
			case "done":
				tmpOutputDone = append(tmpOutputDone, tmp)
			case "init":
				tmpOutputInit = append(tmpOutputInit, tmp)
			}
		}
		tmpOutputWait = append(tmpOutputWait, tmpOutputRunning...)
		tmpOutputWait = append(tmpOutputWait, tmpOutputClosing...)
		tmpOutputWait = append(tmpOutputWait, tmpOutputDone...)
		tmpOutputWait = append(tmpOutputWait, tmpOutputInit...)
		table.Output(tmpOutputWait)

		return nil
	},
}

var orderListPayCmd = &cli.Command{
	Name:  "payList",
	Usage: "list pay infos all pros",
	Action: func(cctx *cli.Context) error {
		repoDir := cctx.String(cmd.FlagNodeRepo)
		addr, headers, err := client.GetMemoClientInfo(repoDir)
		if err != nil {
			return err
		}

		api, closer, err := client.NewUserNode(cctx.Context, addr, headers)
		if err != nil {
			return err
		}
		defer closer()

		ois, err := api.OrderGetPayInfo(cctx.Context)
		if err != nil {
			return err
		}

		type outPutInfo struct {
			ID          uint64
			Size        uint64
			ConfirmSize uint64
			OnChainSize uint64
			NeedPay     string
			Paid        string
		}
		outPut := make([]outPutInfo, 0)
		var tmp outPutInfo

		for _, oi := range ois {
			tmp = outPutInfo{oi.ID, oi.Size, oi.ConfirmSize, oi.OnChainSize, oi.NeedPay.String(), oi.Paid.String()}
			outPut = append(outPut, tmp)
		}
		sort.Slice(outPut, func(i, j int) bool { return outPut[i].ID < outPut[j].ID })
		table.Output(outPut)

		return nil
	},
}

var orderGetCmd = &cli.Command{
	Name:      "get",
	Usage:     "get order info of one provider",
	ArgsUsage: "[provider index required]",
	Action: func(cctx *cli.Context) error {
		if !cctx.Args().Present() {
			return xerrors.Errorf("need amount")
		}
		pid, err := strconv.ParseUint(cctx.Args().First(), 10, 0)
		if err != nil {
			return xerrors.Errorf("parsing 'amount' argument: %w", err)
		}

		repoDir := cctx.String(cmd.FlagNodeRepo)
		addr, headers, err := client.GetMemoClientInfo(repoDir)
		if err != nil {
			return err
		}

		api, closer, err := client.NewUserNode(cctx.Context, addr, headers)
		if err != nil {
			return err
		}
		defer closer()

		oi, err := api.OrderGetJobInfoAt(cctx.Context, pid)
		if err != nil {
			return err
		}

		ns := api.StateGetOrderState(cctx.Context, api.SettleGetRoleID(cctx.Context), pid)

		si, err := api.SettleGetStoreInfo(cctx.Context, api.SettleGetRoleID(cctx.Context), pid)
		if err != nil {
			return err
		}

		type outPutInfo struct {
			ID            uint64
			Jobs          int
			StoreNonce    uint64
			SeqNonce      uint64
			OrderJobNonce uint64
			OrderState    string
			OrderTime     string
			SeqNum        uint32
			SeqState      string
			Ready         bool
			InStop        bool
			AvailTime     string
		}

		tmp := []outPutInfo{{oi.ID, oi.Jobs, si.Nonce, ns.Nonce, oi.Nonce, oi.OrderState, time.Unix(int64(oi.OrderTime), 0).Format(utils.SHOWTIME), oi.SeqNum, oi.SeqState, oi.Ready, oi.InStop, time.Unix(int64(oi.AvailTime), 0).Format(utils.SHOWTIME)}}

		table.Output(tmp)

		return nil
	},
}

var orderDetailCmd = &cli.Command{
	Name:      "detail",
	Usage:     "get detail order seq info of one provider",
	ArgsUsage: "[provider index required] [order nonce] [seq number]",
	Action: func(cctx *cli.Context) error {
		if cctx.Args().Len() != 3 {
			return xerrors.Errorf("need three parameters")
		}

		pid, err := strconv.ParseUint(cctx.Args().First(), 10, 0)
		if err != nil {
			return xerrors.Errorf("parsing 'pro indec' argument: %w", err)
		}

		nc, err := strconv.ParseUint(cctx.Args().Get(1), 10, 0)
		if err != nil {
			return xerrors.Errorf("parsing 'nonce' argument: %w", err)
		}

		sn, err := strconv.ParseUint(cctx.Args().Get(2), 10, 0)
		if err != nil {
			return xerrors.Errorf("parsing 'seq number' argument: %w", err)
		}

		repoDir := cctx.String(cmd.FlagNodeRepo)
		addr, headers, err := client.GetMemoClientInfo(repoDir)
		if err != nil {
			return err
		}

		api, closer, err := client.NewUserNode(cctx.Context, addr, headers)
		if err != nil {
			return err
		}
		defer closer()

		oi, err := api.OrderGetDetail(cctx.Context, pid, nc, uint32(sn))
		if err != nil {
			return err
		}

		type outPutInfo struct {
			ProID      uint64
			Nonce      uint64
			SeqNum     uint32
			Size       uint64
			SegmentNum int
		}

		tmp := []outPutInfo{{oi.ProID, oi.Nonce, oi.SeqNum, oi.Size, oi.Segments.Len()}}
		table.Output(tmp)
		table.Output(oi.Segments)

		return nil
	},
}
