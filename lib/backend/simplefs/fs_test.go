package simplefs

import (
	"os"
	"path"
	"testing"
)

func TestPath(t *testing.T) {
	p := "abd/abc/abc"

	t.Log(path.Base(p))
	t.Log(path.Dir(p))

	t.Log(path.Join("~", path.Dir(p)))

	err := os.MkdirAll(path.Join("/home/fjt", path.Dir(p)), 0755)
	t.Fatal(err)

	t.Fatal("finish")
}
