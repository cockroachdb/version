package version

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVersionScan(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		v := MustParse("v20.1.2-alpha.3-cloudonly.4")

		var scanned Version
		err := scanned.Scan(v.String())
		require.NoError(t, err)
		require.Equal(t, v, scanned)
	})

	t.Run("empty", func(t *testing.T) {
		var scanned Version
		err := scanned.Scan("")
		require.NoError(t, err)
		require.True(t, scanned.Empty())
	})
}

func TestNullVersionScan(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		v := MustParse("v20.1.2-alpha.3-cloudonly.4")
		nv := NewNullVersion(v)

		var scanned NullVersion
		err := scanned.Scan(v.String())
		require.NoError(t, err)
		require.Equal(t, nv, scanned)
	})

	t.Run("null", func(t *testing.T) {
		var scanned NullVersion
		err := scanned.Scan(nil)
		require.NoError(t, err)
		require.False(t, scanned.Valid)
		require.Equal(t, Version{}, scanned.Version)
	})

	t.Run("empty", func(t *testing.T) {
		var scanned NullVersion
		err := scanned.Scan("")
		require.NoError(t, err)
		require.True(t, scanned.Valid)
		require.True(t, scanned.Version.Empty())
	})
}
