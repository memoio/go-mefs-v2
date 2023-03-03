package etag

import (
	"crypto/md5"
	"encoding/hex"
	"hash"

	"github.com/ethereum/go-ethereum/common/math"
	"github.com/gogo/protobuf/proto"
	"github.com/ipfs/go-cid"
	format "github.com/ipfs/go-ipld-format"
	pb "github.com/ipfs/go-unixfs/pb"
	dagpb "github.com/ipld/go-codec-dagpb"
	"github.com/ipld/go-ipld-prime"
	"github.com/ipld/go-ipld-prime/fluent/qp"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	mh "github.com/multiformats/go-multihash"
	"golang.org/x/xerrors"
)

const (
	MaxLinkCnt       = 174        // 8192/(34+8+5)
	DefaultChunkSize = 248 * 1024 // 248KB
	DefaultHashType  = mh.SHA2_256
)

type RootNode struct {
	size  uint64
	links []*format.Link
	pd    *pb.Data
}

func NewRootNode() *RootNode {
	typ := pb.Data_File
	return &RootNode{
		links: make([]*format.Link, 0, 174),
		pd: &pb.Data{
			Type:       &typ,
			Blocksizes: make([]uint64, 0, 174),
		},
	}
}

func (n *RootNode) Serialize() ([]byte, error) {
	nd, err := qp.BuildMap(dagpb.Type.PBNode, 2, func(ma ipld.MapAssembler) {
		qp.MapEntry(ma, "Links", qp.List(int64(len(n.links)), func(la ipld.ListAssembler) {
			for _, link := range n.links {
				qp.ListEntry(la, qp.Map(3, func(ma ipld.MapAssembler) {
					if link.Cid.Defined() {
						qp.MapEntry(ma, "Hash", qp.Link(cidlink.Link{Cid: link.Cid}))
					}
					qp.MapEntry(ma, "Name", qp.String(link.Name))
					qp.MapEntry(ma, "Tsize", qp.Int(int64(link.Size)))
				}))
			}
		}))
		if n.pd != nil {
			pdb, err := proto.Marshal(n.pd)
			if err == nil {
				qp.MapEntry(ma, "Data", qp.Bytes(pdb))
			}
		}
	})
	if err != nil {
		return nil, err
	}

	// 1KiB can be allocated on the stack, and covers most small nodes
	// without having to grow the buffer and cause allocations.
	enc := make([]byte, 0, 1024)

	enc, err = dagpb.AppendEncode(enc, nd)
	if err != nil {
		return nil, err
	}

	return enc, nil
}

func (n *RootNode) Clone() *RootNode {
	rt := NewRootNode()
	for i, l := range n.links {
		rt.AddLink(l.Name, n.pd.Blocksizes[i], l.Size, l.Cid)
	}

	return rt
}

func (n *RootNode) AddLink(name string, filesize, size uint64, cid cid.Cid) {
	// update size
	n.pd.Blocksizes = append(n.pd.Blocksizes, filesize)
	oldSize := n.pd.GetFilesize() + filesize
	n.pd.Filesize = &oldSize

	// update link
	n.links = append(n.links, &format.Link{
		Name: name,
		Size: size,
		Cid:  cid,
	})

	n.size += size
}

func (n *RootNode) Sum(c *Config) (uint64, uint64, cid.Cid) {
	res, err := n.Serialize()
	if err != nil {
		return 0, 0, cid.Undef
	}

	s := uint64(len(res)) + n.size

	mhtag, err := c.Hash(res)
	if err != nil {
		return 0, 0, cid.Undef
	}

	return n.pd.GetFilesize(), uint64(s), cid.NewCidV1(cid.DagProtobuf, mhtag)
}

func (n *RootNode) Full() bool {
	return len(n.links) >= MaxLinkCnt
}

func (n *RootNode) Empty() bool {
	return len(n.links) == 0
}

func (n *RootNode) Reset() {
	n.size = 0
	n.links = make([]*format.Link, 0, 174)
	n.pd = &pb.Data{
		Type:       n.pd.Type,
		Blocksizes: make([]uint64, 0, 174),
	}
}

var _ hash.Hash = (*Tree)(nil)

type Config struct {
	BlockSize int // need store
	HashType  int // can decode from cid
}

type Tree struct {
	depth  int
	layers []*RootNode

	cfg *Config

	nx    int
	cData []byte
}

func NewTree() *Tree {
	c := new(Config)

	return NewTreeWithConfig(c)
}

func NewTreeWithConfig(c *Config) *Tree {
	if c.BlockSize <= 0 {
		c.BlockSize = DefaultChunkSize
	}
	if c.BlockSize > math.MaxInt32 {
		c.BlockSize = DefaultChunkSize
	}

	tr := &Tree{
		layers: make([]*RootNode, 0, 8),
		depth:  1,
		cfg:    c,
		cData:  make([]byte, 0, c.BlockSize),
	}
	tr.layers = append(tr.layers, NewRootNode())

	return tr
}

