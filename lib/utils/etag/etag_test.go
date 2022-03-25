package etag

import (
	"fmt"
	"io/ioutil"
	"testing"
)

func TestChunk(t *testing.T) {
	buf, _ := ioutil.ReadFile("/home/fjt/go-mefs-v2/mefs")
	bufLen := len(buf)

	tr := NewTree()

	fmt.Println(bufLen)

	for start := 0; start < bufLen; {
		stepLen := 32
		if start+stepLen > bufLen {
			stepLen = bufLen - start
		}
		cid := NewCidFromData(buf[start : start+stepLen])
		tr.AddCid(cid, uint64(stepLen))
		start += stepLen
	}

	t.Log(tr.Root())

	//tr.Result()

	// change it to v0 string
	t.Fatal("cid.String()")
}
