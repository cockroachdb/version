package version

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVersionJSONSerialization(t *testing.T) {
	v := MustParse("v20.1.2-alpha.3-cloudonly.4")

	blob, err := json.Marshal(v)
	require.NoError(t, err)

	var parsed Version
	err = json.Unmarshal(blob, &parsed)
	require.NoError(t, err)

	require.Equal(t, v, parsed)
}

func TestNullVersionJSONSerialization(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		v := MustParse("v20.1.2-alpha.3-cloudonly.4")
		nv := NewNullVersion(v)

		blob, err := json.Marshal(nv)
		require.NoError(t, err)

		var parsed NullVersion
		err = json.Unmarshal(blob, &parsed)
		require.NoError(t, err)

		require.True(t, parsed.Valid)
		require.Equal(t, v, parsed.Version)
	})

	t.Run("invalid", func(t *testing.T) {
		v := NullVersion{Valid: false}

		blob, err := json.Marshal(v)
		require.NoError(t, err)

		var parsed NullVersion
		err = json.Unmarshal(blob, &parsed)
		require.NoError(t, err)

		require.False(t, parsed.Valid)
		require.Equal(t, Version{}, parsed.Version)
	})
}
