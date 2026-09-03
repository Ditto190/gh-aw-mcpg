package mountspec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name         string
		spec         string
		wantWritable bool
		wantErr      bool
		errorKind    ErrorKind
	}{
		{name: "read-only", spec: "/srv/data:/data:ro"},
		{name: "read-write", spec: "/srv/data:/data:rw", wantWritable: true},
		{name: "omitted mode defaults to read-write", spec: "/srv/data:/data", wantWritable: true},
		{name: "duplicate modes rejected", spec: "/srv/data:/data:ro,rw", wantErr: true, errorKind: InvalidOptions},
		{name: "empty mode option rejected", spec: "/srv/data:/data:ro,,rw", wantErr: true, errorKind: InvalidOptions},
		{name: "unsupported option rejected", spec: "/srv/data:/data:rslave", wantErr: true, errorKind: InvalidOptions},
		{name: "empty source rejected", spec: ":/data:ro", wantErr: true, errorKind: EmptySource},
		{name: "empty destination rejected", spec: "/srv/data::ro", wantErr: true, errorKind: EmptyDestination},
		{name: "relative source rejected", spec: "data:/data:ro", wantErr: true, errorKind: RelativeSource},
		{name: "relative destination rejected", spec: "/srv/data:data:ro", wantErr: true, errorKind: RelativeDestination},
		{name: "too many segments rejected", spec: "/srv/data:/data:ro:extra", wantErr: true, errorKind: InvalidFormat},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mount, err := Parse(tt.spec)
			if !tt.wantErr {
				require.NoError(t, err)
				assert.Equal(t, "/srv/data", mount.Source)
				assert.Equal(t, "/data", mount.Destination)
				assert.Equal(t, tt.wantWritable, mount.Writable)
				return
			}

			require.Error(t, err)
			var parseErr *ParseError
			require.ErrorAs(t, err, &parseErr)
			assert.Equal(t, tt.errorKind, parseErr.Kind)
		})
	}
}

func TestParseRequiredMode(t *testing.T) {
	_, err := ParseRequiredMode("/srv/data:/data")
	require.Error(t, err)

	var parseErr *ParseError
	require.ErrorAs(t, err, &parseErr)
	assert.Equal(t, InvalidFormat, parseErr.Kind)
}