// Reset resets the Hash to its initial state.
func (tr *Tree) Reset() {
	tr.depth = 1
	tr.layers = make([]*RootNode, 0, 8)
	tr.layers = append(tr.layers, NewRootNode())

	tr.nx = 0
	tr.cData = tr.cData[:0]
}

// Size returns the number of bytes Sum will return.
func (tr *Tree) Size() int {
	return 36
}

// BlockSize returns the hash's underlying block size.
// The Write method must be able to accept any amount
// of data, but it may operate more efficiently if all writes
// are a multiple of the block size.
func (tr *Tree) BlockSize() int {
	return tr.cfg.BlockSize
}

func (tr *Tree) Write(p []byte) (int, error) {
	nn := len(p)
	if nn == 0 {
		return 0, nil
	}
	if tr.nx > 0 {
		clen := tr.cfg.BlockSize - tr.nx
		if clen > nn {
			clen = nn
		}
		tr.cData = append(tr.cData, p[:clen]...)
		tr.nx = len(tr.cData)
		if tr.nx == tr.cfg.BlockSize {
			tr.addChunk(tr.cData)
			tr.nx = 0
			p = p[clen:]
			tr.cData = tr.cData[:0]
		}
	}

	for len(p) >= tr.cfg.BlockSize {
		tr.addChunk(p[:tr.cfg.BlockSize])
		p = p[tr.cfg.BlockSize:]
	}

	// copy left here
	if len(p) > 0 {
		tr.cData = append(tr.cData, p...)
		tr.nx = len(tr.cData)
	}
	return nn, nil
}

func (tr *Tree) addChunk(b []byte) {
	cid := tr.cfg.NewCid(b)
	tr.addCid(cid, len(b))
}

func (tr *Tree) addCid(cid cid.Cid, size int) {
	tr.layers[0].AddLink("", uint64(size), uint64(size), cid)
	for i := 0; i < tr.depth; i++ {
		if tr.layers[i].Full() {
			if i == tr.depth-1 {
				// handle last layer
				tr.layers = append(tr.layers, NewRootNode())
				tr.depth++
			}
			fLen, sLen, cid := tr.layers[i].Sum(tr.cfg)
			tr.layers[i+1].AddLink("", fLen, sLen, cid)
			tr.layers[i].Reset()
		} else {
			break
		}
	}
}

func (tr *Tree) Sum(p []byte) []byte {
	layers := make([]*RootNode, tr.depth)
	for i := 0; i < tr.depth; i++ {
		layers[i] = tr.layers[i].Clone()
	}

	if tr.nx > 0 {
		layers[0].AddLink("", uint64(tr.nx), uint64(tr.nx), tr.cfg.NewCid(tr.cData))
	}

	for i := 0; i < tr.depth-1; i++ {
		fLen, sLen, cid := layers[i].Sum(tr.cfg)
		layers[i+1].AddLink("", fLen, sLen, cid)
	}

	var root cid.Cid
	if tr.depth == 1 && len(layers[0].links) == 1 {
		root = layers[0].links[0].Cid
	} else {
		_, _, root = layers[tr.depth-1].Sum(tr.cfg)
	}

	return root.Bytes()
}

//  36byte: version(1 byte,value 1) + cidCodec(1 byte) + hashType(1 byte) + hashLen(1 byte) + hashValue(32byte for SHA2_256)
func (c *Config) NewCid(data []byte) cid.Cid {
	mhtag, err := c.Hash(data)
	if err != nil {
		panic(err)
	}
	return cid.NewCidV1(cid.Raw, mhtag)
}

func (c *Config) Hash(data []byte) ([]byte, error) {
	switch c.HashType {
	case mh.SHA2_256:
		fallthrough
	default:
		return mh.Sum(data, mh.SHA2_256, 32)
	}
}

func ToString(etag []byte) (string, error) {
	if len(etag) == md5.Size {
		return hex.EncodeToString(etag), nil
	}

	_, ecid, err := cid.CidFromBytes(etag)
	if err != nil {
		return "", err
	}

	return ecid.String(), nil
}

func ToCidV0String(etag []byte) (string, error) {
	if len(etag) == md5.Size {
		return "", xerrors.Errorf("invalid cid format")
	}

	_, ecid, err := cid.CidFromBytes(etag)
	if err != nil {
		return "", err
	}
	if len(etag) > 2 && etag[0] == mh.SHA2_256 && etag[1] == 32 {
		return ecid.String(), nil
	}

	// change it to v0 string
	return cid.NewCidV0(ecid.Hash()).String(), nil
}

func ToByte(str string) ([]byte, error) {
	if len(str) == 2*md5.Size {
		return hex.DecodeString(str)
	}

	ncid, err := cid.Decode(str)
	if err != nil {
		return nil, err
	}

	return ncid.Bytes(), nil
}
