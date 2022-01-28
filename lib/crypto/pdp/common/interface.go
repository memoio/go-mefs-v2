package pdpcommon

type KeySet interface {
	Version() uint16
	PublicKey() PublicKey
	SecreteKey() SecretKey
	VerifyKey() VerifyKey

	GenTag(index, segment []byte, start int, mode bool) ([]byte, error)
	VerifyTag(indice, segment, tag []byte) (bool, error)
}

type SecretKey interface {
	Version() uint16
	Serialize() []byte
	Deserialize([]byte) error
}

type PublicKey interface {
	Version() uint16
	GetCount() int64

	VerifyKey() VerifyKey

	VerifyTag(index, segment, tag []byte, typ int) (bool, error)
	GenProof(chal Challenge, segment, tag []byte, typ int) (Proof, error)

	Serialize() []byte
	Deserialize(buf []byte) error
}

type VerifyKey interface {
	Version() uint16

	VerifyProof(chal Challenge, proof Proof) (bool, error)

	Hash() []byte
	Serialize() []byte
	Deserialize(buf []byte) error
}

type Challenge interface {
	Version() uint16

	Random() [32]byte
	PublicInput() []byte

	Add([]byte) error
	Delete([]byte) error

	Serialize() []byte
	Deserialize([]byte) error
}

type Proof interface {
	Version() uint16
	Serialize() []byte
	Deserialize(buf []byte) error
}

// aggreated proof
type ProofAggregator interface {
	Version() uint16
	Add(index, segment, tag []byte) error
	Result() (Proof, error)
}

// aggreated verify
type DataVerifier interface {
	Version() uint16
	Add(index, segment, tag []byte) error
	Result() (bool, error)
	Reset() // clear after result
}
