package data

import (
	"bytes"
	"context"
	"encoding/binary"
	"math/big"

	lru "github.com/hashicorp/golang-lru"
	"golang.org/x/xerrors"

	"github.com/memoio/go-mefs-v2/api"
	"github.com/memoio/go-mefs-v2/build"
	"github.com/memoio/go-mefs-v2/lib/address"
	logging "github.com/memoio/go-mefs-v2/lib/log"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/segment"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"
	"github.com/memoio/go-mefs-v2/submodule/connect/readpay"
)

var logger = logging.Logger("data-service")

type dataService struct {
	api.INetService // net for send
	api.IRole

	ds       store.KVStore
	segStore segment.SegmentStore

	cache *lru.ARCCache

	is readpay.ISender
}

func New(ds store.KVStore, ss segment.SegmentStore, ins api.INetService, ir api.IRole, is readpay.ISender) *dataService {
	// 250MB
	cache, _ := lru.NewARC(1024)

	d := &dataService{
		INetService: ins,
		IRole:       ir,
		is:          is,
		ds:          ds,
		segStore:    ss,
		cache:       cache,
	}

	return d
}

func (d *dataService) API() *dataAPI {
	return &dataAPI{d}
}

func (d *dataService) PutSegmentToLocal(ctx context.Context, seg segment.Segment) error {
	logger.Debug("put segment to local: ", seg.SegmentID())
	d.cache.Add(seg.SegmentID(), seg)
	return d.segStore.Put(seg)
}

func (d *dataService) GetSegmentFromLocal(ctx context.Context, sid segment.SegmentID) (segment.Segment, error) {
	logger.Debug("get segment from local: ", sid)
	val, has := d.cache.Get(sid.String())
	if has {
		logger.Debugf("cache has segment: %s", sid.String())
		return val.(*segment.BaseSegment), nil
	}
	return d.segStore.Get(sid)
}

func (d *dataService) HasSegment(ctx context.Context, sid segment.SegmentID) (bool, error) {
	logger.Debug("has segment from local: ", sid)
	has := d.cache.Contains(sid.String())
	if has {
		return true, nil
	}
	return d.segStore.Has(sid)
}

// SendSegment over network
func (d *dataService) SendSegment(ctx context.Context, seg segment.Segment, to uint64) error {
	// send to remote
	data, err := seg.Serialize()
	if err != nil {
		return err
	}
	resp, err := d.SendMetaRequest(ctx, to, pb.NetMessage_PutSegment, data, nil)
	if err != nil {
		return err
	}

	if resp.GetHeader().GetType() == pb.NetMessage_Err {
		return xerrors.Errorf("send to %d fails %s", to, string(resp.GetData().MsgInfo))
	}

	return nil
}

func (d *dataService) SendSegmentByID(ctx context.Context, sid segment.SegmentID, to uint64) error {
	// load segment from local
	seg, err := d.GetSegmentFromLocal(ctx, sid)
	if err != nil {
		return err
	}

	return d.SendSegment(ctx, seg, to)
}

// GetSegment from local and network
func (d *dataService) GetSegment(ctx context.Context, sid segment.SegmentID) (segment.Segment, error) {
	// get location from local
	seg, err := d.GetSegmentFromLocal(ctx, sid)
	if err == nil {
		return seg, nil
	}

	from, err := d.GetSegmentLocation(ctx, sid)
	if err != nil {
		return seg, err
	}

	pri, err := d.RoleGet(ctx, from)
	if err != nil {
		return seg, err
	}

	fromAddr, err := address.NewAddress(pri.ChainVerifyKey)
	if err != nil {
		return seg, err
	}

	readPrice := big.NewInt(types.DefaultReadPrice)
	readPrice.Mul(readPrice, big.NewInt(build.DefaultSegSize))

	sig, err := d.is.Pay(fromAddr, readPrice)
	if err != nil {
		return seg, err
	}

	return d.GetSegmentRemote(ctx, sid, from, sig)
}

func (d *dataService) GetSegmentLocation(ctx context.Context, sid segment.SegmentID) (uint64, error) {
	key := store.NewKey(pb.MetaType_SegLocationKey, sid.ToString())
	val, err := d.ds.Get(key)
	if err != nil {
		return 0, err
	}

	if len(val) < 8 {
		return 0, xerrors.Errorf("location is wrong")
	}

	return binary.BigEndian.Uint64(val), nil
}

// GetSegmentFrom get segmemnt over network
func (d *dataService) GetSegmentRemote(ctx context.Context, sid segment.SegmentID, from uint64, sig []byte) (segment.Segment, error) {
	logger.Debug("get segment from remote: ", sid, from)
	resp, err := d.SendMetaRequest(ctx, from, pb.NetMessage_GetSegment, sid.Bytes(), sig)
	if err != nil {
		return nil, err
	}

	if resp.Header.Type == pb.NetMessage_Err {
		return nil, xerrors.Errorf("get segment from %d fails %s", from, string(resp.GetData().MsgInfo))
	}

	bs := new(segment.BaseSegment)
	err = bs.Deserialize(resp.GetData().GetMsgInfo())
	if err != nil {
		return nil, err
	}

	if !bytes.Equal(sid.Bytes(), bs.SegmentID().Bytes()) {
		return nil, xerrors.Errorf("segment is not required, expected %s, got %s", sid, bs.SegmentID())
	}

	d.cache.Add(bs.SegmentID().String(), bs)

	// save to local? or after valid it?

	return bs, nil
}

func (d *dataService) DeleteSegment(ctx context.Context, sid segment.SegmentID) error {
	logger.Debug("delete segment in local: ", sid)
	d.cache.Remove(sid.String())
	return d.segStore.Delete(sid)
}
