package data

import (
	"bytes"
	"context"
	"encoding/binary"
	"math/big"
	"sync"
	"time"

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

type stat struct {
	sync.Mutex

	cnt   uint64
	total time.Duration
	times [100]time.Duration

	average int64
	pCnt    uint32
}

func (s *stat) add() {
	s.Lock()
	defer s.Unlock()
	s.pCnt++
}

func (s *stat) sub() {
	s.Lock()
	defer s.Unlock()
	s.pCnt--
}

func (s *stat) addDurtion(d time.Duration) {
	s.Lock()
	defer s.Unlock()

	s.total += d
	s.total -= s.times[s.cnt%100]
	s.average = (s.total / time.Duration(100)).Milliseconds()
	s.times[s.cnt%100] = d
	s.cnt++
}

// wait 10 ms for low priority if
// 1. more than 16 reads/writes
// 2. average read latency larger than 10ms
func (s *stat) wait() {
	logger.Debug("background read wait: ", s.pCnt, s.average)
	wait := 0
	for wait < 10 {
		if s.pCnt <= 16 && s.average <= 10 {
			return
		}

		time.Sleep(time.Millisecond)
		wait++
	}
}

type dataService struct {
	api.INetService // net for send
	api.IRole

	ds       store.KVStore
	segStore segment.SegmentStore

	cache *lru.ARCCache

	is readpay.ISender

	st *stat
}

func New(ds store.KVStore, ss segment.SegmentStore, ins api.INetService, ir api.IRole, is readpay.ISender) *dataService {
	// 250MB
	// TODO: from env
	cache, _ := lru.NewARC(1024)

	d := &dataService{
		INetService: ins,
		IRole:       ir,
		is:          is,
		ds:          ds,
		segStore:    ss,
		cache:       cache,
		st:          new(stat),
	}

	return d
}

func (d *dataService) API() *dataAPI {
	return &dataAPI{d}
}

func (d *dataService) PutSegmentToLocal(ctx context.Context, seg segment.Segment) error {
	logger.Debug("put segment to local: ", seg.SegmentID())
	d.cache.Add(seg.SegmentID(), seg)

	d.st.add()
	defer func() {
		d.st.sub()
	}()

	return d.segStore.Put(seg)
}

func (d *dataService) GetSegmentFromLocal(ctx context.Context, sid segment.SegmentID) (segment.Segment, error) {
	logger.Debug("get segment from local: ", sid)
	val, has := d.cache.Get(sid.String())
	if has {
		logger.Debugf("cache has segment: %s", sid.String())
		return val.(*segment.BaseSegment), nil
	}

	return d.getSegment(ctx, sid)
}

func (d *dataService) getSegment(ctx context.Context, sid segment.SegmentID) (segment.Segment, error) {
	logger.Debug("get segment from local store: ", sid)

	d.st.add()
	defer func() {
		d.st.sub()
	}()

	// backgroup read limit;
	// 1~2TB/day if read speed is slow
	if ctx.Value("Priority") != "" {
		d.st.wait()
	}

	nt := time.Now()
	seg, err := d.segStore.Get(sid)
	if err != nil {
		return nil, err
	}
	d.st.addDurtion(time.Since(nt))

	return seg, nil
}

func (d *dataService) HasSegment(ctx context.Context, sid segment.SegmentID) (bool, error) {
	//logger.Debug("has segment from local: ", sid)
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
		// only no such or all err?
		return xerrors.Errorf("missing chunk %s", err)
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

	return d.GetSegmentRemote(ctx, sid, from)
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

func (d *dataService) PutSegmentLocation(ctx context.Context, sid segment.SegmentID, pid uint64) error {
	key := store.NewKey(pb.MetaType_SegLocationKey, sid.ToString())
	val := make([]byte, 8)

	binary.BigEndian.PutUint64(val[:8], pid)
	return d.ds.Put(key, val)
}

func (d *dataService) DeleteSegmentLocation(ctx context.Context, sid segment.SegmentID) error {
	key := store.NewKey(pb.MetaType_SegLocationKey, sid.ToString())

	return d.ds.Delete(key)
}

// GetSegmentFrom get segmemnt over network
func (d *dataService) GetSegmentRemote(ctx context.Context, sid segment.SegmentID, from uint64) (segment.Segment, error) {
	logger.Debug("get segment from remote: ", sid, from)

	pri, err := d.RoleGet(ctx, from, false)
	if err != nil {
		return nil, err
	}

	fromAddr, err := address.NewAddress(pri.ChainVerifyKey)
	if err != nil {
		return nil, err
	}

	readPrice := big.NewInt(types.DefaultReadPrice)
	readPrice.Mul(readPrice, big.NewInt(build.DefaultSegSize))

	sig, err := d.is.Pay(fromAddr, readPrice)
	if err != nil {
		return nil, err
	}

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

func (d *dataService) Size() store.DiskStats {
	return d.segStore.Size()
}
