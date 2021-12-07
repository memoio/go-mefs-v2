package data

import (
	"bytes"
	"context"
	"encoding/binary"

	lru "github.com/hashicorp/golang-lru"
	"golang.org/x/xerrors"

	"github.com/memoio/go-mefs-v2/api"
	logging "github.com/memoio/go-mefs-v2/lib/log"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/segment"
	"github.com/memoio/go-mefs-v2/lib/types/store"
	"github.com/memoio/go-mefs-v2/lib/utils"
)

var logger = logging.Logger("data-service")

var (
	ErrData = xerrors.New("err data")
	ErrSend = xerrors.New("send fails")
)

type dataService struct {
	api.INetService // net for send

	ds       store.KVStore
	segStore segment.SegmentStore

	cache *lru.ARCCache
}

func New(ds store.KVStore, ss segment.SegmentStore, is api.INetService) *dataService {
	cache, _ := lru.NewARC(1024)

	d := &dataService{
		INetService: is,
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
	logger.Debug("put segment to local:", seg.SegmentID().String())
	d.cache.Add(seg.SegmentID(), seg)
	return d.segStore.Put(seg)
}

func (d *dataService) GetSegmentFromLocal(ctx context.Context, sid segment.SegmentID) (segment.Segment, error) {
	logger.Debug("get segment from local:", sid.String())
	val, has := d.cache.Get(sid)
	if has {
		return val.(segment.Segment), nil
	}
	return d.segStore.Get(sid)
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
		return ErrSend
	}

	// save meta
	key := store.NewKey(pb.MetaType_SegLocationKey, seg.SegmentID().String())
	val := utils.UintToBytes(to)
	err = d.ds.Put(key, val)
	if err != nil {
		return err
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

	// get from remote
	key := store.NewKey(pb.MetaType_SegLocationKey, sid.String())
	val, err := d.ds.Get(key)
	if err != nil {
		return nil, err
	}

	if len(val) < 8 {
		return nil, ErrData
	}

	from := binary.BigEndian.Uint64(val)

	return d.GetSegmentRemote(ctx, sid, from)
}

// todo: add readpay sign here

// GetSegmentFrom get segmemnt over network
func (d *dataService) GetSegmentRemote(ctx context.Context, sid segment.SegmentID, from uint64) (segment.Segment, error) {
	resp, err := d.SendMetaRequest(ctx, from, pb.NetMessage_GetSegment, sid.Bytes(), nil)
	if err != nil {
		return nil, err
	}

	if resp.Header.Type == pb.NetMessage_Err {
		return nil, ErrData
	}

	bs := new(segment.BaseSegment)
	err = bs.Deserialize(resp.GetData().GetMsgInfo())
	if err != nil {
		return nil, err
	}

	if !bytes.Equal(sid.Bytes(), bs.SegmentID().Bytes()) {
		return nil, ErrData
	}

	// save to local? or after valid it?

	return bs, nil
}
