package hotstuff

import (
	"testing"

	"github.com/memoio/go-mefs-v2/lib/types"
)

func TestHs(t *testing.T) {
	hs := new(HotstuffMessage)
	hs.From = 1
	hs.Type = MsgCommit
	hs.Quorum = types.NewMultiSignature(types.SigBLS)

	data, err := hs.Serialize()
	if err != nil {
		t.Fatal(err)
	}

	nhs := new(HotstuffMessage)
	err = nhs.Deserialize(data)
	if err != nil {
		t.Fatal(err)
	}

	t.Log(hs.Type, nhs.Type, hs.Data.PrevID.String())
	t.Fail()
}
