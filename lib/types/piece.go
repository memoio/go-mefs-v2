package types

import (
	"math/big"
	"sync"

	"github.com/bits-and-blooms/bitset"
	"github.com/memoio/go-mefs-v2/lib/pb"
)

type PieceID [32]byte

// in memory; delete when all locations are active
// store in (PieceLocation) kv when active
type Piece struct {
	active bool   // banned?
	fsID   uint64 // whose piece; for calculated pay?
	expire uint64
	size   uint64
	proIDs []uint64
	price  []*big.Int
	commit []bool // commit + price>0 = veirfy
}

func (p *Piece) Set(porID uint64) error {
	return nil
}

type PieceMgr struct {
	sync.RWMutex
	pieces map[PieceID]*Piece // key: pieceID; value:*Piece; for confirmation;
}

func GetPiece(pid PieceID) (*Piece, error) {
	// get from memeory
	// get from kv
	return nil, nil
}

func (pi *PieceMgr) GetPiece(pid PieceID) (*Piece, error) {
	pi.RLock()
	defer pi.RUnlock()

	p, ok := pi.pieces[pid]
	if !ok {
		return nil, ErrKeyInfoNotFound
	}

	return p, nil
}

func (pi *PieceMgr) AddPiece(pid PieceID, p *Piece) error {
	pi.Lock()
	defer pi.Unlock()

	pi.pieces[pid] = p

	return nil
}

func (pi *PieceMgr) AddSector(sid, proID uint64, swp *pb.SectorPieces) error {

	// verify piece
	for _, pie := range swp.PieceID {
		var pid PieceID
		copy(pid[:], pie)
		p, err := pi.GetPiece(pid)
		if err != nil {
			return err
		}

		err = p.Set(proID)
		if err != nil {
			return err
		}
	}

	return nil
}

type SectorMgr struct {
	sync.RWMutex                           // rw lock for sectors
	sector       uint64                    // largest sector number; incremental from 0
	sectorSet    *bitset.BitSet            // sectors per provider; for challenge
	sectorMap    map[uint64]*pb.SectorInfo // key: sectorID; value: *Sector
}

type PieceInfo struct {
	FsID   uint64
	Expire uint64
	Size   uint64
}

// verify commd; verify proof
type SectorCommit struct {
	pb.SectorInfo
	sectorID uint64
	pInfo    []PieceInfo
	proof    []byte
}
