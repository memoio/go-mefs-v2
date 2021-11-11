package data

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"

	"github.com/memoio/go-mefs-v2/api"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/segment"
	"github.com/memoio/go-mefs-v2/lib/types/store"
	"github.com/memoio/go-mefs-v2/lib/utils"
)

var (
	ErrData = errors.New("err data")
)

type dataService struct {
	api.INetService // net for send

	ds       store.KVStore
	segStore segment.SegmentStore
}

func New(ds store.KVStore, ss segment.SegmentStore, is api.INetService) *dataService {
	d := &dataService{
		INetService: is,
		ds:          ds,
		segStore:    ss,
	}

	return d
}

// todo add piece put/get
// add cache

// SendSegment over network
func (d *dataService) SendSegment(ctx context.Context, seg segment.Segment, to uint64) error {
	// send to remote
	data, err := seg.Serialize()
	if err != nil {
		return err
	}
	_, err = d.SendMetaRequest(ctx, to, pb.NetMessage_PutSegment, data, nil)
	if err != nil {
		return err
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
	seg, err := d.segStore.Get(sid)
	if err != nil {
		return err
	}

	return d.SendSegment(ctx, seg, to)
}

// GetSegment from local and network
func (d *dataService) GetSegment(ctx context.Context, sid segment.SegmentID) (segment.Segment, error) {
	// get location from local
	seg, err := d.segStore.Get(sid)
	if err == nil {
		return seg, nil
	}

	// get from remote
	key := store.NewKey(pb.MetaType_SegLocationKey, sid.String())
	val, err := d.ds.Get(key)
	if err != nil {
		return nil, err
	}
	from := binary.BigEndian.Uint64(val)

	return d.GetSegmentFrom(ctx, sid, from)
}

// GetSegmentFrom get segmemnt over network
func (d *dataService) GetSegmentFrom(ctx context.Context, sid segment.SegmentID, from uint64) (segment.Segment, error) {
	resp, err := d.SendMetaRequest(ctx, from, pb.NetMessage_GetSegment, sid.Bytes(), nil)
	if err != nil {
		return nil, err
	}

	bs := new(segment.BaseSegment)

	err = bs.Deserialize(resp.GetData().GetMsgInfo())
	if err != nil {
		return nil, err
	}

	if !bytes.Equal(sid.Bytes(), bs.SegmentID().Bytes()) {
		return nil, ErrData
	}

	// save to local?

	return bs, nil
}
