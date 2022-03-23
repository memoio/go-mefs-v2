package etag

import (
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

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
	MaxLinkCnt = 174 // 8192/(34+8+5)
)

type RootNode struct {
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
}

func (n *RootNode) Result() (uint64, uint64, cid.Cid) {
	res, err := n.Serialize()
	if err != nil {
		return 0, 0, cid.Undef
	}

	digest := sha256.Sum256(res)

	mhtag, err := mh.Encode(digest[:], mh.SHA2_256)
	if err != nil {
		return 0, 0, cid.Undef
	}

	s := uint64(len(res))
	for _, l := range n.links {
		s += l.Size
	}

	return n.pd.GetFilesize(), uint64(s), cid.NewCidV1(cid.DagProtobuf, mhtag)
}

func (n *RootNode) Full() bool {
	return len(n.links) >= MaxLinkCnt
}

func (n *RootNode) Empty() bool {
	return len(n.links) == 0
}

type Tree struct {
	layers []*RootNode
	depth  int

	cache map[cid.Cid]*RootNode
	root  cid.Cid
}

func NewTree() *Tree {
	tr := &Tree{
		layers: make([]*RootNode, 0, 8),
		depth:  1,
		cache:  make(map[cid.Cid]*RootNode, 8),
		root:   cid.Undef,
	}
	tr.layers = append(tr.layers, NewRootNode())

	return tr
}

func (tr *Tree) AddCid(cid cid.Cid, size uint64) {
	tr.layers[0].AddLink("", size, size, cid)

	for i := 0; i < tr.depth; i++ {
		if tr.layers[i].Full() && i+1 < tr.depth {
			fLen, sLen, cid := tr.layers[i].Result()
			tr.layers[i+1].AddLink("", fLen, sLen, cid)
			tr.cache[cid] = tr.layers[i]
			tr.layers[i] = NewRootNode()
		} else {
			break
		}
	}

	if tr.layers[tr.depth-1].Full() {
		tr.layers = append(tr.layers, NewRootNode())
		fLen, sLen, cid := tr.layers[tr.depth-1].Result()
		tr.layers[tr.depth].AddLink("", fLen, sLen, cid)
		tr.cache[cid] = tr.layers[tr.depth-1]
		tr.layers[tr.depth-1] = NewRootNode()
		tr.depth++
	}
}

func (tr *Tree) Result() {
	for i := 0; i < tr.depth-1; i++ {
		fLen, sLen, cid := tr.layers[i].Result()
		tr.layers[i+1].AddLink("", fLen, sLen, cid)
	}

	for i := 0; i < tr.depth; i++ {
		fLen, sLen, cid := tr.layers[i].Result()
		fmt.Println(i, fLen, sLen, cid.String())
	}
}

func (tr *Tree) TmpRoot() cid.Cid {
	if tr.root != cid.Undef {
		return tr.root
	}
	if tr.depth == 1 && len(tr.layers[0].links) == 1 {
		return tr.layers[0].links[0].Cid
	} else {
		_, _, root := tr.layers[tr.depth-1].Result()
		return root
	}
}

func (tr *Tree) Root() cid.Cid {
	if tr.root != cid.Undef {
		return tr.root
	}
	for i := 0; i < tr.depth-1; i++ {
		fLen, sLen, cid := tr.layers[i].Result()
		tr.layers[i+1].AddLink("", fLen, sLen, cid)
	}

	if tr.depth == 1 && len(tr.layers[0].links) == 1 {
		tr.root = tr.layers[0].links[0].Cid
	} else {
		_, _, tr.root = tr.layers[tr.depth-1].Result()
	}

	return tr.root
}

func NewCidFromData(data []byte) cid.Cid {
	digest := sha256.Sum256(data)

	mhtag, err := mh.Encode(digest[:], mh.SHA2_256)
	if err != nil {
		panic(err)
	}

	return cid.NewCidV1(cid.Raw, mhtag)
}

func NewCid(digest []byte) cid.Cid {
	mhtag, err := mh.Encode(digest, mh.SHA2_256)
	if err != nil {
		return cid.Undef
	}

	return cid.NewCidV1(cid.Raw, mhtag)
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
