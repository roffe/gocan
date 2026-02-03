package canusb

import "testing"

func TestCANHANDLE_VersionInfo(t *testing.T) {
	if err := Init(); err != nil {
		t.Fatalf("canusb.Init() error: %v", err)
	}

	tests := []struct {
		name string

		want    string
		wantErr bool
	}{
		{
			name:    "",
			want:    "V1011 - NY657 - 0.2.2 - LAWICEL AB",
			wantErr: false,
		},
		{
			name:    "lsdfhldshflds",
			want:    "",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Log(tt.name)
			ch, err := Open(tt.name, "500", ACCEPTANCE_CODE_ALL, ACCEPTANCE_MASK_ALL, FLAG_BLOCK|FLAG_TIMESTAMP)
			if err != nil {
				if tt.wantErr {
					return
				}
				t.Errorf("Open() error = %v", err)
				return
			}
			defer ch.Close()
			got, err := ch.VersionInfo()
			if (err != nil) != tt.wantErr {
				t.Errorf("CANHANDLE.VersionInfo() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("CANHANDLE.VersionInfo() = %v, want %v", got, tt.want)
			}

		})
	}
}
