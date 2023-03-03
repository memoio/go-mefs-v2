package etag

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"testing"

	mh "github.com/multiformats/go-multihash"
)

func TestChunk(t *testing.T) {
	buf, _ := ioutil.ReadFile("/home/fjt/go-mefs-v2/mefs-user")
	bufLen := len(buf)

	tr := NewTree(248 * 1024)

	mhtag, err := mh.Encode(buf[:32], mh.SHA2_256)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(len(mhtag))

	fmt.Println(bufLen)

	tr.Write(buf[:248*1024+1])
	root := tr.Sum(nil)
	t.Log(root)

	tr.Write(buf[248*1024+1:])
	root = tr.Sum(nil)
	t.Log(len(root))

	s, err := ToString(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(s)

	//tr.Result()

	// change it to v0 string
	t.Fatal("cid.String()")
}

func BenchmarkChunk(b *testing.B) {
	buf, _ := ioutil.ReadFile("/home/fjt/go-mefs-v2/mefs-user")
	bufLen := len(buf)

	tr := NewTree(248 * 1024)

	fmt.Println(bufLen)

	tr.Write(buf)
	root := tr.Sum(nil)
	s, err := ToString(root)
	if err != nil {
		b.Fatal(err)
	}
	b.Log(s)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		j := i % bufLen
		tr.Reset()
		tr.Write(buf[:j])
		tr.Sum(nil)

		tr.Write(buf[j:])
		nroot := tr.Sum(nil)
		if !bytes.Equal(root, nroot) {
			ns, err := ToString(nroot)
			if err != nil {
				b.Fatal(err)
			}
			b.Fatal("wrong root: ", i, ns)
		}
	}
}
