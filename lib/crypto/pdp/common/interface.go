package pdpcommon

type KeySet interface {
	Version() int
	GenTag(index []byte, segment []byte, start, typ int, mode bool) ([]byte, error)
	PublicKey() PublicKey
	SecreteKey() SecretKey
	VerifyKey() VerifyKey
	VerifyData(indices [][]byte, segments, tags [][]byte, typ int) (bool, error)
}

type Challenge interface {
	Version() int
	GetSeed() int64
	GetIndices() [][]byte
	Serialize() []byte
	Deserialize(buf []byte) error
}

type SecretKey interface {
	Version() int
	Serialize() []byte
	Deserialize(buf []byte) error
}

type PublicKey interface {
	Version() int
	VerifyKey() VerifyKey
	VerifyTag(index, segment, tag []byte, typ int) (bool, error)
	GenProof(chal Challenge, segments, tags [][]byte, typ int) (Proof, error)
	GetCount() int64
	Serialize() []byte
	Deserialize(buf []byte) error
}

type VerifyKey interface {
	Version() int
	VerifyProof(chal Challenge, proof Proof) (bool, error)
	Serialize() []byte
	Deserialize(buf []byte) error
}

type Proof interface {
	Version() int
	Serialize() []byte
	Deserialize(buf []byte) error
}

//往里面塞segment, tag，最后返回一个证明。
type ProofAggregator interface {
	Version() int
	Input(segment []byte, tag []byte, typ int) error
	InputMulti(segments [][]byte, tags [][]byte, typ int) error
	Result() (Proof, error)
}

//往里面加入index，segment，tag，最后返回是否能通过证明。
type DataVerifier interface {
	Version() int
	Input(index, segment, tag []byte) error
	InputMulti(indices, segments, tags [][]byte) error
	Result() bool
	Reset() // clear after result
}

// //聚合Indices，用于最终验证证明
// type IndicesAggregator interface {
// 	Input(index []byte) error
// 	InputMulti(indices [][]byte) error
// 	Result() ([]byte, error)
// }
