package paths

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRepoPathGet(t *testing.T) {

	t.Run("get default repo path", func(t *testing.T) {
		_, err := GetRepoPath("")

		require.NoError(t, err)
	})
}
