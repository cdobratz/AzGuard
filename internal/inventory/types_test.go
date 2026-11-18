package inventory

import "testing"

func TestAttachVolumeSizes(t *testing.T) {
	instances := []Instance{{
		ID: "i-1",
		Volumes: []AttachedVolume{
			{ID: "vol-1"},
			{ID: "vol-2", SizeGB: 4, VolumeType: "gp2"},
			{ID: "vol-missing"},
		},
	}}
	volumes := []Volume{
		{ID: "vol-1", SizeGB: 8, VolumeType: "gp3"},
		{ID: "vol-2", SizeGB: 99, VolumeType: "io1"},
	}
	out := AttachVolumeSizes(instances, volumes)
	if out[0].Volumes[0].SizeGB != 8 || out[0].Volumes[0].VolumeType != "gp3" {
		t.Fatalf("filled %+v", out[0].Volumes[0])
	}
	if out[0].Volumes[1].SizeGB != 4 || out[0].Volumes[1].VolumeType != "gp2" {
		t.Fatalf("kept existing %+v", out[0].Volumes[1])
	}
	if out[0].Volumes[2].SizeGB != 0 {
		t.Fatalf("missing %+v", out[0].Volumes[2])
	}
	if instances[0].Volumes[0].SizeGB != 0 {
		t.Fatal("input was mutated")
	}
}
